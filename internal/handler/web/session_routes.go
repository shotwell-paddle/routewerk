package webhandler

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

// ── Session Route Fields (HTMX partial) ─────────────────────

// SessionRouteFields returns the route form fields partial based on wall type and route type selection.
// GET /sessions/{sessionID}/route-fields?wall_id=...&route_type=...
func (h *Handler) SessionRouteFields(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := chi.URLParam(r, "sessionID")
	locationID := middleware.GetWebLocationID(ctx)
	wallID := r.URL.Query().Get("wall_id")
	routeType := r.URL.Query().Get("route_type")

	// Load wall to get type
	var wallType string
	if wallID != "" {
		wall, err := h.wallRepo.GetByID(ctx, wallID)
		if err == nil && wall != nil {
			wallType = wall.WallType
		}
	}

	// Load settings for grading method + circuit colors
	locSettings, err := h.settingsRepo.GetLocationSettings(ctx, locationID)
	if err != nil {
		locSettings = model.DefaultLocationSettings()
	}

	// Load setters for the setter dropdown
	setters, err := h.userRepo.ListSettersByLocation(ctx, locationID)
	if err != nil {
		slog.Error("load setters for session route fields failed", "location_id", locationID, "error", err)
	}

	rf := buildRouteFields(sessionID, wallID, wallType, locSettings.Grading.BoulderMethod, routeType, locSettings, setters)

	tmpl, ok := h.templates["setter/session-detail.html"]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "route-fields", rf); err != nil {
		slog.Error("render route-fields partial failed", "error", err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

// ── Session Route Management ─────────────────────────────────

// SessionAddRoute creates a new draft route linked to the session (POST /sessions/{sessionID}/routes).
func (h *Handler) SessionAddRoute(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := chi.URLParam(r, "sessionID")
	locationID := middleware.GetWebLocationID(ctx)

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
	setterID := r.FormValue("setter_id")
	routeType := r.FormValue("route_type")
	grade := r.FormValue("grade")
	color := r.FormValue("color")
	name := r.FormValue("name")
	circuitVGrade := r.FormValue("circuit_vgrade")

	if wallID == "" || grade == "" || color == "" || routeType == "" {
		http.Redirect(w, r, "/sessions/"+sessionID, http.StatusSeeOther)
		return
	}

	// Determine grading system from route type
	gradingSystem := "v_scale"
	actualRouteType := routeType
	var circuitColor *string
	if routeType == "route" {
		gradingSystem = "yds"
	} else if routeType == "boulder_circuit" {
		gradingSystem = "circuit"
		actualRouteType = "boulder"
		// grade field holds the circuit color name; optional V-grade in circuitVGrade
		cc := grade
		circuitColor = &cc
		if circuitVGrade != "" {
			grade = circuitVGrade
		} else {
			grade = cc // fallback: use circuit color as the grade display
		}
	}

	route := &model.Route{
		LocationID:    locationID,
		WallID:        wallID,
		RouteType:     actualRouteType,
		Status:        "draft",
		GradingSystem: gradingSystem,
		Grade:         grade,
		CircuitColor:  circuitColor,
		Color:         color,
		DateSet:       session.ScheduledDate,
		SessionID:     &sessionID,
	}
	if setterID != "" {
		route.SetterID = &setterID
	}
	if name != "" {
		route.Name = &name
	}

	if err := h.routeRepo.Create(ctx, route); err != nil {
		slog.Error("add session route failed", "error", err)
	}

	// Auto-add setter to session assignments if not already present,
	// and assign the wall if the setter has a wall-less assignment.
	if setterID != "" {
		assignments, _ := h.sessionRepo.GetAssignments(ctx, sessionID)
		var existing *model.SettingSessionAssignment
		for i, a := range assignments {
			if a.SetterID == setterID {
				existing = &assignments[i]
				break
			}
		}
		if existing == nil {
			// Setter not yet assigned — add with wall
			assignment := &model.SettingSessionAssignment{
				SessionID: sessionID,
				SetterID:  setterID,
				WallID:    &route.WallID,
			}
			if err := h.sessionRepo.AddAssignment(ctx, assignment); err != nil {
				slog.Error("auto-add setter assignment failed", "setter_id", setterID, "error", err)
			}
		} else if existing.WallID == nil {
			// Setter assigned but no wall yet — fill in the wall
			if err := h.sessionRepo.UpdateAssignmentWall(ctx, existing.ID, route.WallID); err != nil {
				slog.Error("auto-set assignment wall failed", "assignment_id", existing.ID, "error", err)
			}
		}
	}

	http.Redirect(w, r, "/sessions/"+sessionID, http.StatusSeeOther)
}

// SessionEditRoute updates a draft route in the session (POST /sessions/{sessionID}/routes/{routeID}/edit).
func (h *Handler) SessionEditRoute(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := chi.URLParam(r, "sessionID")
	routeID := chi.URLParam(r, "routeID")

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

	// Load the existing route
	route, err := h.routeRepo.GetByID(ctx, routeID)
	if err != nil || route == nil {
		h.renderError(w, r, http.StatusNotFound, "Route not found", "That route doesn't exist.")
		return
	}

	// Update fields
	if v := r.FormValue("grade"); v != "" {
		route.Grade = v
	}
	if v := r.FormValue("color"); v != "" {
		route.Color = v
	}
	if v := r.FormValue("wall_id"); v != "" {
		route.WallID = v
	}
	if v := r.FormValue("setter_id"); v != "" {
		route.SetterID = &v
	}
	if v := r.FormValue("route_type"); v != "" {
		if v == "boulder_circuit" {
			route.RouteType = "boulder"
			route.GradingSystem = "circuit"
			cc := r.FormValue("grade")
			route.CircuitColor = &cc
			if cv := r.FormValue("circuit_vgrade"); cv != "" {
				route.Grade = cv
			}
		} else {
			route.RouteType = v
			route.CircuitColor = nil
			if v == "boulder" {
				route.GradingSystem = "v_scale"
			} else {
				route.GradingSystem = "yds"
			}
		}
	}
	if v := r.FormValue("name"); v != "" {
		route.Name = &v
	} else {
		route.Name = nil
	}

	if err := h.routeRepo.Update(ctx, route); err != nil {
		slog.Error("edit session route failed", "error", err)
	}

	// Redirect back to the referring page
	referer := r.Header.Get("Referer")
	if referer == "" {
		referer = "/sessions/" + sessionID
	}
	http.Redirect(w, r, referer, http.StatusSeeOther)
}

// SessionDeleteRoute removes a draft route from the session (POST /sessions/{sessionID}/routes/{routeID}/delete).
func (h *Handler) SessionDeleteRoute(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := chi.URLParam(r, "sessionID")
	routeID := chi.URLParam(r, "routeID")

	session, sErr := h.sessionRepo.GetByID(ctx, sessionID)
	if sErr != nil || session == nil {
		h.renderError(w, r, http.StatusNotFound, "Session not found", "That session doesn't exist.")
		return
	}
	if !h.checkLocationOwnership(w, r, session.LocationID) {
		return
	}

	if err := h.sessionRepo.DeleteSessionRoute(ctx, sessionID, routeID); err != nil {
		slog.Error("delete session route failed", "error", err)
	}

	referer := r.Header.Get("Referer")
	if referer == "" {
		referer = "/sessions/" + sessionID
	}
	http.Redirect(w, r, referer, http.StatusSeeOther)
}
