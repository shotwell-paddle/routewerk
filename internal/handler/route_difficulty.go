package handler

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

// RouteDifficultyHandler exposes the JSON variant of the HTMX difficulty
// vote endpoint (internal/handler/web/climber_feedback.go::DifficultyVote).
// Climbers vote easy/right/hard on a route they've climbed; the server
// aggregates and returns counts + percentages.
type RouteDifficultyHandler struct {
	routes *repository.RouteRepo
	votes  *repository.DifficultyRepo
}

func NewRouteDifficultyHandler(routes *repository.RouteRepo, votes *repository.DifficultyRepo) *RouteDifficultyHandler {
	return &RouteDifficultyHandler{routes: routes, votes: votes}
}

// difficultyResponse mirrors the HTMX template's ConsensusData with the
// current viewer's vote attached so the SPA can highlight the right pill.
type difficultyResponse struct {
	EasyCount  int    `json:"easy_count"`
	RightCount int    `json:"right_count"`
	HardCount  int    `json:"hard_count"`
	TotalVotes int    `json:"total_votes"`
	EasyPct    int    `json:"easy_pct"`
	RightPct   int    `json:"right_pct"`
	HardPct    int    `json:"hard_pct"`
	MyVote     string `json:"my_vote"` // "" / "easy" / "right" / "hard"
}

// Get — GET /api/v1/locations/{locationID}/routes/{routeID}/difficulty.
//
// Returns aggregated vote counts + percentages, and the caller's prior
// vote if one exists. Any authenticated location member can read.
func (h *RouteDifficultyHandler) Get(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	routeID := chi.URLParam(r, "routeID")
	if !isUUID(locationID) || !isUUID(routeID) {
		Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	route, err := h.routes.GetByID(r.Context(), routeID)
	if err != nil || route == nil || route.LocationID != locationID {
		Error(w, http.StatusNotFound, "route not found")
		return
	}

	tally, err := h.votes.RouteCounts(r.Context(), routeID)
	if err != nil {
		slog.Error("difficulty counts failed", "route_id", routeID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := difficultyResponse{
		EasyCount:  tally.Easy,
		RightCount: tally.Right,
		HardCount:  tally.Hard,
		TotalVotes: tally.Total,
	}
	if tally.Total > 0 {
		resp.EasyPct = tally.Easy * 100 / tally.Total
		resp.RightPct = tally.Right * 100 / tally.Total
		resp.HardPct = tally.Hard * 100 / tally.Total
	}

	// Surface the caller's prior vote so the SPA can pre-light the
	// matching pill. Best-effort: missing vote → empty string.
	if userID := middleware.GetUserID(r.Context()); userID != "" {
		if prior, _ := h.votes.GetByUserAndRoute(r.Context(), userID, routeID); prior != nil {
			resp.MyVote = prior.Vote
		}
	}
	JSON(w, http.StatusOK, resp)
}

type difficultyVoteRequest struct {
	Vote string `json:"vote"`
}

var validDifficultyVotes = map[string]bool{
	"easy": true, "right": true, "hard": true,
}

// Vote — POST /api/v1/locations/{locationID}/routes/{routeID}/difficulty
// { vote: "easy" | "right" | "hard" }.
//
// Upserts the caller's vote. Returns the same payload as Get so the
// SPA can swap state in one round-trip.
func (h *RouteDifficultyHandler) Vote(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	routeID := chi.URLParam(r, "routeID")
	if !isUUID(locationID) || !isUUID(routeID) {
		Error(w, http.StatusBadRequest, "invalid id")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		Error(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req difficultyVoteRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !validDifficultyVotes[req.Vote] {
		Error(w, http.StatusBadRequest, "vote must be easy, right, or hard")
		return
	}

	route, err := h.routes.GetByID(r.Context(), routeID)
	if err != nil || route == nil || route.LocationID != locationID {
		Error(w, http.StatusNotFound, "route not found")
		return
	}

	if err := h.votes.Upsert(r.Context(), &model.DifficultyVote{
		UserID:  userID,
		RouteID: routeID,
		Vote:    req.Vote,
	}); err != nil {
		slog.Error("difficulty vote save failed", "route_id", routeID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	tally, _ := h.votes.RouteCounts(r.Context(), routeID)
	resp := difficultyResponse{
		EasyCount:  tally.Easy,
		RightCount: tally.Right,
		HardCount:  tally.Hard,
		TotalVotes: tally.Total,
		MyVote:     req.Vote,
	}
	if tally.Total > 0 {
		resp.EasyPct = tally.Easy * 100 / tally.Total
		resp.RightPct = tally.Right * 100 / tally.Total
		resp.HardPct = tally.Hard * 100 / tally.Total
	}
	JSON(w, http.StatusOK, resp)
}
