package webhandler

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

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
