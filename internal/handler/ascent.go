package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

type AscentHandler struct {
	ascents *repository.AscentRepo
}

func NewAscentHandler(ascents *repository.AscentRepo) *AscentHandler {
	return &AscentHandler{ascents: ascents}
}

type logAscentRequest struct {
	AscentType string  `json:"ascent_type"`
	Attempts   int     `json:"attempts"`
	Notes      *string `json:"notes,omitempty"`
	ClimbedAt  *string `json:"climbed_at,omitempty"`
}

func (h *AscentHandler) Log(w http.ResponseWriter, r *http.Request) {
	routeID := chi.URLParam(r, "routeID")
	userID := middleware.GetUserID(r.Context())

	var req logAscentRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.AscentType != "send" && req.AscentType != "flash" && req.AscentType != "attempt" && req.AscentType != "project" {
		Error(w, http.StatusBadRequest, "ascent_type must be 'send', 'flash', 'attempt', or 'project'")
		return
	}

	if req.Attempts <= 0 {
		req.Attempts = 1
	}

	climbedAt := time.Now()
	if req.ClimbedAt != nil {
		if parsed, err := time.Parse(time.RFC3339, *req.ClimbedAt); err == nil {
			climbedAt = parsed
		}
	}

	ascent := &model.Ascent{
		UserID:     userID,
		RouteID:    routeID,
		AscentType: req.AscentType,
		Attempts:   req.Attempts,
		Notes:      req.Notes,
		ClimbedAt:  climbedAt,
	}

	if err := h.ascents.Create(r.Context(), ascent); err != nil {
		Error(w, http.StatusInternalServerError, "failed to log ascent")
		return
	}

	JSON(w, http.StatusCreated, ascent)
}

func (h *AscentHandler) MyAscents(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	ascents, total, err := h.ascents.ListByUser(r.Context(), userID, limit, offset)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"ascents": ascents,
		"total":   total,
	})
}

func (h *AscentHandler) MyStats(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	stats, err := h.ascents.UserStats(r.Context(), userID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	JSON(w, http.StatusOK, stats)
}

func (h *AscentHandler) RouteAscents(w http.ResponseWriter, r *http.Request) {
	routeID := chi.URLParam(r, "routeID")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	ascents, err := h.ascents.ListByRoute(r.Context(), routeID, limit, offset)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	JSON(w, http.StatusOK, ascents)
}
