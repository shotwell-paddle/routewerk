package webhandler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

// ── Gym Settings (head_setter and above) ─────────────────────

// GymSettingsPage renders the gym settings page (GET /settings).
func (h *Handler) GymSettingsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		h.renderError(w, r, http.StatusBadRequest, "No location selected", "Please select a location.")
		return
	}

	realRole := middleware.GetWebRole(ctx)
	if middleware.RoleRankValue(realRole) < middleware.RoleRankValue("head_setter") {
		h.renderError(w, r, http.StatusForbidden, "Not authorized", "You need to be a head setter or above to access settings.")
		return
	}

	settings, err := h.settingsRepo.GetLocationSettings(ctx, locationID)
	if err != nil {
		slog.Error("load gym settings failed", "location_id", locationID, "error", err)
		settings = model.DefaultLocationSettings()
	}

	// Load org permissions to know what sections are editable
	loc, _ := h.locationRepo.GetByID(ctx, locationID)
	orgPerms := model.DefaultOrgSettings().Permissions
	if loc != nil {
		orgSettings, oErr := h.settingsRepo.GetOrgSettings(ctx, loc.OrgID)
		if oErr == nil {
			orgPerms = orgSettings.Permissions
		}
	}

	// If user is org_admin or gym_manager, they bypass org permission restrictions
	isManager := middleware.RoleRankValue(realRole) >= middleware.RoleRankValue("gym_manager")

	data := &PageData{
		TemplateData:    templateDataFromContext(r, "settings"),
		GymSettings:     &settings,
		OrgPermissions:  &orgPerms,
		IsManager:       isManager,
		SettingsSuccess: r.URL.Query().Get("saved") == "1",
	}
	h.render(w, r, "setter/settings.html", data)
}

// GymSettingsSave handles the settings form (POST /settings).
func (h *Handler) GymSettingsSave(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		h.renderError(w, r, http.StatusBadRequest, "No location selected", "Please select a location.")
		return
	}

	realRole := middleware.GetWebRole(ctx)
	if middleware.RoleRankValue(realRole) < middleware.RoleRankValue("head_setter") {
		h.renderError(w, r, http.StatusForbidden, "Not authorized", "Settings require head setter access.")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Invalid form", "Could not parse form data.")
		return
	}

	// Load current settings as base
	settings, _ := h.settingsRepo.GetLocationSettings(ctx, locationID)

	// Determine which section is being saved
	section := r.FormValue("section")

	switch section {
	case "grading":
		h.saveGradingSettings(r, &settings)
	case "circuits":
		h.saveCircuitSettings(r, &settings)
	case "display":
		h.saveDisplaySettings(r, &settings)
	case "sessions":
		h.saveSessionSettings(r, &settings)
	default:
		// Save all sections (full form)
		h.saveGradingSettings(r, &settings)
		h.saveCircuitSettings(r, &settings)
		h.saveDisplaySettings(r, &settings)
		h.saveSessionSettings(r, &settings)
	}

	if err := h.settingsRepo.UpdateLocationSettings(ctx, locationID, settings); err != nil {
		slog.Error("save gym settings failed", "location_id", locationID, "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Save failed", "Could not save settings.")
		return
	}

	// Redirect back with success indicator
	w.Header().Set("HX-Redirect", "/settings?saved=1")
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) saveGradingSettings(r *http.Request, s *model.LocationSettings) {
	if v := r.FormValue("boulder_method"); v == "v_scale" || v == "circuit" || v == "both" {
		s.Grading.BoulderMethod = v
	}
	if v := r.FormValue("route_grade_format"); v == "plus_minus" || v == "letter" {
		s.Grading.RouteGradeFormat = v
	}
	s.Grading.ShowGradesOnCircuit = r.FormValue("show_grades_on_circuit") == "on"
}

func (h *Handler) saveCircuitSettings(r *http.Request, s *model.LocationSettings) {
	// Circuit colors come as JSON array from the drag-and-drop reorder UI
	colorsJSON := r.FormValue("circuit_colors_json")
	if colorsJSON != "" {
		var colors []model.CircuitColor
		if err := json.Unmarshal([]byte(colorsJSON), &colors); err == nil && len(colors) > 0 {
			// Re-index sort_order
			for i := range colors {
				colors[i].SortOrder = i
				colors[i].Name = strings.TrimSpace(colors[i].Name)
				if !validHexColor.MatchString(colors[i].Hex) {
					colors[i].Hex = "#999999"
				}
			}
			s.Circuits.Colors = colors
		}
	}
}

func (h *Handler) saveDisplaySettings(r *http.Request, s *model.LocationSettings) {
	s.Display.ShowSetterName = r.FormValue("show_setter_name") == "on"
	s.Display.ShowRouteAge = r.FormValue("show_route_age") == "on"
	s.Display.ShowDifficultyConsensus = r.FormValue("show_difficulty_consensus") == "on"

	if v := r.FormValue("default_strip_age_days"); v != "" {
		if days, err := strconv.Atoi(v); err == nil && days >= 0 && days <= 365 {
			s.Display.DefaultStripAgeDays = days
		}
	}
}

func (h *Handler) saveSessionSettings(r *http.Request, s *model.LocationSettings) {
	s.Sessions.DefaultPlaybookEnabled = r.FormValue("default_playbook_enabled") == "on"
	s.Sessions.RequireRoutePhoto = r.FormValue("require_route_photo") == "on"
}

// ── Add Circuit Color ────────────────────────────────────────

// GymSettingsAddCircuit adds a new circuit color (POST /settings/circuits/add).
func (h *Handler) GymSettingsAddCircuit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		http.Error(w, "No location", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("color_name"))
	hex := strings.TrimSpace(r.FormValue("color_hex"))

	if name == "" || !validHexColor.MatchString(hex) {
		http.Error(w, "Invalid color", http.StatusBadRequest)
		return
	}

	// Sanitize name: lowercase, alphanumeric + spaces only
	name = strings.ToLower(regexp.MustCompile(`[^a-z0-9 ]`).ReplaceAllString(name, ""))
	if name == "" {
		http.Error(w, "Invalid color name", http.StatusBadRequest)
		return
	}

	settings, _ := h.settingsRepo.GetLocationSettings(ctx, locationID)

	// Check for duplicate name
	for _, c := range settings.Circuits.Colors {
		if c.Name == name {
			http.Error(w, "Color already exists", http.StatusConflict)
			return
		}
	}

	settings.Circuits.Colors = append(settings.Circuits.Colors, model.CircuitColor{
		Name:      name,
		Hex:       hex,
		SortOrder: len(settings.Circuits.Colors),
	})

	if err := h.settingsRepo.UpdateLocationSettings(ctx, locationID, settings); err != nil {
		slog.Error("add circuit color failed", "error", err)
		http.Error(w, "Save failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Redirect", "/settings?saved=1")
	w.WriteHeader(http.StatusOK)
}

// GymSettingsRemoveCircuit removes a circuit color (POST /settings/circuits/{name}/delete).
func (h *Handler) GymSettingsRemoveCircuit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		http.Error(w, "No location", http.StatusBadRequest)
		return
	}

	colorName := chi.URLParam(r, "colorName")
	if colorName == "" {
		http.Error(w, "Invalid color name", http.StatusBadRequest)
		return
	}

	settings, _ := h.settingsRepo.GetLocationSettings(ctx, locationID)

	// Remove the color and re-index
	var newColors []model.CircuitColor
	found := false
	for _, c := range settings.Circuits.Colors {
		if c.Name == colorName {
			found = true
			continue
		}
		c.SortOrder = len(newColors)
		newColors = append(newColors, c)
	}

	if !found {
		http.Error(w, "Color not found", http.StatusNotFound)
		return
	}

	settings.Circuits.Colors = newColors
	if err := h.settingsRepo.UpdateLocationSettings(ctx, locationID, settings); err != nil {
		slog.Error("remove circuit color failed", "error", err)
		http.Error(w, "Save failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Redirect", "/settings?saved=1")
	w.WriteHeader(http.StatusOK)
}

// ── Hold Color Management ────────────────────────────────────

// GymSettingsAddHoldColor adds a new hold color (POST /settings/hold-colors/add).
func (h *Handler) GymSettingsAddHoldColor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		http.Error(w, "No location", http.StatusBadRequest)
		return
	}

	realRole := middleware.GetWebRole(ctx)
	if middleware.RoleRankValue(realRole) < middleware.RoleRankValue("head_setter") {
		http.Error(w, "Not authorized", http.StatusForbidden)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("color_name"))
	hex := strings.TrimSpace(r.FormValue("color_hex"))

	if name == "" || !validHexColor.MatchString(hex) {
		http.Error(w, "Invalid color", http.StatusBadRequest)
		return
	}

	// Title case the name for display
	name = strings.Title(strings.ToLower(name))

	settings, _ := h.settingsRepo.GetLocationSettings(ctx, locationID)

	// Check for duplicate name
	for _, c := range settings.HoldColors.Colors {
		if strings.EqualFold(c.Name, name) {
			http.Error(w, "Color already exists", http.StatusConflict)
			return
		}
	}

	settings.HoldColors.Colors = append(settings.HoldColors.Colors, model.HoldColor{
		Name: name,
		Hex:  hex,
	})

	if err := h.settingsRepo.UpdateLocationSettings(ctx, locationID, settings); err != nil {
		slog.Error("add hold color failed", "error", err)
		http.Error(w, "Save failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Redirect", "/settings?saved=1")
	w.WriteHeader(http.StatusOK)
}

// GymSettingsRemoveHoldColor removes a hold color (POST /settings/hold-colors/{colorName}/delete).
func (h *Handler) GymSettingsRemoveHoldColor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		http.Error(w, "No location", http.StatusBadRequest)
		return
	}

	realRole := middleware.GetWebRole(ctx)
	if middleware.RoleRankValue(realRole) < middleware.RoleRankValue("head_setter") {
		http.Error(w, "Not authorized", http.StatusForbidden)
		return
	}

	colorName := chi.URLParam(r, "colorName")
	if colorName == "" {
		http.Error(w, "Invalid color name", http.StatusBadRequest)
		return
	}

	settings, _ := h.settingsRepo.GetLocationSettings(ctx, locationID)

	var newColors []model.HoldColor
	found := false
	for _, c := range settings.HoldColors.Colors {
		if strings.EqualFold(c.Name, colorName) {
			found = true
			continue
		}
		newColors = append(newColors, c)
	}

	if !found {
		http.Error(w, "Color not found", http.StatusNotFound)
		return
	}

	settings.HoldColors.Colors = newColors
	if err := h.settingsRepo.UpdateLocationSettings(ctx, locationID, settings); err != nil {
		slog.Error("remove hold color failed", "error", err)
		http.Error(w, "Save failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Redirect", "/settings?saved=1")
	w.WriteHeader(http.StatusOK)
}
