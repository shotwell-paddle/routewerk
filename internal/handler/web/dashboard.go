package webhandler

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		h.renderError(w, r, http.StatusBadRequest, "No location selected", "Please select a location to view the dashboard.")
		return
	}

	// Dashboard stats
	dbStats, err := h.analyticsRepo.LocationDashboardStats(ctx, locationID)
	if err != nil {
		slog.Error("dashboard stats failed", "location_id", locationID, "error", err)
		dbStats = &repository.LocationDashboard{}
	}

	stats := &DashboardStats{
		ActiveRoutes: dbStats.ActiveRoutes,
		ActiveDelta:  dbStats.ActiveDelta,
		TotalSends:   dbStats.TotalSends30d,
		AvgRating:    dbStats.AvgRating,
		DueForStrip:  dbStats.DueForStrip,
	}

	// Walls + active routes in two queries (no N+1)
	walls, err := h.wallRepo.ListByLocation(ctx, locationID)
	if err != nil {
		slog.Error("dashboard walls failed", "location_id", locationID, "error", err)
	}

	allRoutes, err := h.routeRepo.ListActiveByLocation(ctx, locationID)
	if err != nil {
		slog.Error("dashboard routes failed", "location_id", locationID, "error", err)
	}

	// Group routes by wall ID
	routesByWall := make(map[string][]RouteView)
	for _, rd := range allRoutes {
		if len(routesByWall[rd.WallID]) >= 6 {
			continue // dashboard shows max 6 per wall
		}
		routesByWall[rd.WallID] = append(routesByWall[rd.WallID], RouteView{
			Route:      rd.Route,
			WallName:   rd.WallName,
			SetterName: rd.SetterName,
		})
	}

	var wallViews []WallWithRoutes
	for _, wall := range walls {
		rv := routesByWall[wall.ID]
		if len(rv) == 0 {
			continue
		}
		wallViews = append(wallViews, WallWithRoutes{
			Wall:       wall,
			RouteCount: len(rv),
			Routes:     rv,
		})
	}

	// Recent activity
	recentEntries, err := h.analyticsRepo.RecentActivity(ctx, locationID, 8)
	if err != nil {
		slog.Error("dashboard activity failed", "location_id", locationID, "error", err)
	}

	var activityItems []ActivityItem
	for _, e := range recentEntries {
		initial := "?"
		if len(e.UserName) > 0 {
			initial = strings.ToUpper(e.UserName[:1])
		}

		grade := e.RouteGrade
		if e.RouteGradingSystem == "v_scale" && e.RouteCircuitColor != nil {
			grade = titleCase(*e.RouteCircuitColor)
		}
		name := ""
		if e.RouteName != nil {
			name = *e.RouteName
		}

		activityItems = append(activityItems, ActivityItem{
			UserName:    e.UserName,
			UserInitial: initial,
			ActionText:  e.AscentType,
			Time:        e.Time,
			RoutePreview: &RoutePreviewData{
				Color: e.RouteColor,
				Grade: grade,
				Name:  name,
			},
		})
	}

	data := &PageData{
		TemplateData:   templateDataFromContext(r, "dashboard"),
		Stats:          stats,
		Walls:          wallViews,
		RecentActivity: activityItems,
	}
	h.render(w, r, "setter/dashboard.html", data)
}
