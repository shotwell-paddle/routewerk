package webhandler

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

// SetupPage renders the first-run setup form (GET /setup).
// This page is only accessible when no organizations exist yet.
func (h *Handler) SetupPage(w http.ResponseWriter, r *http.Request) {
	if !h.needsSetup(r) {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
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
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
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

	// Create the organization.
	org := &model.Organization{
		Name: orgName,
		Slug: gymSlugify(orgName),
	}
	if err := h.orgRepo.Create(ctx, org); err != nil {
		slog.Error("setup: failed to create org", "error", err)
		h.renderSetup(w, r, "Something went wrong. Please try again.", user.DisplayName, orgName, gymName, address, timezone)
		return
	}

	// Create the first location.
	loc := &model.Location{
		OrgID:    org.ID,
		Name:     gymName,
		Slug:     gymSlugify(gymName),
		Timezone: timezone,
	}
	if address != "" {
		loc.Address = &address
	}
	if err := h.locationRepo.Create(ctx, loc); err != nil {
		slog.Error("setup: failed to create location", "error", err)
		h.renderSetup(w, r, "Something went wrong. Please try again.", user.DisplayName, orgName, gymName, address, timezone)
		return
	}

	// Apply org-level grading defaults to the new location.
	applyOrgDefaults(ctx, h.settingsRepo, org.ID, loc.ID)

	// Create org_admin membership for the current user (org-scoped, no location_id).
	membership := &model.UserMembership{
		UserID: user.ID,
		OrgID:  org.ID,
		Role:   "org_admin",
	}
	if err := h.orgRepo.AddMember(ctx, membership); err != nil {
		slog.Error("setup: failed to create admin membership", "error", err)
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

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
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
