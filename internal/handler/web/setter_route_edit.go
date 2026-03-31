package webhandler

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
)

// RouteEdit renders the route edit form (GET /routes/{routeID}/edit).
func (h *Handler) RouteEdit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	routeID := chi.URLParam(r, "routeID")

	if !validRouteID.MatchString(routeID) {
		h.renderError(w, r, http.StatusBadRequest, "Invalid route", "The route ID is not valid.")
		return
	}

	rt, err := h.routeRepo.GetByID(ctx, routeID)
	if err != nil || rt == nil {
		h.renderError(w, r, http.StatusNotFound, "Route not found", "This route doesn't exist.")
		return
	}

	if !h.checkLocationOwnership(w, r, rt.LocationID) {
		return
	}

	fv := RouteFormValues{
		WallID:        rt.WallID,
		RouteType:     rt.RouteType,
		GradingSystem: rt.GradingSystem,
		Grade:         rt.Grade,
		Color:         rt.Color,
		DateSet:       rt.DateSet.Format("2006-01-02"),
		TagIDs:        make(map[string]bool),
	}
	if rt.SetterID != nil {
		fv.SetterID = *rt.SetterID
	}
	if rt.CircuitColor != nil {
		fv.CircuitColor = *rt.CircuitColor
	}
	if rt.Name != nil {
		fv.Name = *rt.Name
	}
	if rt.Description != nil {
		fv.Description = *rt.Description
	}
	if rt.ProjectedStripDate != nil {
		fv.ProjectedStripDate = rt.ProjectedStripDate.Format("2006-01-02")
	}
	for _, tag := range rt.Tags {
		fv.TagIDs[tag.ID] = true
	}

	wallName := ""
	if wall, wErr := h.wallRepo.GetByID(ctx, rt.WallID); wErr == nil && wall != nil {
		wallName = wall.Name
	}
	setterName := "Unknown"
	if rt.SetterID != nil {
		if setter, uErr := h.userRepo.GetByID(ctx, *rt.SetterID); uErr == nil && setter != nil {
			setterName = setter.DisplayName
		}
	}

	rv := &RouteView{
		Route:      *rt,
		WallName:   wallName,
		SetterName: setterName,
	}

	h.renderRouteForm(w, r, locationID, rv, fv, "")
}

// RouteUpdate processes the route edit form (POST /routes/{routeID}/edit).
func (h *Handler) RouteUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	routeID := chi.URLParam(r, "routeID")

	if !validRouteID.MatchString(routeID) {
		h.renderError(w, r, http.StatusBadRequest, "Invalid route", "The route ID is not valid.")
		return
	}

	rt, err := h.routeRepo.GetByID(ctx, routeID)
	if err != nil || rt == nil {
		h.renderError(w, r, http.StatusNotFound, "Route not found", "This route doesn't exist.")
		return
	}

	if !h.checkLocationOwnership(w, r, rt.LocationID) {
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Invalid form", "Could not parse form data.")
		return
	}

	fv := parseRouteForm(r)

	// Apply form values to existing route
	if fv.WallID != "" {
		rt.WallID = fv.WallID
	}
	if fv.RouteType != "" {
		rt.RouteType = fv.RouteType
	}
	if fv.GradingSystem != "" {
		rt.GradingSystem = fv.GradingSystem
	}
	if fv.Grade != "" {
		rt.Grade = fv.Grade
	}
	if fv.Color != "" {
		rt.Color = fv.Color
	}

	if fv.SetterID != "" {
		rt.SetterID = &fv.SetterID
	}

	if fv.CircuitColor != "" {
		rt.CircuitColor = &fv.CircuitColor
	} else {
		rt.CircuitColor = nil
	}

	if fv.Name != "" {
		rt.Name = &fv.Name
	} else {
		rt.Name = nil
	}

	if fv.Description != "" {
		rt.Description = &fv.Description
	} else {
		rt.Description = nil
	}

	if fv.DateSet != "" {
		if parsed, pErr := time.Parse("2006-01-02", fv.DateSet); pErr == nil {
			rt.DateSet = parsed
		}
	}

	if fv.ProjectedStripDate != "" {
		if parsed, pErr := time.Parse("2006-01-02", fv.ProjectedStripDate); pErr == nil {
			rt.ProjectedStripDate = &parsed
		}
	} else {
		rt.ProjectedStripDate = nil
	}

	if updateErr := h.routeRepo.Update(ctx, rt); updateErr != nil {
		slog.Error("route update failed", "route_id", routeID, "error", updateErr)

		wallName := ""
		if wall, wErr := h.wallRepo.GetByID(ctx, rt.WallID); wErr == nil && wall != nil {
			wallName = wall.Name
		}
		setterName := "Unknown"
		if rt.SetterID != nil {
			if setter, uErr := h.userRepo.GetByID(ctx, *rt.SetterID); uErr == nil && setter != nil {
				setterName = setter.DisplayName
			}
		}
		rv := &RouteView{Route: *rt, WallName: wallName, SetterName: setterName}
		h.renderRouteForm(w, r, locationID, rv, fv, "Failed to update route. Please try again.")
		return
	}

	// Update tags
	var tagIDs []string
	for id := range fv.TagIDs {
		tagIDs = append(tagIDs, id)
	}
	if err := h.routeRepo.SetTags(ctx, rt.ID, tagIDs); err != nil {
		slog.Error("route set tags failed", "route_id", rt.ID, "error", err)
	}

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/routes/manage")
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, "/routes/manage", http.StatusSeeOther)
}

// RouteStatusUpdate handles POST /routes/{routeID}/status.
// For HTMX requests it returns the updated row partial for an instant
// inline swap (no full-page reload). For regular requests it redirects.
func (h *Handler) RouteStatusUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	routeID := chi.URLParam(r, "routeID")

	if !validRouteID.MatchString(routeID) {
		http.Error(w, "invalid route ID", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	newStatus := r.FormValue("status")
	if newStatus != "active" && newStatus != "flagged" && newStatus != "archived" {
		http.Error(w, "invalid status", http.StatusBadRequest)
		return
	}

	// Verify route belongs to current location before updating
	rt, rtErr := h.routeRepo.GetByID(ctx, routeID)
	if rtErr != nil || rt == nil {
		http.Error(w, "route not found", http.StatusNotFound)
		return
	}
	if !h.checkLocationOwnership(w, r, rt.LocationID) {
		return
	}

	if err := h.routeRepo.UpdateStatus(ctx, routeID, newStatus); err != nil {
		slog.Error("route status update failed", "route_id", routeID, "error", err)
		http.Error(w, "failed to update status", http.StatusInternalServerError)
		return
	}

	// For HTMX: render just the updated row for inline swap
	if r.Header.Get("HX-Request") == "true" {
		rt, err := h.routeRepo.GetByID(ctx, routeID)
		if err != nil || rt == nil {
			http.Error(w, "route not found", http.StatusNotFound)
			return
		}

		wallName := ""
		if wall, wErr := h.wallRepo.GetByID(ctx, rt.WallID); wErr == nil && wall != nil {
			wallName = wall.Name
		}
		setterName := "Unknown"
		if rt.SetterID != nil {
			if setter, uErr := h.userRepo.GetByID(ctx, *rt.SetterID); uErr == nil && setter != nil {
				setterName = setter.DisplayName
			}
		}

		rv := &RouteView{Route: *rt, WallName: wallName, SetterName: setterName}

		tmpl, ok := h.templates["setter/route-manage.html"]
		if !ok {
			http.Error(w, "template error", http.StatusInternalServerError)
			return
		}

		// The "route-manage-row" template expects PageData with .RowRoute
		// and .CSRFToken so it can render action buttons with valid tokens.
		data := &PageData{
			TemplateData: templateDataFromContext(r, "manage-routes"),
			RowRoute:     rv,
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.ExecuteTemplate(w, "route-manage-row", data); err != nil {
			slog.Error("row template failed", "error", err)
			http.Error(w, "render error", http.StatusInternalServerError)
		}
		return
	}

	http.Redirect(w, r, "/routes/manage", http.StatusSeeOther)
}
