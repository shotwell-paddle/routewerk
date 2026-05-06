package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/shotwell-paddle/routewerk/internal/repository"
)

// DashboardHandler powers the SPA setter dashboard at /app — the
// stats + recent-activity surface the HTMX /dashboard already renders.
// JSON-only; the HTMX dashboard.go composes its own template view.
type DashboardHandler struct {
	analytics *repository.AnalyticsRepo
}

func NewDashboardHandler(analytics *repository.AnalyticsRepo) *DashboardHandler {
	return &DashboardHandler{analytics: analytics}
}

// Stats — GET /locations/{locationID}/dashboard. Returns the same numbers
// the HTMX dashboard shows in its stat cards (active routes / total sends
// / avg rating / due-for-strip) plus a recent-activity feed for the
// sidebar. Single endpoint so the SPA only round-trips once on mount.
func (h *DashboardHandler) Stats(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")

	activityLimit := 8
	if v := r.URL.Query().Get("activity_limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil && n > 0 && n <= 50 {
			activityLimit = n
		}
	}

	stats, err := h.analytics.LocationDashboardStats(r.Context(), locationID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	activity, err := h.analytics.RecentActivity(r.Context(), locationID, activityLimit)
	if err != nil {
		// Activity is best-effort — empty array is a fine fallback.
		activity = nil
	}
	if activity == nil {
		activity = []repository.ActivityEntry{}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"stats":           stats,
		"recent_activity": activity,
	})
}
