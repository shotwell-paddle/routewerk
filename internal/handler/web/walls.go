package webhandler

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

// ── Wall List ─────────────────────────────────────────────────

// WallList renders the wall management grid (GET /walls).
func (h *Handler) WallList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		h.renderError(w, r, http.StatusBadRequest, "No location selected", "Please select a location.")
		return
	}

	walls, err := h.wallRepo.ListWithCounts(ctx, locationID)
	if err != nil {
		slog.Error("wall list failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Could not load walls.")
		return
	}

	var wallViews []WallView
	for _, wc := range walls {
		wallViews = append(wallViews, WallView{
			Wall:           wc.Wall,
			ActiveRoutes:   wc.ActiveRoutes,
			FlaggedRoutes:  wc.FlaggedRoutes,
			ArchivedRoutes: wc.ArchivedRoutes,
			TotalRoutes:    wc.TotalRoutes,
		})
	}

	data := &PageData{
		TemplateData: templateDataFromContext(r, "walls"),
		WallList:     wallViews,
	}
	h.render(w, r, "setter/walls.html", data)
}

// ── Wall Detail ───────────────────────────────────────────────

// WallDetail renders the wall detail page with its routes (GET /walls/{wallID}).
func (h *Handler) WallDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	wallID := chi.URLParam(r, "wallID")

	if !validRouteID.MatchString(wallID) {
		h.renderError(w, r, http.StatusBadRequest, "Invalid wall", "The wall ID is not valid.")
		return
	}

	wall, err := h.wallRepo.GetByID(ctx, wallID)
	if err != nil || wall == nil {
		h.renderError(w, r, http.StatusNotFound, "Wall not found", "This wall doesn't exist.")
		return
	}

	if !h.checkLocationOwnership(w, r, wall.LocationID) {
		return
	}

	status := r.URL.Query().Get("status")
	if status != "" && status != "active" && status != "flagged" && status != "archived" {
		status = ""
	}

	filter := repository.RouteFilter{
		LocationID: locationID,
		WallID:     wallID,
		Status:     status,
		Limit:      100,
	}

	routes, total, err := h.routeRepo.ListWithDetails(ctx, filter)
	if err != nil {
		slog.Error("wall detail routes failed", "wall_id", wallID, "error", err)
	}

	var routeViews []RouteView
	for _, rd := range routes {
		routeViews = append(routeViews, RouteView{
			Route:      rd.Route,
			WallName:   rd.WallName,
			SetterName: rd.SetterName,
		})
	}

	wv := &WallView{Wall: *wall}

	data := &PageData{
		TemplateData: templateDataFromContext(r, "walls"),
		Wall:         wv,
		Routes:       routeViews,
		TotalRoutes:  total,
		StatusFilter: status,
	}
	h.render(w, r, "setter/wall-detail.html", data)
}

// ── Wall Create ───────────────────────────────────────────────

// WallNew renders the wall creation form (GET /walls/new).
func (h *Handler) WallNew(w http.ResponseWriter, r *http.Request) {
	data := &PageData{
		TemplateData: templateDataFromContext(r, "walls"),
		WallFormValues: WallFormValues{
			WallType:  "boulder",
			SortOrder: "0",
		},
	}
	h.render(w, r, "setter/wall-form.html", data)
}

// WallCreate processes the wall creation form (POST /walls/new).
func (h *Handler) WallCreate(w http.ResponseWriter, r *http.Request) {
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

	fv := parseWallForm(r)

	if fv.Name == "" {
		h.renderWallForm(w, r, nil, fv, "Wall name is required.")
		return
	}

	if fv.WallType != "boulder" && fv.WallType != "route" {
		h.renderWallForm(w, r, nil, fv, "Wall type must be boulder or route.")
		return
	}

	wall := wallFromFormValues(locationID, fv)

	if err := h.wallRepo.Create(ctx, wall); err != nil {
		slog.Error("wall create failed", "error", err)
		h.renderWallForm(w, r, nil, fv, "Failed to create wall. Please try again.")
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/walls")
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, "/walls", http.StatusSeeOther)
}

// ── Wall Edit ─────────────────────────────────────────────────

// WallEdit renders the wall edit form (GET /walls/{wallID}/edit).
func (h *Handler) WallEdit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	wallID := chi.URLParam(r, "wallID")

	if !validRouteID.MatchString(wallID) {
		h.renderError(w, r, http.StatusBadRequest, "Invalid wall", "The wall ID is not valid.")
		return
	}

	wall, err := h.wallRepo.GetByID(ctx, wallID)
	if err != nil || wall == nil {
		h.renderError(w, r, http.StatusNotFound, "Wall not found", "This wall doesn't exist.")
		return
	}

	if !h.checkLocationOwnership(w, r, wall.LocationID) {
		return
	}

	fv := wallFormValuesFromModel(wall)
	wv := &WallView{Wall: *wall}

	data := &PageData{
		TemplateData:   templateDataFromContext(r, "walls"),
		Wall:           wv,
		WallFormValues: fv,
	}
	h.render(w, r, "setter/wall-form.html", data)
}

// WallUpdate processes the wall edit form (POST /walls/{wallID}/edit).
func (h *Handler) WallUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	wallID := chi.URLParam(r, "wallID")

	if !validRouteID.MatchString(wallID) {
		h.renderError(w, r, http.StatusBadRequest, "Invalid wall", "The wall ID is not valid.")
		return
	}

	wall, err := h.wallRepo.GetByID(ctx, wallID)
	if err != nil || wall == nil {
		h.renderError(w, r, http.StatusNotFound, "Wall not found", "This wall doesn't exist.")
		return
	}

	if !h.checkLocationOwnership(w, r, wall.LocationID) {
		return
	}

	if parseErr := r.ParseForm(); parseErr != nil {
		h.renderError(w, r, http.StatusBadRequest, "Invalid form", "Could not parse form data.")
		return
	}

	fv := parseWallForm(r)

	if fv.Name == "" {
		wv := &WallView{Wall: *wall}
		h.renderWallForm(w, r, wv, fv, "Wall name is required.")
		return
	}

	// Apply form values to existing wall
	applyWallFormValues(wall, fv)

	if updateErr := h.wallRepo.Update(ctx, wall); updateErr != nil {
		slog.Error("wall update failed", "wall_id", wallID, "error", updateErr)
		wv := &WallView{Wall: *wall}
		h.renderWallForm(w, r, wv, fv, "Failed to update wall. Please try again.")
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/walls")
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, "/walls", http.StatusSeeOther)
}

// ── Wall Delete ───────────────────────────────────────────────

// WallDelete handles POST /walls/{wallID}/delete.
func (h *Handler) WallDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	wallID := chi.URLParam(r, "wallID")

	if !validRouteID.MatchString(wallID) {
		http.Error(w, "invalid wall ID", http.StatusBadRequest)
		return
	}

	wall, getErr := h.wallRepo.GetByID(ctx, wallID)
	if getErr != nil || wall == nil {
		http.Error(w, "wall not found", http.StatusNotFound)
		return
	}
	if !h.checkLocationOwnership(w, r, wall.LocationID) {
		return
	}

	if err := h.wallRepo.Delete(ctx, wallID); err != nil {
		slog.Error("wall delete failed", "wall_id", wallID, "error", err)
		http.Error(w, "failed to delete wall", http.StatusInternalServerError)
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/walls")
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, "/walls", http.StatusSeeOther)
}

// ── Form Helpers ──────────────────────────────────────────────

func parseWallForm(r *http.Request) WallFormValues {
	return WallFormValues{
		Name:         strings.TrimSpace(r.FormValue("name")),
		WallType:     r.FormValue("wall_type"),
		Angle:        strings.TrimSpace(r.FormValue("angle")),
		HeightMeters: strings.TrimSpace(r.FormValue("height_meters")),
		NumAnchors:   strings.TrimSpace(r.FormValue("num_anchors")),
		SurfaceType:  strings.TrimSpace(r.FormValue("surface_type")),
		SortOrder:    strings.TrimSpace(r.FormValue("sort_order")),
	}
}

func wallFromFormValues(locationID string, fv WallFormValues) *model.Wall {
	wall := &model.Wall{
		LocationID: locationID,
		Name:       fv.Name,
		WallType:   fv.WallType,
	}

	if fv.Angle != "" {
		wall.Angle = &fv.Angle
	}
	if fv.HeightMeters != "" {
		if h, err := strconv.ParseFloat(fv.HeightMeters, 64); err == nil {
			wall.HeightMeters = &h
		}
	}
	if fv.NumAnchors != "" {
		if n, err := strconv.Atoi(fv.NumAnchors); err == nil {
			wall.NumAnchors = &n
		}
	}
	if fv.SurfaceType != "" {
		wall.SurfaceType = &fv.SurfaceType
	}
	if fv.SortOrder != "" {
		if s, err := strconv.Atoi(fv.SortOrder); err == nil {
			wall.SortOrder = s
		}
	}

	return wall
}

func applyWallFormValues(wall *model.Wall, fv WallFormValues) {
	wall.Name = fv.Name
	wall.WallType = fv.WallType

	if fv.Angle != "" {
		wall.Angle = &fv.Angle
	} else {
		wall.Angle = nil
	}

	if fv.HeightMeters != "" {
		if h, err := strconv.ParseFloat(fv.HeightMeters, 64); err == nil {
			wall.HeightMeters = &h
		}
	} else {
		wall.HeightMeters = nil
	}

	if fv.NumAnchors != "" {
		if n, err := strconv.Atoi(fv.NumAnchors); err == nil {
			wall.NumAnchors = &n
		}
	} else {
		wall.NumAnchors = nil
	}

	if fv.SurfaceType != "" {
		wall.SurfaceType = &fv.SurfaceType
	} else {
		wall.SurfaceType = nil
	}

	if fv.SortOrder != "" {
		if s, err := strconv.Atoi(fv.SortOrder); err == nil {
			wall.SortOrder = s
		}
	}
}

func wallFormValuesFromModel(wall *model.Wall) WallFormValues {
	fv := WallFormValues{
		Name:      wall.Name,
		WallType:  wall.WallType,
		SortOrder: strconv.Itoa(wall.SortOrder),
	}
	if wall.Angle != nil {
		fv.Angle = *wall.Angle
	}
	if wall.HeightMeters != nil {
		fv.HeightMeters = fmt.Sprintf("%.1f", *wall.HeightMeters)
	}
	if wall.NumAnchors != nil {
		fv.NumAnchors = strconv.Itoa(*wall.NumAnchors)
	}
	if wall.SurfaceType != nil {
		fv.SurfaceType = *wall.SurfaceType
	}
	return fv
}

func (h *Handler) renderWallForm(w http.ResponseWriter, r *http.Request, wall *WallView, fv WallFormValues, formError string) {
	data := &PageData{
		TemplateData:   templateDataFromContext(r, "walls"),
		Wall:           wall,
		WallFormValues: fv,
		WallFormError:  formError,
	}
	h.render(w, r, "setter/wall-form.html", data)
}
