package webhandler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/shotwell-paddle/routewerk/internal/database"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

// SetupPage renders the first-run setup form (GET /setup).
// This page is only accessible when no organizations exist yet.
func (h *Handler) SetupPage(w http.ResponseWriter, r *http.Request) {
	if !h.needsSetup(r) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	user := middleware.GetWebUser(r.Context())
	userName := ""
	if user != nil {
		userName = user.DisplayName
	}

	h.renderSetup(w, r, "", userName, "", "", "", "America/Chicago")
}

// SetupSubmit handles POST /setup — creates the first org, location, and promotes
// the current user to org_admin.
func (h *Handler) SetupSubmit(w http.ResponseWriter, r *http.Request) {
	if !h.needsSetup(r) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	ctx := r.Context()
	user := middleware.GetWebUser(ctx)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderSetup(w, r, "Invalid form data.", user.DisplayName, "", "", "", "America/Chicago")
		return
	}

	orgName := strings.TrimSpace(r.FormValue("org_name"))
	gymName := strings.TrimSpace(r.FormValue("gym_name"))
	address := strings.TrimSpace(r.FormValue("address"))
	timezone := strings.TrimSpace(r.FormValue("timezone"))

	if orgName == "" || gymName == "" {
		h.renderSetup(w, r, "Organization name and gym name are required.", user.DisplayName, orgName, gymName, address, timezone)
		return
	}

	if timezone == "" {
		timezone = "America/Chicago"
	}

	// Run org creation, location creation, defaults, and membership in a single transaction.
	org := &model.Organization{
		Name: orgName,
		Slug: gymSlugify(orgName),
	}
	loc := &model.Location{
		OrgID:    "", // set after org insert
		Name:     gymName,
		Slug:     gymSlugify(gymName),
		Timezone: timezone,
	}
	if address != "" {
		loc.Address = &address
	}

	err := database.RunInTx(ctx, h.db, func(tx pgx.Tx) error {
		// Create org
		err := tx.QueryRow(ctx,
			`INSERT INTO organizations (name, slug, logo_url) VALUES ($1, $2, $3) RETURNING id, created_at, updated_at`,
			org.Name, org.Slug, nil,
		).Scan(&org.ID, &org.CreatedAt, &org.UpdatedAt)
		if err != nil {
			return fmt.Errorf("create org: %w", err)
		}

		// Create location
		loc.OrgID = org.ID
		err = tx.QueryRow(ctx,
			`INSERT INTO locations (org_id, name, slug, address, timezone, website_url, phone, hours_json, day_pass_info, waiver_url, allow_shared_setters, custom_domain) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING id, created_at, updated_at`,
			loc.OrgID, loc.Name, loc.Slug, loc.Address, loc.Timezone, loc.WebsiteURL, loc.Phone, loc.HoursJSON, loc.DayPassInfo, loc.WaiverURL, loc.AllowSharedSetters, loc.CustomDomain,
		).Scan(&loc.ID, &loc.CreatedAt, &loc.UpdatedAt)
		if err != nil {
			return fmt.Errorf("create location: %w", err)
		}

		// Apply org-level grading defaults
		defaults := model.DefaultLocationSettings()
		settingsJSON, _ := json.Marshal(defaults)
		_, err = tx.Exec(ctx,
			`UPDATE locations SET settings_json = $1, updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL`,
			settingsJSON, loc.ID,
		)
		if err != nil {
			return fmt.Errorf("apply defaults: %w", err)
		}

		// Create org_admin membership
		_, err = tx.Exec(ctx,
			`INSERT INTO user_memberships (user_id, org_id, location_id, role, specialties) VALUES ($1, $2, $3, $4, $5)`,
			user.ID, org.ID, nil, "org_admin", nil,
		)
		if err != nil {
			return fmt.Errorf("create membership: %w", err)
		}

		return nil
	})
	if err != nil {
		slog.Error("setup: transaction failed", "error", err)
		h.renderSetup(w, r, "Something went wrong. Please try again.", user.DisplayName, orgName, gymName, address, timezone)
		return
	}

	// Update the web session to point at the new location.
	session := middleware.GetWebSession(ctx)
	if session != nil {
		if err := h.webSessionRepo.UpdateLocation(ctx, session.ID, loc.ID); err != nil {
			slog.Error("setup: failed to update session location", "error", err)
		}
	}

	slog.Info("first-run setup complete",
		"org_id", org.ID, "org_name", orgName,
		"location_id", loc.ID, "gym_name", gymName,
		"admin_user_id", user.ID,
	)

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// needsSetup returns true if no organizations exist (first-run state).
func (h *Handler) needsSetup(r *http.Request) bool {
	count, err := h.orgRepo.Count(r.Context())
	if err != nil {
		slog.Error("setup: failed to count orgs", "error", err)
		return false
	}
	return count == 0
}

// NeedsSetup is the exported version for use by the router/middleware.
func (h *Handler) NeedsSetup(r *http.Request) bool {
	return h.needsSetup(r)
}

func (h *Handler) renderSetup(w http.ResponseWriter, r *http.Request, errMsg, userName, orgName, gymName, address, timezone string) {
	tmpl, ok := h.templates["auth/setup.html"]
	if !ok {
		http.Error(w, "setup template not found", http.StatusInternalServerError)
		return
	}

	data := struct {
		CSRFToken string
		Error     string
		UserName  string
		OrgName   string
		GymName   string
		Address   string
		Timezone  string
	}{
		CSRFToken: middleware.TokenFromRequest(r),
		Error:     errMsg,
		UserName:  userName,
		OrgName:   orgName,
		GymName:   gymName,
		Address:   address,
		Timezone:  timezone,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		slog.Error("setup render error", "error", err)
	}
}
