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

type LaborHandler struct {
	labor *repository.LaborRepo
}

func NewLaborHandler(labor *repository.LaborRepo) *LaborHandler {
	return &LaborHandler{labor: labor}
}

type logLaborRequest struct {
	SessionID   *string  `json:"session_id,omitempty"`
	Date        string   `json:"date"`
	HoursWorked *float64 `json:"hours_worked,omitempty"`
	RoutesSet   int      `json:"routes_set"`
	Notes       *string  `json:"notes,omitempty"`
}

func (h *LaborHandler) Log(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	userID := middleware.GetUserID(r.Context())

	var req logLaborRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Date == "" {
		Error(w, http.StatusBadRequest, "date is required")
		return
	}

	date, err := time.Parse("2006-01-02", req.Date)
	if err != nil {
		Error(w, http.StatusBadRequest, "date must be YYYY-MM-DD format")
		return
	}

	log := &model.SetterLaborLog{
		UserID:      userID,
		LocationID:  locationID,
		SessionID:   req.SessionID,
		Date:        date,
		HoursWorked: req.HoursWorked,
		RoutesSet:   req.RoutesSet,
		Notes:       req.Notes,
	}

	if err := h.labor.Create(r.Context(), log); err != nil {
		Error(w, http.StatusInternalServerError, "failed to log labor")
		return
	}

	JSON(w, http.StatusCreated, log)
}

func (h *LaborHandler) ListByLocation(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	logs, err := h.labor.ListByLocation(r.Context(), locationID, limit, offset)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	JSON(w, http.StatusOK, logs)
}

func (h *LaborHandler) MyLabor(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	logs, err := h.labor.ListBySetter(r.Context(), userID, limit, offset)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	JSON(w, http.StatusOK, logs)
}
