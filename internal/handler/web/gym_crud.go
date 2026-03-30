package webhandler

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

// ── Gym Creation (org_admin only) ────────────────────────────

// GymNewPage renders the new gym form (GET /settings/organization/gyms/new).
func (h *Handler) GymNewPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		h.renderError(w, r, http.StatusBadRequest, "No location selected", "")
		return
	}

	role := middleware.GetWebRole(ctx)
	if middleware.RoleRankValue(role) < middleware.RoleRankValue("org_admin") {
		h.renderError(w, r, http.StatusForbidden, "Not authorized", "Only organization admins can create new gyms.")
		return
	}

	loc, _ := h.locationRepo.GetByID(ctx, locationID)
	orgName := ""
	if loc != nil {
		if org, _ := h.orgRepo.GetByID(ctx, loc.OrgID); org != nil {
			orgName = org.Name
		}
	}

	data := &PageData{
		TemplateData: templateDataFromContext(r, "org-settings"),
		OrgName:      orgName,
		GymForm:      GymFormValues{},
	}
	h.render(w, r, "setter/gym-new.html", data)
}

// GymCreate handles the gym creation form (POST /settings/organization/gyms/new).
func (h *Handler) GymCreate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		h.renderError(w, r, http.StatusBadRequest, "No location selected", "")
		return
	}

	role := middleware.GetWebRole(ctx)
	if middleware.RoleRankValue(role) < middleware.RoleRankValue("org_admin") {
		h.renderError(w, r, http.StatusForbidden, "Not authorized", "Only organization admins can create new gyms.")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Invalid form", "Could not parse form data.")
		return
	}

	// Find the org ID from the current location
	loc, _ := h.locationRepo.GetByID(ctx, locationID)
	if loc == nil {
		h.renderError(w, r, http.StatusNotFound, "Location not found", "")
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	slug := strings.TrimSpace(r.FormValue("slug"))
	address := strings.TrimSpace(r.FormValue("address"))
	timezone := strings.TrimSpace(r.FormValue("timezone"))
	websiteURL := strings.TrimSpace(r.FormValue("website_url"))
	phone := strings.TrimSpace(r.FormValue("phone"))
	dayPassInfo := strings.TrimSpace(r.FormValue("day_pass_info"))

	if name == "" {
		orgName := ""
		if org, _ := h.orgRepo.GetByID(ctx, loc.OrgID); org != nil {
			orgName = org.Name
		}
		data := &PageData{
			TemplateData: templateDataFromContext(r, "org-settings"),
			OrgName:      orgName,
			FormError:    "Gym name is required.",
			GymForm: GymFormValues{
				Name: name, Slug: slug, Address: address, Timezone: timezone,
				WebsiteURL: websiteURL, Phone: phone, DayPassInfo: dayPassInfo,
			},
		}
		h.render(w, r, "setter/gym-new.html", data)
		return
	}

	if slug == "" {
		slug = gymSlugify(name)
	}
	if timezone == "" {
		timezone = "America/Los_Angeles"
	}

	newLoc := &model.Location{
		OrgID:    loc.OrgID,
		Name:     name,
		Slug:     slug,
		Timezone: timezone,
	}
	if address != "" {
		newLoc.Address = &address
	}
	if websiteURL != "" {
		newLoc.WebsiteURL = &websiteURL
	}
	if phone != "" {
		newLoc.Phone = &phone
	}
	if dayPassInfo != "" {
		newLoc.DayPassInfo = &dayPassInfo
	}

	if err := h.locationRepo.Create(ctx, newLoc); err != nil {
		slog.Error("create gym failed", "error", err)

		orgName := ""
		if org, _ := h.orgRepo.GetByID(ctx, loc.OrgID); org != nil {
			orgName = org.Name
		}
		errMsg := "Could not create gym. The slug may already be in use."
		data := &PageData{
			TemplateData: templateDataFromContext(r, "org-settings"),
			OrgName:      orgName,
			FormError:    errMsg,
			GymForm: GymFormValues{
				Name: name, Slug: slug, Address: address, Timezone: timezone,
				WebsiteURL: websiteURL, Phone: phone, DayPassInfo: dayPassInfo,
			},
		}
		h.render(w, r, "setter/gym-new.html", data)
		return
	}

	// Apply org-level grading defaults to the new location's settings.
	applyOrgDefaults(ctx, h.settingsRepo, loc.OrgID, newLoc.ID)

	slog.Info("new gym created", "location_id", newLoc.ID, "name", name, "org_id", loc.OrgID)

	// Ensure the creator has an org-scoped membership so the gym switcher
	// shows all locations (not just the one they were originally tied to).
	user := middleware.GetWebUser(ctx)
	if user != nil {
		if err := h.orgRepo.EnsureOrgScopedMembership(ctx, user.ID, loc.OrgID, "org_admin"); err != nil {
			slog.Error("ensure org membership failed", "user_id", user.ID, "error", err)
		}
	}

	// Redirect to the org settings page with success message
	w.Header().Set("HX-Redirect", "/settings/organization?gym_created=1")
	w.WriteHeader(http.StatusOK)
}

// ── Gym Editing (org_admin only) ─────────────────────────────

// GymEditPage renders the gym edit form (GET /settings/organization/gyms/{gymID}/edit).
func (h *Handler) GymEditPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	role := middleware.GetWebRole(ctx)
	if middleware.RoleRankValue(role) < middleware.RoleRankValue("org_admin") {
		h.renderError(w, r, http.StatusForbidden, "Not authorized", "Only organization admins can edit gyms.")
		return
	}

	gymID := chi.URLParam(r, "gymID")
	gym, err := h.locationRepo.GetByID(ctx, gymID)
	if err != nil || gym == nil {
		h.renderError(w, r, http.StatusNotFound, "Gym not found", "")
		return
	}

	orgName := ""
	if org, _ := h.orgRepo.GetByID(ctx, gym.OrgID); org != nil {
		orgName = org.Name
	}

	data := &PageData{
		TemplateData: templateDataFromContext(r, "org-settings"),
		OrgName:      orgName,
		EditLocation: gym,
		GymForm:      gymFormFromLocation(gym),
	}

	if r.URL.Query().Get("saved") == "1" {
		data.FormSuccess = "Gym updated."
	}

	h.render(w, r, "setter/gym-edit.html", data)
}

// GymUpdate handles the gym edit form (POST /settings/organization/gyms/{gymID}/edit).
func (h *Handler) GymUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	role := middleware.GetWebRole(ctx)
	if middleware.RoleRankValue(role) < middleware.RoleRankValue("org_admin") {
		h.renderError(w, r, http.StatusForbidden, "Not authorized", "Only organization admins can edit gyms.")
		return
	}

	gymID := chi.URLParam(r, "gymID")
	gym, err := h.locationRepo.GetByID(ctx, gymID)
	if err != nil || gym == nil {
		h.renderError(w, r, http.StatusNotFound, "Gym not found", "")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Invalid form", "Could not parse form data.")
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	slug := strings.TrimSpace(r.FormValue("slug"))
	address := strings.TrimSpace(r.FormValue("address"))
	timezone := strings.TrimSpace(r.FormValue("timezone"))
	websiteURL := strings.TrimSpace(r.FormValue("website_url"))
	phone := strings.TrimSpace(r.FormValue("phone"))
	dayPassInfo := strings.TrimSpace(r.FormValue("day_pass_info"))

	if name == "" {
		orgName := ""
		if org, _ := h.orgRepo.GetByID(ctx, gym.OrgID); org != nil {
			orgName = org.Name
		}
		data := &PageData{
			TemplateData: templateDataFromContext(r, "org-settings"),
			OrgName:      orgName,
			EditLocation: gym,
			GymForm: GymFormValues{
				Name: name, Slug: slug, Address: address, Timezone: timezone,
				WebsiteURL: websiteURL, Phone: phone, DayPassInfo: dayPassInfo,
			},
			FormError: "Gym name is required.",
		}
		h.render(w, r, "setter/gym-edit.html", data)
		return
	}

	if slug == "" {
		slug = gymSlugify(name)
	}
	if timezone == "" {
		timezone = gym.Timezone
	}

	// Update fields
	gym.Name = name
	gym.Slug = slug
	gym.Timezone = timezone
	gym.Address = nilIfEmpty(address)
	gym.WebsiteURL = nilIfEmpty(websiteURL)
	gym.Phone = nilIfEmpty(phone)
	gym.DayPassInfo = nilIfEmpty(dayPassInfo)

	if err := h.locationRepo.Update(ctx, gym); err != nil {
		slog.Error("update gym failed", "error", err)

		orgName := ""
		if org, _ := h.orgRepo.GetByID(ctx, gym.OrgID); org != nil {
			orgName = org.Name
		}
		data := &PageData{
			TemplateData: templateDataFromContext(r, "org-settings"),
			OrgName:      orgName,
			EditLocation: gym,
			GymForm: GymFormValues{
				Name: name, Slug: slug, Address: address, Timezone: timezone,
				WebsiteURL: websiteURL, Phone: phone, DayPassInfo: dayPassInfo,
			},
			FormError: "Could not save changes. The slug may already be in use.",
		}
		h.render(w, r, "setter/gym-edit.html", data)
		return
	}

	slog.Info("gym updated", "location_id", gym.ID, "name", name)
	w.Header().Set("HX-Redirect", "/settings/organization/gyms/"+gym.ID+"/edit?saved=1")
	w.WriteHeader(http.StatusOK)
}

// gymFormFromLocation populates GymFormValues from a Location model.
func gymFormFromLocation(l *model.Location) GymFormValues {
	gf := GymFormValues{
		Name:     l.Name,
		Slug:     l.Slug,
		Timezone: l.Timezone,
	}
	if l.Address != nil {
		gf.Address = *l.Address
	}
	if l.WebsiteURL != nil {
		gf.WebsiteURL = *l.WebsiteURL
	}
	if l.Phone != nil {
		gf.Phone = *l.Phone
	}
	if l.DayPassInfo != nil {
		gf.DayPassInfo = *l.DayPassInfo
	}
	return gf
}

// nilIfEmpty returns a pointer to s if non-empty, nil otherwise.
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// applyOrgDefaults reads the organization's default grading preferences and
// applies them to a newly created location's settings. Non-fatal — if anything
// fails the location just keeps the hardcoded defaults.
func applyOrgDefaults(ctx context.Context, settingsRepo *repository.SettingsRepo, orgID, locationID string) {
	orgSettings, err := settingsRepo.GetOrgSettings(ctx, orgID)
	if err != nil {
		slog.Error("load org defaults for new gym", "org_id", orgID, "error", err)
		return
	}

	locSettings := model.DefaultLocationSettings()
	locSettings.Grading.BoulderMethod = orgSettings.Defaults.BoulderMethod
	locSettings.Grading.RouteGradeFormat = orgSettings.Defaults.RouteGradeFormat
	locSettings.Grading.ShowGradesOnCircuit = orgSettings.Defaults.ShowGradesOnCircuit

	if err := settingsRepo.UpdateLocationSettings(ctx, locationID, locSettings); err != nil {
		slog.Error("apply org defaults to new gym", "location_id", locationID, "error", err)
	}
}

// gymSlugify creates a URL-safe slug from a string.
func gymSlugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	result := make([]byte, 0, len(s))
	lastDash := false
	for _, b := range []byte(s) {
		if (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') {
			result = append(result, b)
			lastDash = false
		} else if !lastDash && len(result) > 0 {
			result = append(result, '-')
			lastDash = true
		}
	}
	return strings.Trim(string(result), "-")
}
