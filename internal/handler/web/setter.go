package webhandler

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

// ── Route Management ──────────────────────────────────────────

// RouteManage renders the setter's route management table (GET /routes/manage).
func (h *Handler) RouteManage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		h.renderError(w, r, http.StatusBadRequest, "No location selected", "Please select a location.")
		return
	}

	status := r.URL.Query().Get("status")
	if status != "" && status != "active" && status != "flagged" && status != "archived" {
		status = ""
	}
	wallFilter := r.URL.Query().Get("wall_id")

	filter := repository.RouteFilter{
		LocationID: locationID,
		Status:     status,
		WallID:     wallFilter,
		Limit:      200,
	}

	routes, total, err := h.routeRepo.ListWithDetails(ctx, filter)
	if err != nil {
		slog.Error("route manage list failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Could not load routes.")
		return
	}

	var routeViews []RouteView
	for _, rd := range routes {
		routeViews = append(routeViews, RouteView{
			Route:      rd.Route,
			WallName:   rd.WallName,
			SetterName: rd.SetterName,
		})
	}

	walls, err := h.wallRepo.ListByLocation(ctx, locationID)
	if err != nil {
		slog.Error("load walls for route manage failed", "location_id", locationID, "error", err)
	}

	data := &PageData{
		TemplateData: templateDataFromContext(r, "manage-routes"),
		Routes:       routeViews,
		TotalRoutes:  total,
		StatusFilter: status,
		WallFilter:   wallFilter,
		FormWalls:    walls,
	}
	h.render(w, r, "setter/route-manage.html", data)
}

// RouteNew renders the route creation form (GET /routes/new).
func (h *Handler) RouteNew(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		h.renderError(w, r, http.StatusBadRequest, "No location selected", "Please select a location.")
		return
	}

	walls, err := h.wallRepo.ListByLocation(ctx, locationID)
	if err != nil {
		slog.Error("load walls for new route failed", "location_id", locationID, "error", err)
	}
	tags := h.loadOrgTags(ctx, locationID)
	locSettings, err := h.settingsRepo.GetLocationSettings(ctx, locationID)
	if err != nil {
		slog.Error("load location settings for new route failed", "location_id", locationID, "error", err)
	}
	setters, err := h.userRepo.ListSettersByLocation(ctx, locationID)
	if err != nil {
		slog.Error("load setters for new route failed", "location_id", locationID, "error", err)
	}

	// Build initial route fields (no wall selected yet — shows placeholder)
	routeFields := buildRouteFields("", "", "", locSettings.Grading.BoulderMethod, "", locSettings, nil)

	// Default to current user as setter
	currentUserID := middleware.GetUserID(ctx)

	data := &PageData{
		TemplateData:  templateDataFromContext(r, "manage-routes"),
		FormWalls:     walls,
		FormTags:      tags,
		Setters:       setters,
		HoldColors:    holdColorsFromSettings(locSettings),
		VScaleGrades:  vScaleGrades,
		YDSGrades:     ydsGrades,
		CircuitColors: circuitColors,
		RouteFields:   routeFields,
		FormValues: RouteFormValues{
			SetterID: currentUserID,
			DateSet:  time.Now().Format("2006-01-02"),
			TagIDs:   make(map[string]bool),
		},
	}
	h.render(w, r, "setter/route-form.html", data)
}

// RouteFormFields returns the type/grade/color fields partial based on wall selection.
// GET /routes/new/fields?wall_id=...&route_type=...
func (h *Handler) RouteFormFields(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	wallID := r.URL.Query().Get("wall_id")
	routeType := r.URL.Query().Get("route_type")

	var wallType string
	if wallID != "" {
		wall, err := h.wallRepo.GetByID(ctx, wallID)
		if err == nil && wall != nil {
			wallType = wall.WallType
		}
	}

	locSettings, err := h.settingsRepo.GetLocationSettings(ctx, locationID)
	if err != nil {
		locSettings = model.DefaultLocationSettings()
	}

	rf := buildRouteFields("", wallID, wallType, locSettings.Grading.BoulderMethod, routeType, locSettings, nil)

	tmpl, ok := h.templates["setter/route-form.html"]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "route-form-fields", rf); err != nil {
		slog.Error("render route-form-fields partial failed", "error", err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

// RouteCreate processes the route creation form (POST /routes/new).
func (h *Handler) RouteCreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		h.renderError(w, r, http.StatusBadRequest, "No location selected", "Please select a location.")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Invalid form", "Could not parse form data.")
		return
	}

	fv := parseRouteForm(r)

	// Validate required fields
	var problems []string
	if fv.WallID == "" {
		problems = append(problems, "Wall is required")
	}
	if fv.Grade == "" {
		problems = append(problems, "Grade is required")
	}
	if fv.Color == "" {
		fv.Color = "#e53935"
	}

	// Verify selected wall belongs to this location
	if fv.WallID != "" {
		wall, wErr := h.wallRepo.GetByID(ctx, fv.WallID)
		if wErr != nil || wall == nil || wall.LocationID != locationID {
			problems = append(problems, "Selected wall is not valid for this location")
		}
	}

	if len(problems) > 0 {
		h.renderRouteForm(w, r, locationID, nil, fv, strings.Join(problems, ". ")+".")
		return
	}

	// Use selected setter from form, falling back to the current user
	setterID := fv.SetterID
	if setterID == "" {
		setterID = middleware.GetUserID(ctx)
	}

	dateSet := time.Now()
	if fv.DateSet != "" {
		if parsed, err := time.Parse("2006-01-02", fv.DateSet); err == nil {
			dateSet = parsed
		}
	}

	var projectedStrip *time.Time
	if fv.ProjectedStripDate != "" {
		if parsed, err := time.Parse("2006-01-02", fv.ProjectedStripDate); err == nil {
			projectedStrip = &parsed
		}
	}

	var circuitColor *string
	if fv.CircuitColor != "" {
		circuitColor = &fv.CircuitColor
	}

	var name *string
	if fv.Name != "" {
		name = &fv.Name
	}

	var description *string
	if fv.Description != "" {
		description = &fv.Description
	}

	rt := &model.Route{
		LocationID:         locationID,
		WallID:             fv.WallID,
		SetterID:           &setterID,
		RouteType:          fv.RouteType,
		Status:             "active",
		GradingSystem:      fv.GradingSystem,
		Grade:              fv.Grade,
		CircuitColor:       circuitColor,
		Name:               name,
		Color:              fv.Color,
		Description:        description,
		DateSet:            dateSet,
		ProjectedStripDate: projectedStrip,
	}

	if err := h.routeRepo.Create(ctx, rt); err != nil {
		slog.Error("route create failed", "error", err)
		h.renderRouteForm(w, r, locationID, nil, fv, "Failed to create route. Please try again.")
		return
	}

	// Set tags
	var tagIDs []string
	for id := range fv.TagIDs {
		tagIDs = append(tagIDs, id)
	}
	if len(tagIDs) > 0 {
		if err := h.routeRepo.SetTags(ctx, rt.ID, tagIDs); err != nil {
			slog.Error("route set tags failed", "route_id", rt.ID, "error", err)
		}
	}

	// Redirect to manage page with success indicator
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/routes/manage")
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, "/routes/manage", http.StatusSeeOther)
}

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

// ── Form Helpers ──────────────────────────────────────────────

// parseRouteForm extracts RouteFormValues from a submitted form.
func parseRouteForm(r *http.Request) RouteFormValues {
	fv := RouteFormValues{
		WallID:             r.FormValue("wall_id"),
		SetterID:           r.FormValue("setter_id"),
		RouteType:          r.FormValue("route_type"),
		GradingSystem:      r.FormValue("grading_system"),
		Grade:              r.FormValue("grade"),
		CircuitColor:       r.FormValue("circuit_color"),
		Name:               strings.TrimSpace(r.FormValue("name")),
		Color:              r.FormValue("color"),
		Description:        strings.TrimSpace(r.FormValue("description")),
		DateSet:            r.FormValue("date_set"),
		ProjectedStripDate: r.FormValue("projected_strip_date"),
		TagIDs:             make(map[string]bool),
	}

	for _, tagID := range r.Form["tag_ids"] {
		fv.TagIDs[tagID] = true
	}

	// Map composite route types from the route-fields partial.
	// "boulder_circuit" means boulder with circuit grading — grade holds the color name.
	if fv.RouteType == "boulder_circuit" {
		fv.RouteType = "boulder"
		fv.GradingSystem = "circuit"
		fv.CircuitColor = fv.Grade
		// If an optional V-grade was provided, use it as the display grade
		if vg := r.FormValue("circuit_vgrade"); vg != "" {
			fv.Grade = vg
		}
	} else if fv.RouteType == "route" {
		fv.GradingSystem = "yds"
	} else {
		// Default boulder = v_scale
		fv.RouteType = "boulder"
		if fv.GradingSystem != "v_scale" && fv.GradingSystem != "circuit" {
			fv.GradingSystem = "v_scale"
		}
	}

	return fv
}

// renderRouteForm renders the route form with current values and optional error.
func (h *Handler) renderRouteForm(w http.ResponseWriter, r *http.Request, locationID string, route *RouteView, fv RouteFormValues, formError string) {
	ctx := r.Context()

	walls, err := h.wallRepo.ListByLocation(ctx, locationID)
	if err != nil {
		slog.Error("load walls for route form failed", "location_id", locationID, "error", err)
	}
	tags := h.loadOrgTags(ctx, locationID)
	locSettings, err := h.settingsRepo.GetLocationSettings(ctx, locationID)
	if err != nil {
		slog.Error("load location settings for route form failed", "location_id", locationID, "error", err)
	}
	setters, err := h.userRepo.ListSettersByLocation(ctx, locationID)
	if err != nil {
		slog.Error("load setters for route form failed", "location_id", locationID, "error", err)
	}

	// Determine wall type for route fields
	var wallType string
	if fv.WallID != "" {
		if wall, wErr := h.wallRepo.GetByID(ctx, fv.WallID); wErr == nil && wall != nil {
			wallType = wall.WallType
		}
	}
	// Map stored route type + grading system back to composite type for buildRouteFields.
	// Database stores "boulder" + "circuit", but the form uses "boulder_circuit".
	selectedType := fv.RouteType
	if fv.RouteType == "boulder" && fv.GradingSystem == "circuit" {
		selectedType = "boulder_circuit"
	}
	routeFields := buildRouteFields("", fv.WallID, wallType, locSettings.Grading.BoulderMethod, selectedType, locSettings, nil)

	data := &PageData{
		TemplateData:  templateDataFromContext(r, "manage-routes"),
		Route:         route,
		FormWalls:     walls,
		FormTags:      tags,
		Setters:       setters,
		FormValues:    fv,
		FormError:     formError,
		HoldColors:    holdColorsFromSettings(locSettings),
		VScaleGrades:  vScaleGrades,
		YDSGrades:     ydsGrades,
		CircuitColors: circuitColors,
		RouteFields:   routeFields,
	}

	h.render(w, r, "setter/route-form.html", data)
}

// loadOrgTags fetches tags for the org that owns the given location.
func (h *Handler) loadOrgTags(ctx context.Context, locationID string) []model.Tag {
	loc, err := h.locationRepo.GetByID(ctx, locationID)
	if err != nil || loc == nil {
		return nil
	}
	tags, err := h.tagRepo.ListByOrg(ctx, loc.OrgID, "")
	if err != nil {
		slog.Error("load org tags failed", "org_id", loc.OrgID, "error", err)
		return nil
	}
	return tags
}
