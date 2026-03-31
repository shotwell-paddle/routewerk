package webhandler

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
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
