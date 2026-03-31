package webhandler

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/shotwell-paddle/routewerk/internal/model"
)

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
		PhotosEnabled: h.storageService.IsConfigured(),
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
