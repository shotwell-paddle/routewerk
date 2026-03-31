package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

type AnalyticsHandler struct {
	analytics *repository.AnalyticsRepo
}

func NewAnalyticsHandler(analytics *repository.AnalyticsRepo) *AnalyticsHandler {
	return &AnalyticsHandler{analytics: analytics}
}

func (h *AnalyticsHandler) GradeDistribution(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	wallID := r.URL.Query().Get("wall_id")

	grades, err := h.analytics.GradeDistribution(r.Context(), locationID, wallID, "")
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	JSON(w, http.StatusOK, grades)
}

func (h *AnalyticsHandler) RouteLifecycle(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")

	routes, err := h.analytics.RouteLifecycle(r.Context(), locationID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	JSON(w, http.StatusOK, routes)
}

func (h *AnalyticsHandler) Engagement(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))

	stats, err := h.analytics.Engagement(r.Context(), locationID, days)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	JSON(w, http.StatusOK, stats)
}

func (h *AnalyticsHandler) SetterProductivity(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))

	stats, err := h.analytics.SetterProductivity(r.Context(), locationID, days)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	JSON(w, http.StatusOK, stats)
}

func (h *AnalyticsHandler) OrgOverview(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgID")

	overview, err := h.analytics.OrgOverview(r.Context(), orgID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	JSON(w, http.StatusOK, overview)
}
