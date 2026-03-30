package webhandler

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

// ── Session List ─────────────────────────────────────────────

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

// ── Session Detail ───────────────────────────────────────────

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

// ── Create Session ───────────────────────────────────────────

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

// ── Edit Session ─────────────────────────────────────────────

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

// ── Add Assignment ───────────────────────────────────────────

// SessionAddAssignment handles adding a setter to a session (POST /sessions/{sessionID}/assign).
func (h *Handler) SessionAddAssignment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := chi.URLParam(r, "sessionID")

	// Verify session belongs to current location
	session, sErr := h.sessionRepo.GetByID(ctx, sessionID)
	if sErr != nil || session == nil {
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

	setterID := r.FormValue("setter_id")
	wallID := r.FormValue("wall_id")
	notes := r.FormValue("notes")

	if setterID == "" {
		// Redirect back with no action
		http.Redirect(w, r, "/sessions/"+sessionID, http.StatusSeeOther)
		return
	}

	assignment := &model.SettingSessionAssignment{
		SessionID: sessionID,
		SetterID:  setterID,
	}
	if wallID != "" {
		assignment.WallID = &wallID
	}
	if notes != "" {
		assignment.Notes = &notes
	}

	if err := h.sessionRepo.AddAssignment(ctx, assignment); err != nil {
		slog.Error("add assignment failed", "error", err)
	}

	http.Redirect(w, r, "/sessions/"+sessionID, http.StatusSeeOther)
}

// SessionRemoveAssignment removes a setter from a session (POST /sessions/{sessionID}/unassign/{assignmentID}).
func (h *Handler) SessionRemoveAssignment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := chi.URLParam(r, "sessionID")
	assignmentID := chi.URLParam(r, "assignmentID")

	// Verify session belongs to current location
	session, sErr := h.sessionRepo.GetByID(ctx, sessionID)
	if sErr != nil || session == nil {
		h.renderError(w, r, http.StatusNotFound, "Session not found", "That session doesn't exist.")
		return
	}
	if !h.checkLocationOwnership(w, r, session.LocationID) {
		return
	}

	if err := h.sessionRepo.RemoveAssignment(ctx, assignmentID); err != nil {
		slog.Error("remove assignment failed", "error", err)
	}

	http.Redirect(w, r, "/sessions/"+sessionID, http.StatusSeeOther)
}

// ── Strip Targets ────────────────────────────────────────────

// SessionAddStripTarget adds a wall or route to strip (POST /sessions/{sessionID}/strip).
func (h *Handler) SessionAddStripTarget(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := chi.URLParam(r, "sessionID")

	session, sErr := h.sessionRepo.GetByID(ctx, sessionID)
	if sErr != nil || session == nil {
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

	wallID := r.FormValue("wall_id")
	routeID := r.FormValue("route_id")

	if wallID == "" {
		http.Redirect(w, r, "/sessions/"+sessionID, http.StatusSeeOther)
		return
	}

	target := &model.SessionStripTarget{
		SessionID: sessionID,
		WallID:    wallID,
	}
	if routeID != "" {
		target.RouteID = &routeID
	}

	if err := h.sessionRepo.AddStripTarget(ctx, target); err != nil {
		slog.Error("add strip target failed", "error", err)
	}

	http.Redirect(w, r, "/sessions/"+sessionID, http.StatusSeeOther)
}

// SessionRemoveStripTarget removes a strip target (POST /sessions/{sessionID}/strip/{targetID}/delete).
func (h *Handler) SessionRemoveStripTarget(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := chi.URLParam(r, "sessionID")
	targetID := chi.URLParam(r, "targetID")

	session, sErr := h.sessionRepo.GetByID(ctx, sessionID)
	if sErr != nil || session == nil {
		h.renderError(w, r, http.StatusNotFound, "Session not found", "That session doesn't exist.")
		return
	}
	if !h.checkLocationOwnership(w, r, session.LocationID) {
		return
	}

	if err := h.sessionRepo.RemoveStripTarget(ctx, targetID); err != nil {
		slog.Error("remove strip target failed", "error", err)
	}

	http.Redirect(w, r, "/sessions/"+sessionID, http.StatusSeeOther)
}

// ── Checklist ────────────────────────────────────────────────

// SessionToggleChecklist toggles a checklist item (POST /sessions/{sessionID}/checklist/{itemID}/toggle).
func (h *Handler) SessionToggleChecklist(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := chi.URLParam(r, "sessionID")
	itemID := chi.URLParam(r, "itemID")
	user := middleware.GetWebUser(ctx)

	session, sErr := h.sessionRepo.GetByID(ctx, sessionID)
	if sErr != nil || session == nil {
		h.renderError(w, r, http.StatusNotFound, "Session not found", "That session doesn't exist.")
		return
	}
	if !h.checkLocationOwnership(w, r, session.LocationID) {
		return
	}

	userID := ""
	if user != nil {
		userID = user.ID
	}

	if err := h.sessionRepo.ToggleChecklistItem(ctx, itemID, userID); err != nil {
		slog.Error("toggle checklist item failed", "error", err)
	}

	http.Redirect(w, r, "/sessions/"+sessionID, http.StatusSeeOther)
}

// ── Session Lifecycle ────────────────────────────────────────

// SessionDelete removes a session and its draft routes (POST /sessions/{sessionID}/delete).
func (h *Handler) SessionDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := chi.URLParam(r, "sessionID")

	session, sErr := h.sessionRepo.GetByID(ctx, sessionID)
	if sErr != nil || session == nil {
		h.renderError(w, r, http.StatusNotFound, "Session not found", "That session doesn't exist.")
		return
	}
	if !h.checkLocationOwnership(w, r, session.LocationID) {
		return
	}

	if err := h.sessionRepo.Delete(ctx, sessionID); err != nil {
		slog.Error("delete session failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Delete failed", "Could not delete session.")
		return
	}

	http.Redirect(w, r, "/sessions", http.StatusSeeOther)
}

// SessionComplete renders the review/publish page (GET /sessions/{sessionID}/complete).
func (h *Handler) SessionComplete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := chi.URLParam(r, "sessionID")
	locationID := middleware.GetWebLocationID(ctx)

	session, assignments, err := h.sessionRepo.GetByIDWithDetails(ctx, sessionID)
	if err != nil || session == nil {
		h.renderError(w, r, http.StatusNotFound, "Session not found", "That session doesn't exist.")
		return
	}
	if !h.checkLocationOwnership(w, r, session.LocationID) {
		return
	}

	sessionRoutes, err := h.sessionRepo.ListSessionRoutes(ctx, sessionID)
	if err != nil {
		slog.Error("list session routes for complete failed", "error", err)
	}

	walls, err := h.wallRepo.ListByLocation(ctx, locationID)
	if err != nil {
		slog.Error("load walls for session complete failed", "location_id", locationID, "error", err)
	}
	setters, err := h.userRepo.ListSettersByLocation(ctx, locationID)
	if err != nil {
		slog.Error("load setters for session complete failed", "location_id", locationID, "error", err)
	}

	locSettings, err := h.settingsRepo.GetLocationSettings(ctx, locationID)
	if err != nil {
		slog.Error("load location settings for session complete failed", "location_id", locationID, "error", err)
		locSettings = model.DefaultLocationSettings()
	}

	data := &PageData{
		TemplateData:       templateDataFromContext(r, "sessions"),
		Session:            session,
		SessionAssignments: assignments,
		SessionRoutes:      sessionRoutes,
		FormWalls:          walls,
		Setters:            setters,
		HoldColors:         holdColorsFromSettings(locSettings),
		VScaleGrades:       vScaleGrades,
		YDSGrades:          ydsGrades,
		CircuitColors:      circuitColors,
	}

	h.render(w, r, "setter/session-complete.html", data)
}

// SessionPublish publishes all draft routes and marks session complete (POST /sessions/{sessionID}/publish).
func (h *Handler) SessionPublish(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := chi.URLParam(r, "sessionID")

	session, sErr := h.sessionRepo.GetByID(ctx, sessionID)
	if sErr != nil || session == nil {
		h.renderError(w, r, http.StatusNotFound, "Session not found", "That session doesn't exist.")
		return
	}
	if !h.checkLocationOwnership(w, r, session.LocationID) {
		return
	}

	// Archive routes on the stripping list BEFORE publishing new routes,
	// so that whole-wall strips don't catch the freshly published ones.
	stripTargets, err := h.sessionRepo.ListStripTargets(ctx, sessionID)
	if err != nil {
		slog.Error("load strip targets failed", "error", err)
	} else {
		var routeIDs []string
		for _, t := range stripTargets {
			if t.RouteID == nil {
				// Whole-wall strip — archive all active routes on this wall
				n, archErr := h.routeRepo.BulkArchiveByWall(ctx, t.WallID)
				if archErr != nil {
					slog.Error("archive wall routes failed", "wall_id", t.WallID, "error", archErr)
				} else if n > 0 {
					slog.Info("archived wall routes", "session_id", sessionID, "wall_id", t.WallID, "count", n)
				}
			} else {
				// Individual route strip
				routeIDs = append(routeIDs, *t.RouteID)
			}
		}
		if len(routeIDs) > 0 {
			n, archErr := h.routeRepo.BulkArchive(ctx, routeIDs)
			if archErr != nil {
				slog.Error("archive stripped routes failed", "error", archErr)
			} else if n > 0 {
				slog.Info("archived stripped routes", "session_id", sessionID, "count", n)
			}
		}
	}

	// Publish all draft routes (after stripping, so new routes stay active)
	if _, err := h.sessionRepo.PublishSessionRoutes(ctx, sessionID); err != nil {
		slog.Error("publish session routes failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Publish failed", "Could not publish routes.")
		return
	}

	// Mark session complete
	if err := h.sessionRepo.UpdateStatus(ctx, sessionID, "complete"); err != nil {
		slog.Error("update session status failed", "error", err)
	}

	// Redirect to photos page so the setter can upload route photos.
	// Fall back to session detail if storage isn't configured.
	if h.storageService.IsConfigured() {
		http.Redirect(w, r, "/sessions/"+sessionID+"/photos", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/sessions/"+sessionID, http.StatusSeeOther)
}

// SessionReopen sets a completed session back to in_progress (POST /sessions/{sessionID}/reopen).
// Requires head_setter role or above.
func (h *Handler) SessionReopen(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := chi.URLParam(r, "sessionID")

	session, sErr := h.sessionRepo.GetByID(ctx, sessionID)
	if sErr != nil || session == nil {
		h.renderError(w, r, http.StatusNotFound, "Session not found", "That session doesn't exist.")
		return
	}
	if !h.checkLocationOwnership(w, r, session.LocationID) {
		return
	}

	// Only head_setter+ can reopen
	role := middleware.GetWebRole(ctx)
	if middleware.RoleRankValue(role) < 3 {
		h.renderError(w, r, http.StatusForbidden, "Not allowed", "Only head setters and above can reopen sessions.")
		return
	}

	if err := h.sessionRepo.UpdateStatus(ctx, sessionID, "in_progress"); err != nil {
		slog.Error("reopen session failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Reopen failed", "Could not reopen session.")
		return
	}

	http.Redirect(w, r, "/sessions/"+sessionID, http.StatusSeeOther)
}

// Session route management handlers are in session_routes.go

// ── Session Photos ──────────────────────────────────────────

// SessionPhotos renders the photo upload page for a completed session (GET /sessions/{sessionID}/photos).
func (h *Handler) SessionPhotos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := chi.URLParam(r, "sessionID")

	session, sErr := h.sessionRepo.GetByID(ctx, sessionID)
	if sErr != nil || session == nil {
		h.renderError(w, r, http.StatusNotFound, "Session not found", "That session doesn't exist.")
		return
	}
	if !h.checkLocationOwnership(w, r, session.LocationID) {
		return
	}

	// Load routes from this session
	sessionRoutes, err := h.sessionRepo.ListSessionRoutes(ctx, sessionID)
	if err != nil {
		slog.Error("load session routes for photos failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Could not load session routes.")
		return
	}

	// Count how many already have photos
	photosUploaded := 0
	for _, rt := range sessionRoutes {
		if rt.PhotoURL != nil {
			photosUploaded++
		}
	}
	photosPercent := 0
	if len(sessionRoutes) > 0 {
		photosPercent = photosUploaded * 100 / len(sessionRoutes)
	}

	data := &PageData{
		TemplateData:   templateDataFromContext(r, "sessions"),
		Session:        session,
		SessionRoutes:  sessionRoutes,
		PhotosEnabled:  h.storageService.IsConfigured(),
		PhotosUploaded: photosUploaded,
		PhotosPercent:  photosPercent,
	}

	h.render(w, r, "setter/session-photos.html", data)
}

// ── Helper ───────────────────────────────────────────────────

func (h *Handler) renderSessionFormError(w http.ResponseWriter, r *http.Request, msg string, values SessionFormValues) {
	data := &PageData{
		TemplateData:      templateDataFromContext(r, "sessions"),
		SessionFormValues: values,
		SessionFormError:  msg,
	}
	h.render(w, r, "setter/session-form.html", data)
}
