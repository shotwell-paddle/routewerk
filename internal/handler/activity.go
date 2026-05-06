package handler

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

// ActivityHandler exposes the location-wide activity feed (climber
// quest progress + completions, badges earned, route sets, etc.).
// Mirrors the HTMX QuestActivity view at
// internal/handler/web/progressions_climber.go::QuestActivity, except
// the JSON endpoint is not gated by progressions_enabled — staff often
// want to see activity while the feature flag is still off.
type ActivityHandler struct {
	activity *repository.ActivityRepo
}

func NewActivityHandler(activity *repository.ActivityRepo) *ActivityHandler {
	return &ActivityHandler{activity: activity}
}

// List — GET /api/v1/locations/{locationID}/activity?limit=50&offset=0&type=...
//
// Returns the most recent activity entries for the location. Any
// authenticated location member can read.
func (h *ActivityHandler) List(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	if !isUUID(locationID) {
		Error(w, http.StatusBadRequest, "invalid location id")
		return
	}

	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	var (
		entries []model.ActivityLogEntry
		err     error
	)
	if t := r.URL.Query().Get("type"); t != "" {
		entries, err = h.activity.ListByLocationAndType(r.Context(), locationID, t, limit, offset)
	} else {
		entries, err = h.activity.ListByLocation(r.Context(), locationID, limit, offset)
	}
	if err != nil {
		slog.Error("activity feed failed", "location_id", locationID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if entries == nil {
		entries = []model.ActivityLogEntry{}
	}
	JSON(w, http.StatusOK, entries)
}
