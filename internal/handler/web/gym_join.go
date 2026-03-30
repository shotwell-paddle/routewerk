package webhandler

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

// ── Gym Search & Join ─────────────────────────────────────────

// JoinGymPage handles GET /join-gym — shows the gym search page.
// On first load (no query), shows all gyms. With ?q= param, filters.
func (h *Handler) JoinGymPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := strings.TrimSpace(r.URL.Query().Get("q"))

	var results []repository.LocationSearchResult
	var err error

	if query != "" {
		results, err = h.locationRepo.SearchPublic(ctx, query, 20)
	} else {
		results, err = h.locationRepo.ListAllPublic(ctx, 50)
	}
	if err != nil {
		slog.Error("gym search failed", "query", query, "error", err)
	}

	data := &PageData{
		TemplateData: templateDataFromContext(r, "join-gym"),
		GymResults:   results,
		GymQuery:     query,
	}
	h.render(w, r, "climber/join-gym.html", data)
}

// JoinGymSearch handles GET /join-gym/search — HTMX partial for live search.
func (h *Handler) JoinGymSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := strings.TrimSpace(r.URL.Query().Get("q"))

	var results []repository.LocationSearchResult
	var err error

	if query != "" {
		results, err = h.locationRepo.SearchPublic(ctx, query, 20)
	} else {
		results, err = h.locationRepo.ListAllPublic(ctx, 50)
	}
	if err != nil {
		slog.Error("gym search failed", "query", query, "error", err)
	}

	data := &PageData{
		TemplateData: templateDataFromContext(r, "join-gym"),
		GymResults:   results,
		GymQuery:     query,
	}

	tmpl, ok := h.templates["climber/join-gym.html"]
	if !ok {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "gym-result-list", data); err != nil {
		slog.Error("gym search render failed", "error", err)
		http.Error(w, "Render error", http.StatusInternalServerError)
	}
}

// JoinGymSubmit handles POST /join-gym — creates a climber membership.
func (h *Handler) JoinGymSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := middleware.GetWebUser(ctx)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	locationID := r.FormValue("location_id")
	orgID := r.FormValue("org_id")

	if locationID == "" || orgID == "" {
		http.Error(w, "Missing gym selection", http.StatusBadRequest)
		return
	}

	// Verify the location exists
	loc, err := h.locationRepo.GetByID(ctx, locationID)
	if err != nil || loc == nil {
		slog.Error("join gym: location not found", "location_id", locationID, "error", err)
		http.Error(w, "Gym not found", http.StatusNotFound)
		return
	}

	// Create climber membership
	membership := &model.UserMembership{
		UserID:     user.ID,
		OrgID:      orgID,
		LocationID: &locationID,
		Role:       "climber",
	}

	if err := h.orgRepo.AddMember(ctx, membership); err != nil {
		// Likely a duplicate membership
		slog.Error("join gym: create membership failed", "user_id", user.ID, "location_id", locationID, "error", err)

		// Re-render with error
		results, _ := h.locationRepo.ListAllPublic(ctx, 50)
		data := &PageData{
			TemplateData: templateDataFromContext(r, "join-gym"),
			GymResults:   results,
			JoinError:    "Could not join this gym. You may already be a member.",
		}
		h.render(w, r, "climber/join-gym.html", data)
		return
	}

	// Update the web session with the new location
	session := middleware.GetWebSession(ctx)
	if session != nil && session.LocationID == nil {
		if err := h.webSessionRepo.UpdateLocation(ctx, session.ID, locationID); err != nil {
			slog.Error("update session location failed", "session_id", session.ID, "error", err)
		}
	}

	// Redirect to dashboard — now they have a location
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}
