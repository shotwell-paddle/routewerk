package webhandler

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

// SessionList renders the setting sessions page (GET /sessions).
func (h *Handler) SessionList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		h.renderError(w, r, http.StatusBadRequest, "No location selected", "Please select a location.")
		return
	}

	statusFilter := r.URL.Query().Get("status")
	// Validate filter value
	if statusFilter != "" && statusFilter != "open" && statusFilter != "complete" {
		statusFilter = ""
	}
	// Map "open" to non-complete statuses at DB level
	var dbFilter string
	if statusFilter == "complete" {
		dbFilter = "complete"
	} else if statusFilter == "open" {
		dbFilter = "open"
	}

	sessions, err := h.sessionRepo.ListByLocationWithDetails(ctx, locationID, 50, 0, dbFilter)
	if err != nil {
		slog.Error("session list failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Could not load sessions.")
		return
	}

	data := &PageData{
		TemplateData:  templateDataFromContext(r, "sessions"),
		Sessions:      sessions,
		SessionFilter: statusFilter,
	}

	h.render(w, r, "setter/sessions.html", data)
}

// SessionDetail renders a single session with its assignments (GET /sessions/{sessionID}).
func (h *Handler) SessionDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := chi.URLParam(r, "sessionID")
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		h.renderError(w, r, http.StatusBadRequest, "No location selected", "Please select a location.")
		return
	}

	session, assignments, err := h.sessionRepo.GetByIDWithDetails(ctx, sessionID)
	if err != nil {
		slog.Error("session detail failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Could not load session.")
		return
	}
	if session == nil {
		h.renderError(w, r, http.StatusNotFound, "Session not found", "That session doesn't exist.")
		return
	}

	if !h.checkLocationOwnership(w, r, session.LocationID) {
		return
	}

	// Load setters for the assignment form
	setters, err := h.userRepo.ListSettersByLocation(ctx, locationID)
	if err != nil {
		slog.Error("load setters for session failed", "error", err)
	}

	// Load walls for the assignment form
	walls, err := h.wallRepo.ListByLocation(ctx, locationID)
	if err != nil {
		slog.Error("load walls for session failed", "error", err)
	}

	// Load strip targets for this session
	stripTargets, err := h.sessionRepo.ListStripTargets(ctx, sessionID)
	if err != nil {
		slog.Error("load strip targets failed", "error", err)
	}

	// Load walls with active routes for the strip target picker
	wallsWithRoutes, err := h.sessionRepo.ActiveRoutesByWall(ctx, locationID)
	if err != nil {
		slog.Error("load walls with routes failed", "error", err)
	}

	// Initialize checklist from location playbook (no-op if already done)
	if err := h.sessionRepo.InitializeChecklist(ctx, sessionID, locationID); err != nil {
		slog.Error("initialize checklist failed", "error", err)
	}

	// Load checklist items
	checklistItems, err := h.sessionRepo.ListChecklistItems(ctx, sessionID)
	if err != nil {
		slog.Error("load checklist failed", "error", err)
	}

	// Load routes created in this session
	sessionRoutes, err := h.sessionRepo.ListSessionRoutes(ctx, sessionID)
	if err != nil {
		slog.Error("load session routes failed", "error", err)
	}

	// Load location settings for grading method
	locSettings, settErr := h.settingsRepo.GetLocationSettings(ctx, locationID)
	if settErr != nil {
		slog.Error("load location settings failed", "error", settErr)
		locSettings = model.DefaultLocationSettings()
	}

	// Default route fields — no wall selected yet
	routeFields := buildRouteFields(sessionID, "", "", locSettings.Grading.BoulderMethod, "", locSettings, setters)

	data := &PageData{
		TemplateData:       templateDataFromContext(r, "sessions"),
		Session:            session,
		SessionAssignments: assignments,
		Setters:            setters,
		FormWalls:          walls,
		StripTargets:       stripTargets,
		WallsWithRoutes:    wallsWithRoutes,
		ChecklistItems:     checklistItems,
		SessionRoutes:      sessionRoutes,
		HoldColors:         holdColorsFromSettings(locSettings),
		VScaleGrades:       vScaleGrades,
		YDSGrades:          ydsGrades,
		CircuitColors:      circuitColors,
		BoulderMethod:      locSettings.Grading.BoulderMethod,
		RouteFields:        routeFields,
	}

	h.render(w, r, "setter/session-detail.html", data)
}

// SessionNew renders the new session form (GET /sessions/new).
func (h *Handler) SessionNew(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		h.renderError(w, r, http.StatusBadRequest, "No location selected", "Please select a location.")
		return
	}

	// Default date to tomorrow
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")

	data := &PageData{
		TemplateData: templateDataFromContext(r, "sessions"),
		SessionFormValues: SessionFormValues{
			ScheduledDate: tomorrow,
		},
	}

	h.render(w, r, "setter/session-form.html", data)
}

// SessionCreate handles the form POST to create a session (POST /sessions/new).
func (h *Handler) SessionCreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := middleware.GetWebUser(ctx)
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" || user == nil {
		h.renderError(w, r, http.StatusBadRequest, "No location selected", "Please select a location.")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Invalid form", "Could not parse form data.")
		return
	}

	dateStr := r.FormValue("scheduled_date")
	notes := r.FormValue("notes")

	if dateStr == "" {
		h.renderSessionFormError(w, r, "Scheduled date is required.", SessionFormValues{
			ScheduledDate: dateStr,
			Notes:         notes,
		})
		return
	}

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		h.renderSessionFormError(w, r, "Invalid date format. Use YYYY-MM-DD.", SessionFormValues{
			ScheduledDate: dateStr,
			Notes:         notes,
		})
		return
	}

	session := &model.SettingSession{
		LocationID:    locationID,
		ScheduledDate: date,
		CreatedBy:     user.ID,
	}
	if notes != "" {
		session.Notes = &notes
	}

	if err := h.sessionRepo.Create(ctx, session); err != nil {
		slog.Error("session create failed", "error", err)
		h.renderSessionFormError(w, r, "Could not create session. Please try again.", SessionFormValues{
			ScheduledDate: dateStr,
			Notes:         notes,
		})
		return
	}

	http.Redirect(w, r, "/sessions/"+session.ID, http.StatusSeeOther)
}

// SessionEdit renders the edit session form (GET /sessions/{sessionID}/edit).
func (h *Handler) SessionEdit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := chi.URLParam(r, "sessionID")

	session, err := h.sessionRepo.GetByID(ctx, sessionID)
	if err != nil || session == nil {
		h.renderError(w, r, http.StatusNotFound, "Session not found", "That session doesn't exist.")
		return
	}

	if !h.checkLocationOwnership(w, r, session.LocationID) {
		return
	}

	data := &PageData{
		TemplateData: templateDataFromContext(r, "sessions"),
		Session:      session,
		SessionFormValues: SessionFormValues{
			ScheduledDate: session.ScheduledDate.Format("2006-01-02"),
			Notes:         derefString(session.Notes),
		},
	}

	h.render(w, r, "setter/session-form.html", data)
}

// SessionUpdate handles the form POST to update a session (POST /sessions/{sessionID}/edit).
func (h *Handler) SessionUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := chi.URLParam(r, "sessionID")

	session, err := h.sessionRepo.GetByID(ctx, sessionID)
	if err != nil || session == nil {
		h.renderError(w, r, http.StatusNotFound, "Session not found", "That session doesn't exist.")
		return
	}

	if !h.checkLocationOwnership(w, r, session.LocationID) {
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Invalid form", "Could not parse form data.")
		return
	}

	dateStr := r.FormValue("scheduled_date")
	notes := r.FormValue("notes")

	if dateStr == "" {
		h.renderSessionFormError(w, r, "Scheduled date is required.", SessionFormValues{
			ScheduledDate: dateStr,
			Notes:         notes,
		})
		return
	}

	date, parseErr := time.Parse("2006-01-02", dateStr)
	if parseErr != nil {
		h.renderSessionFormError(w, r, "Invalid date format.", SessionFormValues{
			ScheduledDate: dateStr,
			Notes:         notes,
		})
		return
	}

	session.ScheduledDate = date
	if notes != "" {
		session.Notes = &notes
	} else {
		session.Notes = nil
	}

	if err := h.sessionRepo.Update(ctx, session); err != nil {
		slog.Error("session update failed", "error", err)
		h.renderSessionFormError(w, r, "Could not update session.", SessionFormValues{
			ScheduledDate: dateStr,
			Notes:         notes,
		})
		return
	}

	http.Redirect(w, r, "/sessions/"+sessionID, http.StatusSeeOther)
}

func (h *Handler) renderSessionFormError(w http.ResponseWriter, r *http.Request, msg string, values SessionFormValues) {
	data := &PageData{
		TemplateData:      templateDataFromContext(r, "sessions"),
		SessionFormValues: values,
		SessionFormError:  msg,
	}
	h.render(w, r, "setter/session-form.html", data)
}
