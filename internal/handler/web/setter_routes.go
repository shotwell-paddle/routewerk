package webhandler

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
	"github.com/shotwell-paddle/routewerk/internal/service"
)

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
		PhotosEnabled: h.storageService.IsConfigured(),
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

	// Parse as multipart to support optional photo upload.
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		// Fall back to regular form parse (no file attached).
		if err := r.ParseForm(); err != nil {
			h.renderError(w, r, http.StatusBadRequest, "Invalid form", "Could not parse form data.")
			return
		}
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

	// Collect tags for atomic creation
	var tagIDs []string
	for id := range fv.TagIDs {
		tagIDs = append(tagIDs, id)
	}

	if err := h.routeRepo.CreateWithTags(ctx, rt, tagIDs); err != nil {
		slog.Error("route create failed", "error", err)
		h.renderRouteForm(w, r, locationID, nil, fv, "Failed to create route. Please try again.")
		return
	}

	// Process optional photo upload.
	h.processRoutePhoto(ctx, r, rt)

	// Redirect to manage page with success indicator
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/routes/manage")
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, "/routes/manage", http.StatusSeeOther)
}

// processRoutePhoto handles an optional photo upload attached to a route form.
// It logs errors but never fails the request — the route is already created.
func (h *Handler) processRoutePhoto(ctx context.Context, r *http.Request, route *model.Route) {
	file, header, err := r.FormFile("photo")
	if err != nil {
		return // no file attached — normal case
	}
	defer file.Close()

	if !h.storageService.IsConfigured() {
		return
	}

	contentType := header.Header.Get("Content-Type")
	if !allowedImageTypes[contentType] {
		slog.Warn("route create photo: invalid type", "type", contentType)
		return
	}
	if header.Size > maxUploadSize {
		slog.Warn("route create photo: too large", "size", header.Size)
		return
	}

	// Acquire upload semaphore.
	select {
	case h.uploadSem <- struct{}{}:
		defer func() { <-h.uploadSem }()
	case <-ctx.Done():
		return
	}

	processed, err := service.ProcessImage(file, contentType)
	if err != nil {
		slog.Error("route create photo: processing failed", "route_id", route.ID, "error", err)
		return
	}

	uploadFilename := strings.TrimSuffix(header.Filename, ".webp") + processed.Extension
	photoURL, err := h.storageService.Upload(ctx, route.ID, uploadFilename, processed.ContentType, processed.Data)
	if err != nil {
		slog.Error("route create photo: upload failed", "route_id", route.ID, "error", err)
		return
	}

	user := middleware.GetWebUser(ctx)
	var uploaderID *string
	if user != nil {
		uploaderID = &user.ID
	}

	photo := &model.RoutePhoto{
		RouteID:    route.ID,
		PhotoURL:   photoURL,
		UploadedBy: uploaderID,
		SortOrder:  0,
	}
	if err := h.photoRepo.Create(ctx, photo); err != nil {
		slog.Error("route create photo: save failed", "route_id", route.ID, "error", err)
		_ = h.storageService.Delete(ctx, photoURL)
		return
	}

	// Set as primary photo.
	route.PhotoURL = &photoURL
	if err := h.routeRepo.Update(ctx, route); err != nil {
		slog.Error("route create photo: set primary failed", "route_id", route.ID, "error", err)
	}
	slog.Info("route photo uploaded during creation", "route_id", route.ID, "url", fmt.Sprintf("%.40s…", photoURL))
}
