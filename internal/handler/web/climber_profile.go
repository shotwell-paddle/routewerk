package webhandler

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
	"github.com/shotwell-paddle/routewerk/internal/service"
)

// ── Climber Profile ───────────────────────────────────────────

// ClimberProfile handles GET /profile — the logged-in user's tick list and stats.
func (h *Handler) ClimberProfile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := middleware.GetWebUser(ctx)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Parse tick list filters from query params
	tickFilter := repository.TickFilter{
		RouteType:  r.URL.Query().Get("type"),
		AscentType: r.URL.Query().Get("ascent"),
		Sort:       r.URL.Query().Get("sort"),
	}
	// Validate filter values
	if tickFilter.RouteType != "boulder" && tickFilter.RouteType != "route" {
		tickFilter.RouteType = ""
	}
	if tickFilter.AscentType != "send" && tickFilter.AscentType != "flash" && tickFilter.AscentType != "attempt" && tickFilter.AscentType != "project" {
		tickFilter.AscentType = ""
	}
	if tickFilter.Sort != "grade" {
		tickFilter.Sort = "date"
	}

	// Stats
	stats, err := h.ascentRepo.UserStats(ctx, user.ID)
	if err != nil {
		slog.Error("profile stats failed", "user_id", user.ID, "error", err)
		stats = &repository.UserClimbingStats{}
	}

	// Tick list — recent ascents with route info
	limit := 50
	ascents, total, err := h.ascentRepo.ListByUserFiltered(ctx, user.ID, tickFilter, limit, 0)
	if err != nil {
		slog.Error("profile tick list failed", "user_id", user.ID, "error", err)
	}

	var tickList []TickListItem
	for _, a := range ascents {
		name := ""
		if a.RouteName != nil {
			name = *a.RouteName
		}
		tickList = append(tickList, TickListItem{
			Ascent:     a.Ascent,
			RouteGrade: a.RouteGrade,
			RouteName:  name,
			RouteColor: a.RouteColor,
			RouteType:  a.RouteType,
			WallID:     a.WallID,
			TypeLabel:  ascentTypeLabel(a.AscentType),
		})
	}

	// Build grade pyramid data for the template
	pyramid := buildPyramidBars(stats.GradePyramid)

	// Quest & badge data for the progressions section
	locationID := middleware.GetWebLocationID(ctx)
	activeQuests, err := h.questSvc.ListUserQuests(ctx, user.ID, "active")
	if err != nil {
		slog.Error("profile active quests failed", "error", err)
	}
	completedQuests, err := h.questSvc.ListUserQuests(ctx, user.ID, "completed")
	if err != nil {
		slog.Error("profile completed quests failed", "error", err)
	}
	var badges []model.ClimberBadge
	var domainProgress []repository.DomainProgress
	if locationID != "" {
		badges, err = h.badgeRepo.ListUserBadgesForLocation(ctx, user.ID, locationID)
		if err != nil {
			slog.Error("profile badges failed", "error", err)
		}
		domainProgress, err = h.questSvc.UserDomainProgress(ctx, user.ID, locationID)
		if err != nil {
			slog.Error("profile domain progress failed", "error", err)
		}
	}

	data := &PageData{
		TemplateData:     templateDataFromContext(r, "profile"),
		User:             user,
		ClimberStats:     stats,
		TickList:         tickList,
		TickListTotal:    total,
		GradePyramid:     pyramid,
		TickFilterType:   tickFilter.RouteType,
		TickFilterAscent: tickFilter.AscentType,
		TickSort:         tickFilter.Sort,
		ActiveQuests:     activeQuests,
		CompletedQuests:  completedQuests,
		ClimberBadges:    badges,
		DomainProgress:   domainProgress,
	}
	h.render(w, r, "climber/profile.html", data)
}

// ProfileTicks renders just the tick-list-section partial (GET /profile/ticks).
// Used by HTMX filter chips to swap the tick list without replacing the full page.
func (h *Handler) ProfileTicks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := middleware.GetWebUser(ctx)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	tickFilter := repository.TickFilter{
		RouteType:  r.URL.Query().Get("type"),
		AscentType: r.URL.Query().Get("ascent"),
		Sort:       r.URL.Query().Get("sort"),
	}
	if tickFilter.RouteType != "boulder" && tickFilter.RouteType != "route" {
		tickFilter.RouteType = ""
	}
	if tickFilter.AscentType != "send" && tickFilter.AscentType != "flash" && tickFilter.AscentType != "attempt" && tickFilter.AscentType != "project" {
		tickFilter.AscentType = ""
	}
	if tickFilter.Sort != "grade" {
		tickFilter.Sort = "date"
	}

	limit := 50
	ascents, total, err := h.ascentRepo.ListByUserFiltered(ctx, user.ID, tickFilter, limit, 0)
	if err != nil {
		slog.Error("profile ticks partial failed", "user_id", user.ID, "error", err)
	}

	var tickList []TickListItem
	for _, a := range ascents {
		name := ""
		if a.RouteName != nil {
			name = *a.RouteName
		}
		tickList = append(tickList, TickListItem{
			Ascent:     a.Ascent,
			RouteGrade: a.RouteGrade,
			RouteName:  name,
			RouteColor: a.RouteColor,
			RouteType:  a.RouteType,
			WallID:     a.WallID,
			TypeLabel:  ascentTypeLabel(a.AscentType),
		})
	}

	data := &PageData{
		TemplateData:     templateDataFromContext(r, "profile"),
		TickList:         tickList,
		TickListTotal:    total,
		TickFilterType:   tickFilter.RouteType,
		TickFilterAscent: tickFilter.AscentType,
		TickSort:         tickFilter.Sort,
	}
	data.CSRFToken = middleware.TokenFromRequest(r)

	tmpl, ok := h.templates["climber/profile.html"]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "tick-list-section", data); err != nil {
		slog.Error("render tick-list-section failed", "error", err)
		http.Error(w, "Render error", http.StatusInternalServerError)
	}
}

// ── Profile Settings ─────────────────────────────────────────

// ProfileSettings renders the profile settings page (GET /profile/settings).
func (h *Handler) ProfileSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := middleware.GetWebUser(ctx)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	userSettings, _ := h.settingsRepo.GetUserSettings(ctx, user.ID)

	data := &PageData{
		TemplateData: templateDataFromContext(r, "profile"),
		User:         user,
		UserSettings: userSettings,
	}
	if r.URL.Query().Get("saved") == "1" {
		data.FormSuccess = "Settings saved."
	}
	h.render(w, r, "climber/profile-settings.html", data)
}

// ProfileSettingsSave handles POST /profile/settings.
func (h *Handler) ProfileSettingsSave(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := middleware.GetWebUser(ctx)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// 2 MB max for avatar upload
	if err := r.ParseMultipartForm(2 << 20); err != nil {
		if pErr := r.ParseForm(); pErr != nil {
			h.renderError(w, r, http.StatusBadRequest, "Invalid form", "Could not parse form data.")
			return
		}
	}

	// Update display name
	displayName := strings.TrimSpace(r.FormValue("display_name"))
	if displayName == "" {
		displayName = user.DisplayName
	}
	if len(displayName) > 50 {
		displayName = displayName[:50]
	}
	user.DisplayName = displayName

	// Update bio
	bio := strings.TrimSpace(r.FormValue("bio"))
	if bio != "" {
		if len(bio) > 300 {
			bio = bio[:300]
		}
		user.Bio = &bio
	} else {
		user.Bio = nil
	}

	// Handle avatar upload
	if r.FormValue("remove_avatar") == "1" {
		user.AvatarURL = nil
	} else if file, header, fErr := r.FormFile("avatar"); fErr == nil {
		defer file.Close()
		if header.Size <= 2<<20 && h.storageService.IsConfigured() {
			ct := header.Header.Get("Content-Type")
			if ct == "image/jpeg" || ct == "image/png" || ct == "image/webp" {
				url, uErr := h.storageService.Upload(ctx, "avatars/"+user.ID, header.Filename, ct, file)
				if uErr == nil {
					user.AvatarURL = &url
				} else {
					slog.Error("avatar upload failed", "user_id", user.ID, "error", uErr)
				}
			}
		}
	}

	if err := h.userRepo.Update(ctx, user); err != nil {
		slog.Error("profile update failed", "user_id", user.ID, "error", err)
		userSettings, _ := h.settingsRepo.GetUserSettings(ctx, user.ID)
		data := &PageData{
			TemplateData: templateDataFromContext(r, "profile"),
			User:         user,
			UserSettings: userSettings,
			FormError:    "Failed to save profile. Please try again.",
		}
		h.render(w, r, "climber/profile-settings.html", data)
		return
	}

	// Update privacy settings
	userSettings, _ := h.settingsRepo.GetUserSettings(ctx, user.ID)
	userSettings.Privacy = model.PrivacySettings{
		ShowProfile:       r.FormValue("show_profile") == "on",
		ShowTickList:      r.FormValue("show_tick_list") == "on",
		ShowStats:         r.FormValue("show_stats") == "on",
		ShowOnLeaderboard: r.FormValue("show_on_leaderboard") == "on",
	}
	if err := h.settingsRepo.UpdateUserSettings(ctx, user.ID, userSettings); err != nil {
		slog.Error("user settings update failed", "user_id", user.ID, "error", err)
	}

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/profile/settings?saved=1")
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, "/profile/settings?saved=1", http.StatusSeeOther)
}

// ── Password Change ─────────────────────────────────────────

// PasswordChange handles POST /profile/password.
func (h *Handler) PasswordChange(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := middleware.GetWebUser(ctx)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		fmt.Fprint(w, `<div class="form-alert form-alert-error mb-4">Invalid form data.</div>`)
		return
	}

	currentPassword := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")
	newPasswordConfirm := r.FormValue("new_password_confirm")

	if currentPassword == "" || newPassword == "" {
		fmt.Fprint(w, `<div class="form-alert form-alert-error mb-4">All fields are required.</div>`)
		return
	}

	if len(newPassword) < 8 {
		fmt.Fprint(w, `<div class="form-alert form-alert-error mb-4">New password must be at least 8 characters.</div>`)
		return
	}

	if len(newPassword) > 72 {
		fmt.Fprint(w, `<div class="form-alert form-alert-error mb-4">New password must be 72 characters or fewer.</div>`)
		return
	}

	if newPassword != newPasswordConfirm {
		fmt.Fprint(w, `<div class="form-alert form-alert-error mb-4">New passwords do not match.</div>`)
		return
	}

	if err := h.authService.ChangePassword(ctx, user.ID, currentPassword, newPassword); err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			fmt.Fprint(w, `<div class="form-alert form-alert-error mb-4">Current password is incorrect.</div>`)
			return
		}
		slog.Error("password change failed", "user_id", user.ID, "error", err)
		fmt.Fprint(w, `<div class="form-alert form-alert-error mb-4">Something went wrong. Please try again.</div>`)
		return
	}

	// Revoke all other sessions — the current session stays valid, but every
	// other browser/device is forced to re-authenticate with the new password.
	if session := middleware.GetWebSession(ctx); session != nil {
		if revoked, err := h.webSessionRepo.RevokeAllForUserExcept(ctx, user.ID, session.ID); err != nil {
			slog.Error("session revocation after password change failed", "user_id", user.ID, "error", err)
		} else if revoked > 0 {
			slog.Info("revoked sessions after password change", "user_id", user.ID, "revoked", revoked)
		}
	}

	fmt.Fprint(w, `<div class="form-alert form-alert-success mb-4">Password updated. Other sessions have been signed out.</div>`)
}
