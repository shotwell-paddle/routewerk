package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
	"github.com/shotwell-paddle/routewerk/internal/service"
)

// QuestHandler exposes the JSON quest API for the SPA at /app/quests
// (Phase 2.8). The HTMX side at /quests still drives the same QuestService
// underneath; this handler is the parallel JSON entry point.
type QuestHandler struct {
	quests *service.QuestService
}

func NewQuestHandler(quests *service.QuestService) *QuestHandler {
	return &QuestHandler{quests: quests}
}

// ListAvailable — GET /locations/{locationID}/quests. Returns the active
// quest catalog at this location, with social-proof active/completed counts.
func (h *QuestHandler) ListAvailable(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	items, err := h.quests.ListAvailable(r.Context(), locationID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if items == nil {
		items = []repository.QuestListItem{}
	}
	JSON(w, http.StatusOK, map[string]interface{}{"quests": items})
}

// MyQuests — GET /me/quests?status=active|completed. Defaults to all.
func (h *QuestHandler) MyQuests(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	status := r.URL.Query().Get("status") // empty = all

	enrollments, err := h.quests.ListUserQuests(r.Context(), userID, status)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if enrollments == nil {
		enrollments = []model.ClimberQuest{}
	}
	JSON(w, http.StatusOK, map[string]interface{}{"quests": enrollments})
}

// Get — GET /quests/{questID}. Quest detail (no enrollment context).
func (h *QuestHandler) Get(w http.ResponseWriter, r *http.Request) {
	questID := chi.URLParam(r, "questID")
	q, err := h.quests.GetQuest(r.Context(), questID)
	if err != nil {
		if errors.Is(err, service.ErrQuestNotFound) {
			Error(w, http.StatusNotFound, "quest not found")
			return
		}
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	JSON(w, http.StatusOK, q)
}

type startQuestRequest struct {
	LocationID string `json:"location_id"`
}

// Start — POST /quests/{questID}/start. Enrolls the caller. The location id
// is required because the quest service uses it to filter available quests
// (some quests may only be visible at certain locations).
func (h *QuestHandler) Start(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	questID := chi.URLParam(r, "questID")

	var req startQuestRequest
	_ = Decode(r, &req)
	if req.LocationID == "" {
		Error(w, http.StatusBadRequest, "location_id is required")
		return
	}

	enrollment, err := h.quests.StartQuest(r.Context(), userID, questID, req.LocationID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrQuestNotFound):
			Error(w, http.StatusNotFound, "quest not found")
		case errors.Is(err, service.ErrQuestNotActive),
			errors.Is(err, service.ErrQuestNotAvailable):
			Error(w, http.StatusBadRequest, "quest not available")
		case errors.Is(err, service.ErrAlreadyEnrolled):
			// Treat as 200 — the SPA can re-fetch /me/quests.
			JSON(w, http.StatusOK, map[string]interface{}{"already_enrolled": true})
			return
		default:
			Error(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	JSON(w, http.StatusCreated, map[string]interface{}{"enrollment": enrollment})
}

type logQuestRequest struct {
	LogType string  `json:"log_type"`
	Notes   *string `json:"notes,omitempty"`
	RouteID *string `json:"route_id,omitempty"`
	Rating  *int    `json:"rating,omitempty"`
}

// LogProgress — POST /climber-quests/{climberQuestID}/log. Records a progress
// entry against an enrollment the caller owns. Returns the new log row.
func (h *QuestHandler) LogProgress(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	climberQuestID := chi.URLParam(r, "climberQuestID")

	var req logQuestRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	logType := strings.TrimSpace(req.LogType)
	if logType == "" {
		logType = "general"
	}

	log := &model.QuestLog{
		LogType: logType,
		Notes:   req.Notes,
		RouteID: req.RouteID,
		Rating:  req.Rating,
	}
	if err := h.quests.LogProgress(r.Context(), userID, climberQuestID, log); err != nil {
		switch {
		case errors.Is(err, service.ErrClimberQuestNotFound):
			Error(w, http.StatusNotFound, "enrollment not found")
		case errors.Is(err, service.ErrNotOwner):
			Error(w, http.StatusForbidden, "you do not own this enrollment")
		default:
			Error(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	JSON(w, http.StatusCreated, map[string]interface{}{"log": log})
}

// Abandon — DELETE /climber-quests/{climberQuestID}. Drops the caller out of
// a quest. Returns 204 on success.
func (h *QuestHandler) Abandon(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	climberQuestID := chi.URLParam(r, "climberQuestID")

	if err := h.quests.AbandonQuest(r.Context(), userID, climberQuestID); err != nil {
		switch {
		case errors.Is(err, service.ErrClimberQuestNotFound):
			Error(w, http.StatusNotFound, "enrollment not found")
		case errors.Is(err, service.ErrNotOwner):
			Error(w, http.StatusForbidden, "you do not own this enrollment")
		default:
			Error(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
