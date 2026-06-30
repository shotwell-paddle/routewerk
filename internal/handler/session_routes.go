package handler

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

// Session draft-route management. These endpoints let the session builder
// (the SvelteKit /sessions/[id] page) create the climbs being set in a
// session as *draft* routes bound to the session. Publish later flips them
// to active in one shot. Mirrors the original HTMX flow at
// internal/handler/web/session_routes.go::SessionAddRoute, which was
// orphaned when the SPA was promoted to root (#98) — leaving the SPA
// builder with no way to add routes at all.
//
// head_setter+ is enforced by the router; cross-tenant access is guarded
// by resolveSession (defined in session.go).

// sessionRouteRequest is the write shape for adding/editing a session
// draft route. It deliberately mirrors the SPA RouteForm's RouteWriteShape
// (wall_id, route_type, grading_system, grade, color, circuit_color, …)
// plus an optional setter_id, so a route created here is stored identically
// to one created via the normal route form — only status + session_id
// differ.
type sessionRouteRequest struct {
	WallID        string  `json:"wall_id"`
	RouteType     string  `json:"route_type"`
	GradingSystem string  `json:"grading_system"`
	Grade         string  `json:"grade"`
	Color         string  `json:"color"`
	CircuitColor  *string `json:"circuit_color,omitempty"`
	GradeLow      *string `json:"grade_low,omitempty"`
	GradeHigh     *string `json:"grade_high,omitempty"`
	Name          *string `json:"name,omitempty"`
	Description   *string `json:"description,omitempty"`
	SetterID      *string `json:"setter_id,omitempty"`
}

// normalizeOptional trims an optional string pointer, collapsing nil or
// whitespace-only values to nil so we never persist empty-string foreign
// keys (e.g. setter_id = "") or blank names.
func normalizeOptional(s *string) *string {
	if s == nil {
		return nil
	}
	if strings.TrimSpace(*s) == "" {
		return nil
	}
	return s
}

// buildSessionRoute validates a session-route request and maps it to a
// draft route bound to the given session. Pure (no I/O) so it can be
// table-tested without a database. On invalid input it returns a non-empty
// message suitable for a 400 response.
func buildSessionRoute(req sessionRouteRequest, session *model.SettingSession) (*model.Route, string) {
	if req.WallID == "" || req.RouteType == "" || req.GradingSystem == "" || req.Grade == "" || req.Color == "" {
		return nil, "wall_id, route_type, grading_system, grade, and color are required"
	}

	return &model.Route{
		LocationID:    session.LocationID,
		WallID:        req.WallID,
		SetterID:      normalizeOptional(req.SetterID),
		RouteType:     req.RouteType,
		Status:        "draft",
		GradingSystem: req.GradingSystem,
		Grade:         req.Grade,
		GradeLow:      normalizeOptional(req.GradeLow),
		GradeHigh:     normalizeOptional(req.GradeHigh),
		CircuitColor:  normalizeOptional(req.CircuitColor),
		Name:          normalizeOptional(req.Name),
		Color:         req.Color,
		Description:   normalizeOptional(req.Description),
		// Session drafts inherit the session's scheduled date so the
		// route's "date set" reflects when it actually goes up.
		DateSet:   session.ScheduledDate,
		SessionID: &session.ID,
	}, ""
}

// AddRoute — POST /sessions/{sessionID}/routes. Creates a draft route
// linked to the session. head_setter+ enforced by router middleware.
func (h *SessionHandler) AddRoute(w http.ResponseWriter, r *http.Request) {
	session, ok := h.resolveSession(w, r)
	if !ok {
		return
	}
	// A completed session's drafts have already been published; adding a
	// new draft now would create an orphan that never activates. Make the
	// caller reopen first.
	if session.Status == "complete" {
		Error(w, http.StatusConflict, "session is complete; reopen it before adding routes")
		return
	}

	var req sessionRouteRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	route, msg := buildSessionRoute(req, session)
	if msg != "" {
		Error(w, http.StatusBadRequest, msg)
		return
	}

	if err := h.routes.Create(r.Context(), route); err != nil {
		Error(w, http.StatusInternalServerError, "failed to add route")
		return
	}

	JSON(w, http.StatusCreated, route)
}

// EditRoute — PUT /sessions/{sessionID}/routes/{routeID}. Full-replace of
// the editable fields on a draft route that belongs to this session.
// Identity, draft status, session link, and date are preserved.
func (h *SessionHandler) EditRoute(w http.ResponseWriter, r *http.Request) {
	session, ok := h.resolveSession(w, r)
	if !ok {
		return
	}
	routeID := chi.URLParam(r, "routeID")

	// Load via the session-scoped query: it filters on session_id +
	// status='draft' at the DB level. (RouteRepo.GetByID can't be used here
	// — it doesn't select session_id, so its SessionID is always nil.)
	route, err := h.sessions.GetSessionDraftRoute(r.Context(), session.ID, routeID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if route == nil {
		Error(w, http.StatusNotFound, "draft route not found in this session")
		return
	}

	var req sessionRouteRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	next, msg := buildSessionRoute(req, session)
	if msg != "" {
		Error(w, http.StatusBadRequest, msg)
		return
	}

	// Copy editable fields onto the loaded route, preserving id, status,
	// session link, date_set, photo, and strip date.
	route.WallID = next.WallID
	route.SetterID = next.SetterID
	route.RouteType = next.RouteType
	route.GradingSystem = next.GradingSystem
	route.Grade = next.Grade
	route.GradeLow = next.GradeLow
	route.GradeHigh = next.GradeHigh
	route.CircuitColor = next.CircuitColor
	route.Name = next.Name
	route.Color = next.Color
	route.Description = next.Description

	if err := h.routes.Update(r.Context(), route); err != nil {
		Error(w, http.StatusInternalServerError, "failed to update route")
		return
	}

	JSON(w, http.StatusOK, route)
}

// DeleteRoute — DELETE /sessions/{sessionID}/routes/{routeID}. Hard-deletes
// a draft route (the repository guards on status='draft' + session match,
// so a published route can't be removed through here).
func (h *SessionHandler) DeleteRoute(w http.ResponseWriter, r *http.Request) {
	session, ok := h.resolveSession(w, r)
	if !ok {
		return
	}
	routeID := chi.URLParam(r, "routeID")

	n, err := h.sessions.DeleteSessionRoute(r.Context(), session.ID, routeID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "failed to delete route")
		return
	}
	if n == 0 {
		// No draft matched: wrong id, already published, or another session.
		Error(w, http.StatusNotFound, "draft route not found in this session")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
