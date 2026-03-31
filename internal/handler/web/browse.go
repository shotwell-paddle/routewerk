package webhandler

import (
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

func (h *Handler) Routes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		h.renderError(w, r, http.StatusBadRequest, "No location selected", "Please select a location.")
		return
	}

	routeType := r.URL.Query().Get("type")
	if !validRouteTypes[routeType] {
		routeType = ""
	}
	wallID := r.URL.Query().Get("wall")
	gradeFilter := r.URL.Query().Get("grade")

	// Map frontend route types to DB route_type enum
	dbRouteType := routeType
	if routeType == "sport" || routeType == "top_rope" {
		dbRouteType = "route" // DB uses 'route' for all roped climbs
	}

	filter := repository.RouteFilter{
		LocationID: locationID,
		WallID:     wallID,
		Status:     "active",
		RouteType:  dbRouteType,
		Limit:      50,
	}

	// Expand grade filter chip values into SQL-friendly filter fields
	if gradeFilter != "" {
		if cc, ok := strings.CutPrefix(gradeFilter, "circuit:"); ok {
			filter.CircuitColor = cc
		} else {
			filter.GradeIn = expandGradeRange(gradeFilter)
		}
	}

	routes, total, err := h.routeRepo.ListWithDetails(ctx, filter)
	if err != nil {
		slog.Error("routes list failed", "location_id", locationID, "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Could not load routes.")
		return
	}

	// Load location settings for circuit grade visibility and filter chips
	locSettings, settErr := h.settingsRepo.GetLocationSettings(ctx, locationID)
	if settErr != nil {
		locSettings = model.DefaultLocationSettings()
	}

	// Determine if circuit grades should be hidden from this user
	effectiveRole := middleware.GetWebRole(ctx)
	isSetter := effectiveRole != "climber"
	hideCircuitGrade := !isSetter && !locSettings.Grading.ShowGradesOnCircuit

	var routeViews []RouteView
	for _, rd := range routes {
		routeViews = append(routeViews, RouteView{
			Route:            rd.Route,
			WallName:         rd.WallName,
			SetterName:       rd.SetterName,
			HideCircuitGrade: hideCircuitGrade,
		})
	}

	// Build grade groups from the grade distribution query
	gradeDist, err := h.analyticsRepo.GradeDistribution(ctx, locationID, "", "active")
	if err != nil {
		slog.Error("grade distribution failed", "location_id", locationID, "error", err)
	}

	gradeGroups := buildGradeGroups(gradeDist, &locSettings, isSetter)

	// Load walls for filter dropdown
	walls, wallErr := h.wallRepo.ListByLocation(ctx, locationID)
	if wallErr != nil {
		slog.Error("load walls for route filter failed", "error", wallErr)
	}

	data := &PageData{
		TemplateData:        templateDataFromContext(r, "routes"),
		Routes:              routeViews,
		TotalRoutes:         total,
		RouteType:           routeType,
		GradeGroups:         gradeGroups,
		GradeFilter:         gradeFilter,
		FormWalls:           walls,
		WallFilter:          wallID,
		ShowGradesOnCircuit: locSettings.Grading.ShowGradesOnCircuit,
	}
	h.render(w, r, "climber/routes.html", data)
}

// Archive renders the archived routes browser (GET /archive).
func (h *Handler) Archive(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		h.renderError(w, r, http.StatusBadRequest, "No location selected", "Please select a location.")
		return
	}

	routeType := r.URL.Query().Get("type")
	if !validRouteTypes[routeType] {
		routeType = ""
	}
	wallID := r.URL.Query().Get("wall")
	gradeFilter := r.URL.Query().Get("grade")
	dateFrom := r.URL.Query().Get("from")
	dateTo := r.URL.Query().Get("to")

	// Validate date format if provided
	if dateFrom != "" {
		if _, err := time.Parse("2006-01-02", dateFrom); err != nil {
			dateFrom = ""
		}
	}
	if dateTo != "" {
		if _, err := time.Parse("2006-01-02", dateTo); err != nil {
			dateTo = ""
		}
	}

	filter := repository.RouteFilter{
		LocationID: locationID,
		WallID:     wallID,
		Status:     "archived",
		RouteType:  routeType,
		DateFrom:   dateFrom,
		DateTo:     dateTo,
		Limit:      50,
	}

	if gradeFilter != "" {
		if cc, ok := strings.CutPrefix(gradeFilter, "circuit:"); ok {
			filter.CircuitColor = cc
		} else {
			filter.GradeIn = expandGradeRange(gradeFilter)
		}
	}

	routes, total, err := h.routeRepo.ListWithDetails(ctx, filter)
	if err != nil {
		slog.Error("archive list failed", "location_id", locationID, "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Could not load archived routes.")
		return
	}

	locSettings, settErr := h.settingsRepo.GetLocationSettings(ctx, locationID)
	if settErr != nil {
		locSettings = model.DefaultLocationSettings()
	}

	effectiveRole := middleware.GetWebRole(ctx)
	isSetter := effectiveRole != "climber"
	hideCircuitGrade := !isSetter && !locSettings.Grading.ShowGradesOnCircuit

	var routeViews []RouteView
	for _, rd := range routes {
		routeViews = append(routeViews, RouteView{
			Route:            rd.Route,
			WallName:         rd.WallName,
			SetterName:       rd.SetterName,
			HideCircuitGrade: hideCircuitGrade,
		})
	}

	// Load walls for filter dropdown
	walls, wallErr := h.wallRepo.ListByLocation(ctx, locationID)
	if wallErr != nil {
		slog.Error("load walls for archive filter failed", "error", wallErr)
	}

	// Build grade groups from the actual archived routes so chips reflect
	// what's in the archive, not what's currently active on the walls.
	gradeDist, err := h.analyticsRepo.GradeDistribution(ctx, locationID, "", "archived")
	if err != nil {
		slog.Error("archive grade distribution failed", "location_id", locationID, "error", err)
	}
	gradeGroups := buildGradeGroups(gradeDist, &locSettings, isSetter)

	data := &PageData{
		TemplateData:        templateDataFromContext(r, "archive"),
		Routes:              routeViews,
		TotalRoutes:         total,
		RouteType:           routeType,
		GradeGroups:         gradeGroups,
		GradeFilter:         gradeFilter,
		FormWalls:           walls,
		WallFilter:          wallID,
		DateFrom:            dateFrom,
		DateTo:              dateTo,
		TypeFilter:          routeType,
		ShowGradesOnCircuit: locSettings.Grading.ShowGradesOnCircuit,
	}
	h.render(w, r, "climber/archive.html", data)
}

func (h *Handler) RouteDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	routeID := chi.URLParam(r, "routeID")

	if !validRouteID.MatchString(routeID) {
		h.renderError(w, r, http.StatusBadRequest, "Invalid route", "The route ID format is not valid.")
		return
	}

	rt, err := h.routeRepo.GetByID(ctx, routeID)
	if err != nil {
		slog.Error("route detail failed", "route_id", routeID, "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Could not load route.")
		return
	}
	if rt == nil {
		h.renderError(w, r, http.StatusNotFound, "Route not found", "This route doesn't exist or has been removed.")
		return
	}

	if !h.checkLocationOwnership(w, r, rt.LocationID) {
		return
	}

	effectiveRole := middleware.GetWebRole(ctx)
	isSetter := effectiveRole != "climber"
	user := middleware.GetWebUser(ctx)

	// Parallelize all independent queries using a WaitGroup.
	var wg sync.WaitGroup

	var wallName string
	wg.Add(1)
	go func() {
		defer wg.Done()
		if wall, wErr := h.wallRepo.GetByID(ctx, rt.WallID); wErr == nil && wall != nil {
			wallName = wall.Name
		}
	}()

	setterName := "Unknown"
	wg.Add(1)
	go func() {
		defer wg.Done()
		if rt.SetterID != nil {
			if setter, uErr := h.userRepo.GetByID(ctx, *rt.SetterID); uErr == nil && setter != nil {
				setterName = setter.DisplayName
			}
		}
	}()

	var locSettings model.LocationSettings
	wg.Add(1)
	go func() {
		defer wg.Done()
		ls, settErr := h.settingsRepo.GetLocationSettings(ctx, rt.LocationID)
		if settErr != nil {
			locSettings = model.DefaultLocationSettings()
		} else {
			locSettings = ls
		}
	}()

	var recentAscents []AscentView
	wg.Add(1)
	go func() {
		defer wg.Done()
		viewerID := ""
		if user != nil {
			viewerID = user.ID
		}
		ascents, ascentErr := h.ascentRepo.ListByRouteForViewer(ctx, routeID, viewerID, 10, 0)
		if ascentErr != nil {
			slog.Error("load ascents for route detail failed", "route_id", routeID, "error", ascentErr)
		}
		for _, a := range ascents {
			initial := "?"
			if len(a.UserDisplayName) > 0 {
				initial = strings.ToUpper(a.UserDisplayName[:1])
			}
			recentAscents = append(recentAscents, AscentView{
				Ascent:      a.Ascent,
				UserName:    a.UserDisplayName,
				UserInitial: initial,
				AscentType:  ascentTypeLabel(a.AscentType),
				Notes:       a.Notes,
			})
		}
	}()

	var consensus *ConsensusData
	wg.Add(1)
	go func() {
		defer wg.Done()
		consensus = loadConsensus(ctx, h.difficultyRepo, routeID)
	}()

	userRating := 0
	canFlash := true
	hasCompleted := false
	if user != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if existing, rErr := h.ratingRepo.GetByUserAndRoute(ctx, user.ID, routeID); rErr == nil && existing != nil {
				userRating = existing.Rating
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			if hasAny, completed, sErr := h.ascentRepo.RouteAscentStatus(ctx, user.ID, routeID); sErr == nil {
				canFlash = !hasAny
				hasCompleted = completed
			}
		}()
	}

	var photos []repository.PhotoWithUploader
	wg.Add(1)
	go func() {
		defer wg.Done()
		var err error
		photos, err = h.photoRepo.ListByRoute(ctx, routeID)
		if err != nil {
			slog.Error("load route photos failed", "route_id", routeID, "error", err)
		}
	}()

	var communityTags []repository.AggregatedTag
	wg.Add(1)
	go func() {
		defer wg.Done()
		viewerID := ""
		if user != nil {
			viewerID = user.ID
		}
		var err error
		communityTags, err = h.userTagRepo.ListByRoute(ctx, routeID, viewerID)
		if err != nil {
			slog.Error("load community tags failed", "route_id", routeID, "error", err)
		}
	}()

	wg.Wait()

	photosEnabled := h.storageService.IsConfigured()

	rv := RouteView{
		Route:            *rt,
		WallName:         wallName,
		SetterName:       setterName,
		HideCircuitGrade: !isSetter && !locSettings.Grading.ShowGradesOnCircuit,
	}

	data := &PageData{
		TemplateData:        templateDataFromContext(r, "routes"),
		Route:               &rv,
		WallName:            wallName,
		SetterName:          setterName,
		RecentAscents:       recentAscents,
		Consensus:           consensus,
		UserRating:          userRating,
		CanFlash:            canFlash,
		HasCompleted:        hasCompleted,
		CommunityTags:       communityTags,
		Photos:              photos,
		PhotosEnabled:       photosEnabled,
		CanUploadPhoto:      photosEnabled,
		ShowGradesOnCircuit: locSettings.Grading.ShowGradesOnCircuit,
	}
	h.render(w, r, "climber/route-detail.html", data)
}
