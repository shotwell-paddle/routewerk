package handler

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

// BadgeShowcaseHandler exposes the climber-side badge view that the
// HTMX page at /quests/badges renders. Returns the location's full
// badge catalog plus the caller's earned set so the SPA can show
// earned-vs-unearned cards.
type BadgeShowcaseHandler struct {
	badges *repository.BadgeRepo
}

func NewBadgeShowcaseHandler(badges *repository.BadgeRepo) *BadgeShowcaseHandler {
	return &BadgeShowcaseHandler{badges: badges}
}

type badgeShowcaseEntry struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	Icon        string  `json:"icon"`
	Color       string  `json:"color"`
	EarnedAt    *string `json:"earned_at,omitempty"`
}

// Get — GET /api/v1/locations/{locationID}/badges/showcase.
//
// Any authenticated location member can read. Returns every badge at
// the location with `earned_at` populated when the caller has earned it
// (matches the HTMX BadgeShowcase merge in
// internal/handler/web/progressions_climber.go).
func (h *BadgeShowcaseHandler) Get(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	if !isUUID(locationID) {
		Error(w, http.StatusBadRequest, "invalid location id")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		Error(w, http.StatusUnauthorized, "authentication required")
		return
	}

	all, err := h.badges.ListByLocation(r.Context(), locationID)
	if err != nil {
		slog.Error("badge showcase: list all failed", "location_id", locationID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	earned, err := h.badges.ListUserBadgesForLocation(r.Context(), userID, locationID)
	if err != nil {
		slog.Error("badge showcase: list earned failed", "location_id", locationID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	earnedAt := make(map[string]string, len(earned))
	for _, cb := range earned {
		earnedAt[cb.BadgeID] = cb.EarnedAt.Format("2006-01-02T15:04:05Z07:00")
	}

	out := make([]badgeShowcaseEntry, 0, len(all))
	for _, b := range all {
		entry := badgeShowcaseEntry{
			ID:          b.ID,
			Name:        b.Name,
			Description: b.Description,
			Icon:        b.Icon,
			Color:       b.Color,
		}
		if iso, ok := earnedAt[b.ID]; ok {
			entry.EarnedAt = &iso
		}
		out = append(out, entry)
	}
	JSON(w, http.StatusOK, out)
}
