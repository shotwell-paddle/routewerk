package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
	"github.com/shotwell-paddle/routewerk/internal/service"
)

// SettingsHandler exposes the location-level settings JSON for the
// SPA at /app/settings/gym. Backed by the cached settings repo that
// HTMX uses, so reads stay cheap and writes invalidate the cache.
type SettingsHandler struct {
	settings *repository.CachedSettingsRepo
	audit    *service.AuditService
}

func NewSettingsHandler(settings *repository.CachedSettingsRepo, audit *service.AuditService) *SettingsHandler {
	return &SettingsHandler{settings: settings, audit: audit}
}

// GetLocationSettings — GET /locations/{locationID}/settings.
// Returns the full LocationSettings struct (grading + circuits +
// hold-colors + display + sessions). Defaults are returned if the
// column is empty.
func (h *SettingsHandler) GetLocationSettings(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	settings, err := h.settings.GetLocationSettings(r.Context(), locationID)
	if err != nil {
		InternalError(w, r, "internal error", err)
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
		InternalError(w, r, "failed to save settings", err)
		return
	}
	// Settings drive grading systems, circuits, and palettes gym-wide —
	// a privileged mutation worth a trail (audit gap flagged in the
	// 2026-07 best-practices audit).
	if h.audit != nil {
		h.audit.Record(r, service.AuditSettingsUpdate, "location_settings", locationID, "", map[string]interface{}{
			"boulder_method": settings.Grading.BoulderMethod,
			"circuit_count":  len(settings.Circuits.Colors),
		})
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

type palettePresetSwatch struct {
	Name string `json:"name"`
	Hex  string `json:"hex"`
}

type palettePresetEntry struct {
	Name        string                `json:"name"`
	DisplayName string                `json:"display_name"`
	Description string                `json:"description"`
	Circuits    []palettePresetSwatch `json:"circuits"`
}

// ListPalettePresets — GET /api/v1/settings/palette-presets.
//
// Returns the catalog of named palette presets so the SPA can render
// them as one-click apply buttons with a swatch preview. Public (any
// authenticated user) because the catalog itself is global; the apply
// endpoint is gated.
func (h *SettingsHandler) ListPalettePresets(w http.ResponseWriter, r *http.Request) {
	out := make([]palettePresetEntry, 0, len(model.PalettePresets))
	for _, p := range model.PalettePresets {
		swatches := make([]palettePresetSwatch, 0, len(p.Circuits))
		for _, c := range p.Circuits {
			swatches = append(swatches, palettePresetSwatch{Name: c.Name, Hex: c.Hex})
		}
		out = append(out, palettePresetEntry{
			Name:        p.Name,
			DisplayName: p.DisplayName,
			Description: p.Description,
			Circuits:    swatches,
		})
	}
	JSON(w, http.StatusOK, out)
}

type applyPalettePresetRequest struct {
	Preset string `json:"preset"`
}

// ApplyPalettePreset — POST /api/v1/locations/{id}/settings/palette-preset.
//
// Replaces the gym's circuits + hold-color lists with the named preset
// in one shot. head_setter+ matches the HTMX policy at
// internal/handler/web/settings.go::GymSettingsApplyPalettePreset.
// Returns the post-apply settings so the SPA can refresh inline.
func (h *SettingsHandler) ApplyPalettePreset(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	var req applyPalettePresetRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	preset := model.LookupPalettePreset(req.Preset)
	if preset == nil {
		Error(w, http.StatusBadRequest, "unknown preset")
		return
	}

	settings, err := h.settings.GetLocationSettings(r.Context(), locationID)
	if err != nil {
		InternalError(w, r, "could not load settings", err)
		return
	}
	// Clone to avoid sharing backing storage between locations.
	settings.Circuits.Colors = append([]model.CircuitColor(nil), preset.Circuits...)
	settings.HoldColors.Colors = append([]model.HoldColor(nil), preset.HoldColors...)

	if err := h.settings.UpdateLocationSettings(r.Context(), locationID, settings); err != nil {
		InternalError(w, r, "failed to save settings", err)
		return
	}
	if h.audit != nil {
		h.audit.Record(r, service.AuditSettingsUpdate, "location_settings", locationID, "", map[string]interface{}{
			"palette_preset": preset.Name,
		})
	}
	saved, err := h.settings.GetLocationSettings(r.Context(), locationID)
	if err != nil {
		JSON(w, http.StatusOK, settings)
		return
	}
	JSON(w, http.StatusOK, saved)
}
