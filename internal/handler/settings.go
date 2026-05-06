package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

// SettingsHandler exposes the location-level settings JSON for the
// SPA at /app/settings/gym. Backed by the cached settings repo that
// HTMX uses, so reads stay cheap and writes invalidate the cache.
type SettingsHandler struct {
	settings *repository.CachedSettingsRepo
}

func NewSettingsHandler(settings *repository.CachedSettingsRepo) *SettingsHandler {
	return &SettingsHandler{settings: settings}
}

// GetLocationSettings — GET /locations/{locationID}/settings.
// Returns the full LocationSettings struct (grading + circuits +
// hold-colors + display + sessions). Defaults are returned if the
// column is empty.
func (h *SettingsHandler) GetLocationSettings(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	settings, err := h.settings.GetLocationSettings(r.Context(), locationID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	JSON(w, http.StatusOK, settings)
}

// UpdateLocationSettings — PUT /locations/{locationID}/settings.
// Replaces the full LocationSettings struct. The router gates this on
// gym_manager+ at the location to match HTMX policy.
func (h *SettingsHandler) UpdateLocationSettings(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	var settings model.LocationSettings
	if err := Decode(r, &settings); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.settings.UpdateLocationSettings(r.Context(), locationID, settings); err != nil {
		Error(w, http.StatusInternalServerError, "failed to save settings")
		return
	}
	// Re-read from the cache (the Update method invalidates) so the
	// caller sees the post-default-merge state if anything was filled in.
	saved, err := h.settings.GetLocationSettings(r.Context(), locationID)
	if err != nil {
		// Fall back to whatever was sent.
		JSON(w, http.StatusOK, settings)
		return
	}
	JSON(w, http.StatusOK, saved)
}
