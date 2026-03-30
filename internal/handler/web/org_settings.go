package webhandler

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

// ── Organization Settings (org_admin only) ───────────────────

// OrgSettingsPage renders the org settings page (GET /settings/organization).
func (h *Handler) OrgSettingsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		h.renderError(w, r, http.StatusBadRequest, "No location selected", "Please select a location.")
		return
	}

	realRole := middleware.GetWebRole(ctx)
	if middleware.RoleRankValue(realRole) < middleware.RoleRankValue("org_admin") {
		h.renderError(w, r, http.StatusForbidden, "Not authorized", "Organization settings require admin access.")
		return
	}

	loc, err := h.locationRepo.GetByID(ctx, locationID)
	if err != nil || loc == nil {
		slog.Error("load location for org settings failed", "location_id", locationID, "error", err)
		h.renderError(w, r, http.StatusNotFound, "Location not found", "")
		return
	}

	orgSettings, err := h.settingsRepo.GetOrgSettings(ctx, loc.OrgID)
	if err != nil {
		slog.Error("load org settings failed", "org_id", loc.OrgID, "error", err)
		orgSettings = model.DefaultOrgSettings()
	}

	org, err := h.orgRepo.GetByID(ctx, loc.OrgID)
	if err != nil {
		slog.Error("load org failed", "org_id", loc.OrgID, "error", err)
	}

	// Ensure the org_admin has an org-scoped membership so the gym switcher works
	user := middleware.GetWebUser(ctx)
	if user != nil {
		if err := h.orgRepo.EnsureOrgScopedMembership(ctx, user.ID, loc.OrgID, realRole); err != nil {
			slog.Error("ensure org membership failed", "user_id", user.ID, "error", err)
		}
	}

	// Load all gyms under this org for the gym list
	orgLocations, err := h.locationRepo.ListByOrg(ctx, loc.OrgID)
	if err != nil {
		slog.Error("load org locations failed", "org_id", loc.OrgID, "error", err)
	}

	data := &PageData{
		TemplateData:    templateDataFromContext(r, "org-settings"),
		OrgSettingsData: &orgSettings,
		OrgName:         "",
		OrgLocations:    orgLocations,
		SettingsSuccess: r.URL.Query().Get("saved") == "1" || r.URL.Query().Get("gym_created") == "1",
	}
	if org != nil {
		data.OrgName = org.Name
	}
	h.render(w, r, "setter/org-settings.html", data)
}

// OrgSettingsSave handles the org settings form (POST /settings/organization).
func (h *Handler) OrgSettingsSave(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	if locationID == "" {
		h.renderError(w, r, http.StatusBadRequest, "No location selected", "")
		return
	}

	realRole := middleware.GetWebRole(ctx)
	if middleware.RoleRankValue(realRole) < middleware.RoleRankValue("org_admin") {
		h.renderError(w, r, http.StatusForbidden, "Not authorized", "Organization settings require admin access.")
		return
	}

	loc, _ := h.locationRepo.GetByID(ctx, locationID)
	if loc == nil {
		h.renderError(w, r, http.StatusNotFound, "Location not found", "")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Invalid form", "")
		return
	}

	settings, _ := h.settingsRepo.GetOrgSettings(ctx, loc.OrgID)

	// Permissions
	settings.Permissions.HeadSetterCanEditGrading = r.FormValue("hs_edit_grading") == "on"
	settings.Permissions.HeadSetterCanEditCircuits = r.FormValue("hs_edit_circuits") == "on"
	settings.Permissions.HeadSetterCanEditHoldColors = r.FormValue("hs_edit_hold_colors") == "on"
	settings.Permissions.HeadSetterCanEditDisplay = r.FormValue("hs_edit_display") == "on"
	settings.Permissions.HeadSetterCanEditSessions = r.FormValue("hs_edit_sessions") == "on"

	// Defaults
	if v := r.FormValue("default_boulder_method"); v == "v_scale" || v == "circuit" || v == "both" {
		settings.Defaults.BoulderMethod = v
	}
	if v := r.FormValue("default_route_grade_format"); v == "plus_minus" || v == "letter" {
		settings.Defaults.RouteGradeFormat = v
	}
	settings.Defaults.ShowGradesOnCircuit = r.FormValue("default_show_grades_on_circuit") == "on"

	if err := h.settingsRepo.UpdateOrgSettings(ctx, loc.OrgID, settings); err != nil {
		slog.Error("save org settings failed", "org_id", loc.OrgID, "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Save failed", "Could not save organization settings.")
		return
	}

	w.Header().Set("HX-Redirect", "/settings/organization?saved=1")
	w.WriteHeader(http.StatusOK)
}

// ── Team Management ──────────────────────────────────────────

// TeamPage renders the team management page (GET /settings/team).
// Head setters can promote climbers to setter and demote setters to climber.
func (h *Handler) TeamPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	role := middleware.GetWebRole(ctx)

	if middleware.RoleRankValue(role) < 3 {
		h.renderError(w, r, http.StatusForbidden, "Not allowed", "Only head setters and above can manage the team.")
		return
	}

	q := r.URL.Query()
	page := 1
	if v, err := strconv.Atoi(q.Get("page")); err == nil && v > 0 {
		page = v
	}
	const perPage = 50
	params := repository.MemberSearchParams{
		Query:      strings.TrimSpace(q.Get("q")),
		RoleFilter: q.Get("role"),
		Limit:      perPage,
		Offset:     (page - 1) * perPage,
	}

	result, err := h.userRepo.SearchMembersByLocation(ctx, locationID, params)
	if err != nil {
		slog.Error("load team members failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Could not load team members.")
		return
	}

	totalPages := (result.TotalCount + perPage - 1) / perPage
	if totalPages < 1 {
		totalPages = 1
	}

	data := &PageData{
		TemplateData:    templateDataFromContext(r, "settings"),
		TeamMembers:     result.Members,
		TeamTotalCount:  result.TotalCount,
		TeamPage:        page,
		TeamTotalPages:  totalPages,
		TeamQuery:       params.Query,
		TeamRoleFilter:  params.RoleFilter,
		IsManager:       middleware.RoleRankValue(role) >= 4,
		SettingsSuccess: r.URL.Query().Get("saved") == "1",
	}

	h.render(w, r, "setter/team.html", data)
}

// TeamUpdateRole changes a member's role (POST /settings/team/{membershipID}/role).
func (h *Handler) TeamUpdateRole(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	membershipID := chi.URLParam(r, "membershipID")
	role := middleware.GetWebRole(ctx)

	if middleware.RoleRankValue(role) < 3 {
		h.renderError(w, r, http.StatusForbidden, "Not allowed", "Only head setters and above can change roles.")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Invalid form", "Could not parse form data.")
		return
	}

	newRole := r.FormValue("role")
	// Head setters can only assign: climber <-> setter
	// Managers+ can assign: climber, setter, head_setter
	allowedRoles := map[string]bool{"climber": true, "setter": true}
	if middleware.RoleRankValue(role) >= 4 {
		allowedRoles["head_setter"] = true
	}
	if !allowedRoles[newRole] {
		h.renderError(w, r, http.StatusForbidden, "Not allowed", "You cannot assign that role.")
		return
	}

	if err := h.userRepo.UpdateMemberRole(ctx, membershipID, newRole); err != nil {
		slog.Error("update member role failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Update failed", "Could not update role.")
		return
	}

	w.Header().Set("HX-Redirect", "/settings/team?saved=1")
	w.WriteHeader(http.StatusOK)
}

// OrgTeamPage renders the organization team management page (GET /settings/organization/team).
func (h *Handler) OrgTeamPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	locationID := middleware.GetWebLocationID(ctx)
	role := middleware.GetWebRole(ctx)

	if middleware.RoleRankValue(role) < 5 {
		h.renderError(w, r, http.StatusForbidden, "Not allowed", "Only organization admins can manage the org team.")
		return
	}

	loc, err := h.locationRepo.GetByID(ctx, locationID)
	if err != nil || loc == nil {
		h.renderError(w, r, http.StatusNotFound, "Location not found", "")
		return
	}

	q := r.URL.Query()
	page := 1
	if v, err := strconv.Atoi(q.Get("page")); err == nil && v > 0 {
		page = v
	}
	const perPage = 50
	params := repository.MemberSearchParams{
		Query:      strings.TrimSpace(q.Get("q")),
		RoleFilter: q.Get("role"),
		Limit:      perPage,
		Offset:     (page - 1) * perPage,
	}

	result, err := h.userRepo.SearchMembersByOrg(ctx, loc.OrgID, params)
	if err != nil {
		slog.Error("load org team members failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Something went wrong", "Could not load team members.")
		return
	}

	totalPages := (result.TotalCount + perPage - 1) / perPage
	if totalPages < 1 {
		totalPages = 1
	}

	data := &PageData{
		TemplateData:    templateDataFromContext(r, "org-settings"),
		TeamMembers:     result.Members,
		TeamTotalCount:  result.TotalCount,
		TeamPage:        page,
		TeamTotalPages:  totalPages,
		TeamQuery:       params.Query,
		TeamRoleFilter:  params.RoleFilter,
		IsManager:       true,
		SettingsSuccess: r.URL.Query().Get("saved") == "1",
	}

	h.render(w, r, "setter/org-team.html", data)
}

// OrgTeamUpdateRole changes a member's role at org level (POST /settings/organization/team/{membershipID}/role).
func (h *Handler) OrgTeamUpdateRole(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	membershipID := chi.URLParam(r, "membershipID")
	role := middleware.GetWebRole(ctx)

	if middleware.RoleRankValue(role) < 5 {
		h.renderError(w, r, http.StatusForbidden, "Not allowed", "Only organization admins can change roles at org level.")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, http.StatusBadRequest, "Invalid form", "Could not parse form data.")
		return
	}

	newRole := r.FormValue("role")
	allowedRoles := map[string]bool{"climber": true, "setter": true, "head_setter": true, "gym_manager": true}
	if !allowedRoles[newRole] {
		h.renderError(w, r, http.StatusForbidden, "Not allowed", "You cannot assign that role.")
		return
	}

	if err := h.userRepo.UpdateMemberRole(ctx, membershipID, newRole); err != nil {
		slog.Error("update org member role failed", "error", err)
		h.renderError(w, r, http.StatusInternalServerError, "Update failed", "Could not update role.")
		return
	}

	// gym_manager needs org-scoped membership to see all locations
	if newRole == "gym_manager" {
		membership, err := h.userRepo.GetMembershipByID(ctx, membershipID)
		if err == nil && membership != nil {
			if err := h.orgRepo.EnsureOrgScopedMembership(ctx, membership.UserID, membership.OrgID, newRole); err != nil {
				slog.Error("ensure org membership for gym_manager failed", "user_id", membership.UserID, "error", err)
			}
		}
	}

	w.Header().Set("HX-Redirect", "/settings/organization/team?saved=1")
	w.WriteHeader(http.StatusOK)
}
