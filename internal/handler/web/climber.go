package webhandler

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
	"github.com/shotwell-paddle/routewerk/internal/service"
)

// ── Ascent Logging ────────────────────────────────────────────

// LogAscent handles POST /routes/{routeID}/ascent from the route detail page.
// Returns a single feed-item partial to prepend into #route-ascents via HTMX.
func (h *Handler) LogAscent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	routeID := chi.URLParam(r, "routeID")
	user := middleware.GetWebUser(ctx)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if !validRouteID.MatchString(routeID) {
		http.Error(w, "Invalid route", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	ascentType := r.FormValue("ascent_type")
	validTypes := map[string]bool{
		"send": true, "flash": true, "attempt": true, "project": true,
	}
	if !validTypes[ascentType] {
		http.Error(w, "Invalid ascent type", http.StatusBadRequest)
		return
	}

	attempts, _ := strconv.Atoi(r.FormValue("attempts"))
	if attempts < 1 {
		attempts = 1
	}

	var notes *string
	if n := strings.TrimSpace(r.FormValue("notes")); n != "" {
		if len(n) > 500 {
			http.Error(w, "Notes too long (max 500 characters)", http.StatusBadRequest)
			return
		}
		notes = &n
	}

	// Verify route exists and belongs to user's gym
	rt, err := h.routeRepo.GetByID(ctx, routeID)
	if err != nil || rt == nil {
		slog.Error("log ascent: route not found", "route_id", routeID, "error", err)
		http.Error(w, "Route not found", http.StatusNotFound)
		return
	}
	if !h.checkLocationOwnership(w, r, rt.LocationID) {
		return
	}

	// A flash means first try — block if any prior ascent exists.
	// A send is also blocked if user already has a send/flash (one completion per route).
	// For flash: HasPriorAscents covers both checks (no prior ascents = no completed route).
	// For send: only need HasCompletedRoute.
	if ascentType == "flash" {
		hasPrior, pErr := h.ascentRepo.HasPriorAscents(ctx, user.ID, routeID)
		if pErr != nil {
			slog.Error("log ascent: prior check failed", "error", pErr)
			http.Error(w, "Could not verify ascent history", http.StatusInternalServerError)
			return
		}
		if hasPrior {
			http.Error(w, "Cannot log a flash — you already have logged attempts on this route", http.StatusUnprocessableEntity)
			return
		}
	} else if ascentType == "send" {
		completed, cErr := h.ascentRepo.HasCompletedRoute(ctx, user.ID, routeID)
		if cErr != nil {
			slog.Error("log ascent: completion check failed", "error", cErr)
			http.Error(w, "Could not verify ascent history", http.StatusInternalServerError)
			return
		}
		if completed {
			http.Error(w, "You've already sent this route", http.StatusUnprocessableEntity)
			return
		}
	}

	ascent := &model.Ascent{
		UserID:     user.ID,
		RouteID:    routeID,
		AscentType: ascentType,
		Attempts:   attempts,
		Notes:      notes,
		ClimbedAt:  time.Now(),
	}

	if err := h.ascentRepo.Create(ctx, ascent); err != nil {
		// Handle unique constraint violation from idx_ascents_one_completion_per_user_route
		if strings.Contains(err.Error(), "idx_ascents_one_completion_per_user_route") {
			http.Error(w, "You've already sent this route", http.StatusUnprocessableEntity)
			return
		}
		slog.Error("log ascent failed", "user_id", user.ID, "route_id", routeID, "error", err)
		http.Error(w, "Could not log ascent", http.StatusInternalServerError)
		return
	}

	// Render a single feed-item partial
	initial := "?"
	if len(user.DisplayName) > 0 {
		initial = strings.ToUpper(user.DisplayName[:1])
	}

	av := AscentView{
		Ascent:      *ascent,
		UserName:    user.DisplayName,
		UserInitial: initial,
		AscentType:  ascentTypeLabel(ascentType),
		Notes:       notes,
	}

	data := &PageData{
		TemplateData:  templateDataFromContext(r, "routes"),
		RecentAscents: []AscentView{av},
	}

	tmpl, ok := h.templates["climber/route-detail.html"]
	if !ok {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "ascent-feed-item", data); err != nil {
		slog.Error("ascent feed-item render failed", "error", err)
		http.Error(w, "Render error", http.StatusInternalServerError)
	}
}

// AscentsFeed handles GET /routes/{routeID}/ascents-feed — lazy-loads
// recent ascents for the route detail page.
func (h *Handler) AscentsFeed(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	routeID := chi.URLParam(r, "routeID")

	if !validRouteID.MatchString(routeID) {
		http.Error(w, "Invalid route", http.StatusBadRequest)
		return
	}

	viewerID := ""
	if user := middleware.GetWebUser(ctx); user != nil {
		viewerID = user.ID
	}

	ascents, err := h.ascentRepo.ListByRouteForViewer(ctx, routeID, viewerID, 20, 0)
	if err != nil {
		slog.Error("ascent feed failed", "route_id", routeID, "error", err)
		http.Error(w, "Could not load ascents", http.StatusInternalServerError)
		return
	}

	var views []AscentView
	for _, a := range ascents {
		initial := "?"
		if len(a.UserDisplayName) > 0 {
			initial = strings.ToUpper(a.UserDisplayName[:1])
		}
		views = append(views, AscentView{
			Ascent:      a.Ascent,
			UserName:    a.UserDisplayName,
			UserInitial: initial,
			AscentType:  ascentTypeLabel(a.AscentType),
			Notes:       a.Notes,
		})
	}

	data := &PageData{
		TemplateData:  templateDataFromContext(r, "routes"),
		RecentAscents: views,
	}

	tmpl, ok := h.templates["climber/route-detail.html"]
	if !ok {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "ascent-feed-list", data); err != nil {
		slog.Error("ascent feed render failed", "error", err)
		http.Error(w, "Render error", http.StatusInternalServerError)
	}
}

// ── Rating ────────────────────────────────────────────────────

// RateRoute handles POST /routes/{routeID}/rate from the route detail page.
// Returns a confirmation card partial to replace the rating form via HTMX outerHTML.
func (h *Handler) RateRoute(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	routeID := chi.URLParam(r, "routeID")
	user := middleware.GetWebUser(ctx)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if !validRouteID.MatchString(routeID) {
		http.Error(w, "Invalid route", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	rating, _ := strconv.Atoi(r.FormValue("rating"))
	if rating < 1 || rating > 5 {
		http.Error(w, "Rating must be 1-5", http.StatusBadRequest)
		return
	}

	var comment *string
	if c := strings.TrimSpace(r.FormValue("comment")); c != "" {
		if len(c) > 1000 {
			http.Error(w, "Comment too long (max 1000 characters)", http.StatusBadRequest)
			return
		}
		comment = &c
	}

	// Verify route exists and belongs to user's gym
	rt, rtErr := h.routeRepo.GetByID(ctx, routeID)
	if rtErr != nil || rt == nil {
		http.Error(w, "Route not found", http.StatusNotFound)
		return
	}
	if !h.checkLocationOwnership(w, r, rt.LocationID) {
		return
	}

	rr := &model.RouteRating{
		UserID:  user.ID,
		RouteID: routeID,
		Rating:  rating,
		Comment: comment,
	}

	if err := h.ratingRepo.Upsert(ctx, rr); err != nil {
		slog.Error("rate route failed", "user_id", user.ID, "route_id", routeID, "error", err)
		http.Error(w, "Could not save rating", http.StatusInternalServerError)
		return
	}

	data := &PageData{
		TemplateData: templateDataFromContext(r, "routes"),
		UserRating:   rating,
	}

	tmpl, ok := h.templates["climber/route-detail.html"]
	if !ok {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "rating-confirmation", data); err != nil {
		slog.Error("rating confirmation render failed", "error", err)
		http.Error(w, "Render error", http.StatusInternalServerError)
	}
}

// ── Difficulty Consensus ──────────────────────────────────────

// DifficultyVote handles POST /routes/{routeID}/difficulty?vote=easy|right|hard.
// Returns the updated consensus-section partial via HTMX outerHTML.
func (h *Handler) DifficultyVote(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	routeID := chi.URLParam(r, "routeID")
	user := middleware.GetWebUser(ctx)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if !validRouteID.MatchString(routeID) {
		http.Error(w, "Invalid route", http.StatusBadRequest)
		return
	}

	vote := r.URL.Query().Get("vote")
	validVotes := map[string]bool{"easy": true, "right": true, "hard": true}
	if !validVotes[vote] {
		http.Error(w, "Invalid vote", http.StatusBadRequest)
		return
	}

	// Verify route exists and belongs to user's gym
	rt, rtErr := h.routeRepo.GetByID(ctx, routeID)
	if rtErr != nil || rt == nil {
		http.Error(w, "Route not found", http.StatusNotFound)
		return
	}
	if !h.checkLocationOwnership(w, r, rt.LocationID) {
		return
	}

	dv := &model.DifficultyVote{
		UserID:  user.ID,
		RouteID: routeID,
		Vote:    vote,
	}

	if err := h.difficultyRepo.Upsert(ctx, dv); err != nil {
		slog.Error("difficulty vote failed", "user_id", user.ID, "route_id", routeID, "error", err)
		http.Error(w, "Could not save vote", http.StatusInternalServerError)
		return
	}

	// Fetch updated counts
	consensus := loadConsensus(ctx, h.difficultyRepo, routeID)

	// Re-fetch the route to get its ID for template links
	data := &PageData{
		TemplateData: templateDataFromContext(r, "routes"),
		Route:        &RouteView{Route: model.Route{ID: routeID}},
		Consensus:    consensus,
	}

	tmpl, ok := h.templates["climber/route-detail.html"]
	if !ok {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "consensus-section", data); err != nil {
		slog.Error("consensus section render failed", "error", err)
		http.Error(w, "Render error", http.StatusInternalServerError)
	}
}

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

	data := &PageData{
		TemplateData:    templateDataFromContext(r, "profile"),
		User:            user,
		ClimberStats:    stats,
		TickList:        tickList,
		TickListTotal:   total,
		GradePyramid:    pyramid,
		TickFilterType:  tickFilter.RouteType,
		TickFilterAscent: tickFilter.AscentType,
		TickSort:        tickFilter.Sort,
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

	fmt.Fprint(w, `<div class="form-alert form-alert-success mb-4">Password updated.</div>`)
}

// ── Tick Editing ─────────────────────────────────────────────

// TickEditForm returns the inline edit form for a single tick item (GET /profile/ticks/{ascentID}/edit).
func (h *Handler) TickEditForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := middleware.GetWebUser(ctx)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	ascentID := chi.URLParam(r, "ascentID")
	if !validRouteID.MatchString(ascentID) {
		http.Error(w, "Invalid ascent ID", http.StatusBadRequest)
		return
	}
	ascent, err := h.ascentRepo.GetByID(ctx, ascentID)
	if err != nil || ascent == nil {
		http.Error(w, "ascent not found", http.StatusNotFound)
		return
	}
	if ascent.UserID != user.ID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// Parallelize independent queries for rating, difficulty, and flash check.
	var wg sync.WaitGroup

	userRating := 0
	wg.Add(1)
	go func() {
		defer wg.Done()
		if existing, rErr := h.ratingRepo.GetByUserAndRoute(ctx, user.ID, ascent.RouteID); rErr == nil && existing != nil {
			userRating = existing.Rating
		}
	}()

	userDifficulty := ""
	wg.Add(1)
	go func() {
		defer wg.Done()
		if existing, dErr := h.difficultyRepo.GetByUserAndRoute(ctx, user.ID, ascent.RouteID); dErr == nil && existing != nil {
			userDifficulty = existing.Vote
		}
	}()

	canFlash := true
	wg.Add(1)
	go func() {
		defer wg.Done()
		if hasPrior, pErr := h.ascentRepo.HasPriorAscents(ctx, user.ID, ascent.RouteID); pErr == nil && hasPrior {
			canFlash = false
		}
	}()

	wg.Wait()

	tmpl, ok := h.templates["climber/profile.html"]
	if !ok {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := struct {
		model.Ascent
		CSRFToken      string
		UserRating     int
		UserDifficulty string
		CanFlash       bool
	}{*ascent, middleware.TokenFromRequest(r), userRating, userDifficulty, canFlash}
	tmpl.ExecuteTemplate(w, "tick-edit-form", data) //nolint:errcheck
}

// TickUpdate handles POST /profile/ticks/{ascentID} — updates an ascent.
func (h *Handler) TickUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := middleware.GetWebUser(ctx)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	ascentID := chi.URLParam(r, "ascentID")
	if !validRouteID.MatchString(ascentID) {
		http.Error(w, "Invalid ascent ID", http.StatusBadRequest)
		return
	}
	ascent, err := h.ascentRepo.GetByID(ctx, ascentID)
	if err != nil || ascent == nil {
		http.Error(w, "ascent not found", http.StatusNotFound)
		return
	}
	if ascent.UserID != user.ID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if pErr := r.ParseForm(); pErr != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	ascentType := r.FormValue("ascent_type")
	if ascentType != "send" && ascentType != "flash" && ascentType != "attempt" && ascentType != "project" {
		ascentType = ascent.AscentType
	}

	// Block changing to flash if user has other ascents on this route
	if ascentType == "flash" && ascent.AscentType != "flash" {
		hasPrior, pErr := h.ascentRepo.HasPriorAscents(ctx, user.ID, ascent.RouteID)
		if pErr != nil {
			slog.Error("tick update: prior check failed", "error", pErr)
			http.Error(w, "Could not verify ascent history", http.StatusInternalServerError)
			return
		}
		if hasPrior {
			http.Error(w, "Cannot change to flash — you have other logged attempts on this route", http.StatusUnprocessableEntity)
			return
		}
	}

	// Block changing to send/flash if user already has a completed send on this route
	if (ascentType == "send" || ascentType == "flash") && ascent.AscentType != "send" && ascent.AscentType != "flash" {
		completed, cErr := h.ascentRepo.HasCompletedRoute(ctx, user.ID, ascent.RouteID)
		if cErr != nil {
			slog.Error("tick update: completion check failed", "error", cErr)
			http.Error(w, "Could not verify ascent history", http.StatusInternalServerError)
			return
		}
		if completed {
			http.Error(w, "You've already sent this route", http.StatusUnprocessableEntity)
			return
		}
	}

	attempts, _ := strconv.Atoi(r.FormValue("attempts"))
	if attempts < 1 {
		attempts = ascent.Attempts
	}

	notes := strings.TrimSpace(r.FormValue("notes"))
	if len(notes) > 500 {
		http.Error(w, "Notes too long (max 500 characters)", http.StatusBadRequest)
		return
	}
	var notesPtr *string
	if notes != "" {
		notesPtr = &notes
	}

	ascent.AscentType = ascentType
	ascent.Attempts = attempts
	ascent.Notes = notesPtr

	if err := h.ascentRepo.Update(ctx, ascent); err != nil {
		slog.Error("tick update failed", "ascent_id", ascentID, "error", err)
		http.Error(w, "update failed", http.StatusInternalServerError)
		return
	}

	// Save rating if provided (1-5)
	if ratingStr := r.FormValue("rating"); ratingStr != "" {
		if rVal, rErr := strconv.Atoi(ratingStr); rErr == nil && rVal >= 1 && rVal <= 5 {
			rating := &model.RouteRating{
				UserID:  user.ID,
				RouteID: ascent.RouteID,
				Rating:  rVal,
			}
			if uErr := h.ratingRepo.Upsert(ctx, rating); uErr != nil {
				slog.Error("tick update: rating save failed", "error", uErr)
			}
		}
	}

	// Save difficulty opinion if provided
	if diff := r.FormValue("difficulty"); diff == "easy" || diff == "right" || diff == "hard" {
		vote := &model.DifficultyVote{
			UserID:  user.ID,
			RouteID: ascent.RouteID,
			Vote:    diff,
		}
		if uErr := h.difficultyRepo.Upsert(ctx, vote); uErr != nil {
			slog.Error("tick update: difficulty save failed", "error", uErr)
		}
	}

	// Rebuild the tick item from current data + route info for display
	item := TickListItem{
		Ascent:    *ascent,
		TypeLabel: ascentTypeLabel(ascent.AscentType),
	}
	if rt, rErr := h.routeRepo.GetByID(ctx, ascent.RouteID); rErr == nil && rt != nil {
		item.RouteGrade = rt.Grade
		item.RouteColor = rt.Color
		item.RouteType = rt.RouteType
		item.WallID = rt.WallID
		if rt.Name != nil {
			item.RouteName = *rt.Name
		}
	}

	tmpl, ok := h.templates["climber/profile.html"]
	if !ok {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := struct {
		TickListItem
		CSRFToken string
	}{item, middleware.TokenFromRequest(r)}
	tmpl.ExecuteTemplate(w, "tick-item", data) //nolint:errcheck
}

// TickDelete handles POST /profile/ticks/{ascentID}/delete — removes an ascent.
func (h *Handler) TickDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := middleware.GetWebUser(ctx)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	ascentID := chi.URLParam(r, "ascentID")
	if !validRouteID.MatchString(ascentID) {
		http.Error(w, "Invalid ascent ID", http.StatusBadRequest)
		return
	}
	if err := h.ascentRepo.Delete(ctx, ascentID, user.ID); err != nil {
		slog.Error("tick delete failed", "ascent_id", ascentID, "error", err)
		http.Error(w, "delete failed", http.StatusInternalServerError)
		return
	}

	// Return empty response — HTMX will remove the element
	w.WriteHeader(http.StatusOK)
}
