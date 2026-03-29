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

type SessionHandler struct {
	sessions *repository.SessionRepo
}

func NewSessionHandler(sessions *repository.SessionRepo) *SessionHandler {
	return &SessionHandler{sessions: sessions}
}

type createSessionRequest struct {
	ScheduledDate string  `json:"scheduled_date"`
	Notes         *string `json:"notes,omitempty"`
}

type assignRequest struct {
	SetterID     string   `json:"setter_id"`
	WallID       *string  `json:"wall_id,omitempty"`
	TargetGrades []string `json:"target_grades,omitempty"`
	Notes        *string  `json:"notes,omitempty"`
}

func (h *SessionHandler) Create(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	userID := middleware.GetUserID(r.Context())

	var req createSessionRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ScheduledDate == "" {
		Error(w, http.StatusBadRequest, "scheduled_date is required")
		return
	}

	date, err := time.Parse("2006-01-02", req.ScheduledDate)
	if err != nil {
		Error(w, http.StatusBadRequest, "scheduled_date must be YYYY-MM-DD format")
		return
	}

	session := &model.SettingSession{
		LocationID:    locationID,
		ScheduledDate: date,
		Notes:         req.Notes,
		CreatedBy:     userID,
	}

	if err := h.sessions.Create(r.Context(), session); err != nil {
		Error(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	JSON(w, http.StatusCreated, session)
}

func (h *SessionHandler) List(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	sessions, err := h.sessions.ListByLocation(r.Context(), locationID, limit, offset)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	if sessions == nil {
		sessions = []model.SettingSession{}
	}

	JSON(w, http.StatusOK, sessions)
}

func (h *SessionHandler) Get(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")

	session, err := h.sessions.GetByID(r.Context(), sessionID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if session == nil {
		Error(w, http.StatusNotFound, "session not found")
		return
	}

	JSON(w, http.StatusOK, session)
}

func (h *SessionHandler) Update(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")

	session, err := h.sessions.GetByID(r.Context(), sessionID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if session == nil {
		Error(w, http.StatusNotFound, "session not found")
		return
	}

	var req createSessionRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ScheduledDate != "" {
		date, err := time.Parse("2006-01-02", req.ScheduledDate)
		if err == nil {
			session.ScheduledDate = date
		}
	}
	if req.Notes != nil {
		session.Notes = req.Notes
	}

	if err := h.sessions.Update(r.Context(), session); err != nil {
		Error(w, http.StatusInternalServerError, "failed to update session")
		return
	}

	session, _ = h.sessions.GetByID(r.Context(), sessionID)
	JSON(w, http.StatusOK, session)
}

func (h *SessionHandler) Assign(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")

	var req assignRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.SetterID == "" {
		Error(w, http.StatusBadRequest, "setter_id is required")
		return
	}

	assignment := &model.SettingSessionAssignment{
		SessionID:    sessionID,
		SetterID:     req.SetterID,
		WallID:       req.WallID,
		TargetGrades: req.TargetGrades,
		Notes:        req.Notes,
	}

	if err := h.sessions.AddAssignment(r.Context(), assignment); err != nil {
		Error(w, http.StatusInternalServerError, "failed to add assignment")
		return
	}

	JSON(w, http.StatusCreated, assignment)
}
