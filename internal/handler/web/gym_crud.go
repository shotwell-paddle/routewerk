package webhandler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/shotwell-paddle/routewerk/internal/database"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
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

	user := middleware.GetWebUser(ctx)

	// Run location creation, settings defaults, and membership update in a transaction.
	err := database.RunInTx(ctx, h.db, func(tx pgx.Tx) error {
		// Create location
		err := tx.QueryRow(ctx,
			`INSERT INTO locations (org_id, name, slug, address, timezone, website_url, phone, hours_json, day_pass_info, waiver_url, allow_shared_setters, custom_domain) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING id, created_at, updated_at`,
			newLoc.OrgID, newLoc.Name, newLoc.Slug, newLoc.Address, newLoc.Timezone, newLoc.WebsiteURL, newLoc.Phone, newLoc.HoursJSON, newLoc.DayPassInfo, newLoc.WaiverURL, newLoc.AllowSharedSetters, newLoc.CustomDomain,
		).Scan(&newLoc.ID, &newLoc.CreatedAt, &newLoc.UpdatedAt)
		if err != nil {
			return fmt.Errorf("create location: %w", err)
		}

		// Apply org-level grading defaults
		var raw []byte
		err = tx.QueryRow(ctx,
			`SELECT COALESCE(settings_json, '{}'::jsonb) FROM organizations WHERE id = $1 AND deleted_at IS NULL`,
			loc.OrgID,
		).Scan(&raw)
		if err != nil {
			return fmt.Errorf("get org settings: %w", err)
		}

		orgSettings := model.DefaultOrgSettings()
		if len(raw) > 2 {
			if err := json.Unmarshal(raw, &orgSettings); err != nil {
				return fmt.Errorf("parse org settings: %w", err)
			}
		}

		locSettings := model.DefaultLocationSettings()
		locSettings.Grading.BoulderMethod = orgSettings.Defaults.BoulderMethod
		locSettings.Grading.RouteGradeFormat = orgSettings.Defaults.RouteGradeFormat
		locSettings.Grading.ShowGradesOnCircuit = orgSettings.Defaults.ShowGradesOnCircuit

		settingsJSON, _ := json.Marshal(locSettings)
		_, err = tx.Exec(ctx,
			`UPDATE locations SET settings_json = $1, updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL`,
			settingsJSON, newLoc.ID,
		)
		if err != nil {
			return fmt.Errorf("apply defaults: %w", err)
		}

		// Ensure creator has org-scoped membership
		if user != nil {
			// Check if an org-scoped membership already exists
			var exists bool
			err = tx.QueryRow(ctx, `
				SELECT EXISTS(
					SELECT 1 FROM user_memberships
					WHERE user_id = $1 AND org_id = $2 AND location_id IS NULL AND deleted_at IS NULL
				)`, user.ID, loc.OrgID).Scan(&exists)
			if err != nil {
				return fmt.Errorf("check org membership: %w", err)
			}

			if !exists {
				// Upgrade the user's location-scoped membership to org-scoped
				_, err = tx.Exec(ctx, `
					UPDATE user_memberships
					SET location_id = NULL, role = $3, updated_at = NOW()
					WHERE id = (
						SELECT id FROM user_memberships
						WHERE user_id = $1 AND org_id = $2 AND deleted_at IS NULL
						ORDER BY CASE role
							WHEN 'org_admin' THEN 1
							WHEN 'gym_manager' THEN 2
							WHEN 'head_setter' THEN 3
							WHEN 'setter' THEN 4
							ELSE 5
						END
						LIMIT 1
					)`, user.ID, loc.OrgID, "org_admin")
				if err != nil {
					return fmt.Errorf("upgrade to org membership: %w", err)
				}
			}
		}

		return nil
	})
	if err != nil {
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

	slog.Info("new gym created", "location_id", newLoc.ID, "name", name, "org_id", loc.OrgID)

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
	customDomain := strings.ToLower(strings.TrimSpace(r.FormValue("custom_domain")))

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
				CustomDomain: customDomain,
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
	gym.CustomDomain = nilIfEmpty(customDomain)

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
				CustomDomain: customDomain,
			},
			FormError: "Could not save changes. The slug or custom domain may already be in use.",
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
	if l.CustomDomain != nil {
		gf.CustomDomain = *l.CustomDomain
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
func applyOrgDefaults(ctx context.Context, settingsRepo SettingsStore, orgID, locationID string) {
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
