package webhandler

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

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

// SessionToggleChecklist toggles a checklist item
// (POST /sessions/{sessionID}/checklist/{itemID}/toggle).
//
// HTMX requests get back just the playbook-checklist partial so the swap
// is local and keeps scroll/focus state. Non-HTMX requests redirect to
// the session page so a manual curl or noscript fallback still works.
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

	rowsAffected, err := h.sessionRepo.ToggleChecklistItem(ctx, itemID, userID)
	if err != nil {
		slog.Error("toggle checklist item failed", "error", err, "session_id", sessionID, "item_id", itemID)
	} else if rowsAffected == 0 {
		slog.Warn("toggle checklist item matched no rows", "session_id", sessionID, "item_id", itemID)
	}

	if r.Header.Get("HX-Request") != "true" {
		http.Redirect(w, r, "/sessions/"+sessionID, http.StatusSeeOther)
		return
	}

	items, err := h.sessionRepo.ListChecklistItems(ctx, sessionID)
	if err != nil {
		slog.Error("reload checklist failed", "error", err)
		http.Error(w, "Reload failed", http.StatusInternalServerError)
		return
	}

	tmpl, ok := h.templates["setter/session-detail.html"]
	if !ok {
		slog.Error("session-detail template not loaded")
		http.Error(w, "Template missing", http.StatusInternalServerError)
		return
	}

	data := &PageData{
		TemplateData: TemplateData{
			CSRFToken: middleware.TokenFromRequest(r),
		},
		ChecklistItems: items,
	}
	// Override the client's hx-target/hx-swap so a long-lived tab whose
	// page was rendered before the toggle-fix landed (where the rows still
	// declare hx-target="#main-content") doesn't splat the partial across
	// the whole page. New pages already declare these values; setting them
	// here makes the response idempotent regardless of caller markup.
	w.Header().Set("HX-Retarget", "#session-playbook-checklist")
	w.Header().Set("HX-Reswap", "outerHTML")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "playbook-checklist", data); err != nil {
		slog.Error("render checklist partial failed", "error", err)
	}
}

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
