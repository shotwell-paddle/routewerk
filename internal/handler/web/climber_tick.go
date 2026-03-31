package webhandler

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

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
