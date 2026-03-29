package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

type TrainingHandler struct {
	training *repository.TrainingRepo
}

func NewTrainingHandler(training *repository.TrainingRepo) *TrainingHandler {
	return &TrainingHandler{training: training}
}

type createPlanRequest struct {
	ClimberID   string  `json:"climber_id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}

type addItemRequest struct {
	RouteID   *string `json:"route_id,omitempty"`
	SortOrder int     `json:"sort_order"`
	Title     string  `json:"title"`
	Notes     *string `json:"notes,omitempty"`
}

type updateItemRequest struct {
	Completed *bool   `json:"completed,omitempty"`
	Title     *string `json:"title,omitempty"`
	Notes     *string `json:"notes,omitempty"`
}

func (h *TrainingHandler) Create(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	coachID := middleware.GetUserID(r.Context())

	var req createPlanRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ClimberID == "" || req.Name == "" {
		Error(w, http.StatusBadRequest, "climber_id and name are required")
		return
	}

	plan := &model.TrainingPlan{
		CoachID:     coachID,
		ClimberID:   req.ClimberID,
		LocationID:  locationID,
		Name:        req.Name,
		Description: req.Description,
		Active:      true,
	}

	if err := h.training.Create(r.Context(), plan); err != nil {
		Error(w, http.StatusInternalServerError, "failed to create plan")
		return
	}

	JSON(w, http.StatusCreated, plan)
}

func (h *TrainingHandler) List(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")

	plans, err := h.training.ListByLocation(r.Context(), locationID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	if plans == nil {
		plans = []model.TrainingPlan{}
	}

	JSON(w, http.StatusOK, plans)
}

func (h *TrainingHandler) Get(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "planID")

	plan, err := h.training.GetByID(r.Context(), planID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if plan == nil {
		Error(w, http.StatusNotFound, "plan not found")
		return
	}

	JSON(w, http.StatusOK, plan)
}

func (h *TrainingHandler) Update(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "planID")

	plan, err := h.training.GetByID(r.Context(), planID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if plan == nil {
		Error(w, http.StatusNotFound, "plan not found")
		return
	}

	var req struct {
		Name        *string `json:"name,omitempty"`
		Description *string `json:"description,omitempty"`
		Active      *bool   `json:"active,omitempty"`
	}
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name != nil {
		plan.Name = *req.Name
	}
	if req.Description != nil {
		plan.Description = req.Description
	}
	if req.Active != nil {
		plan.Active = *req.Active
	}

	if err := h.training.Update(r.Context(), plan); err != nil {
		Error(w, http.StatusInternalServerError, "failed to update plan")
		return
	}

	plan, _ = h.training.GetByID(r.Context(), planID)
	JSON(w, http.StatusOK, plan)
}

func (h *TrainingHandler) AddItem(w http.ResponseWriter, r *http.Request) {
	planID := chi.URLParam(r, "planID")

	var req addItemRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Title == "" {
		Error(w, http.StatusBadRequest, "title is required")
		return
	}

	item := &model.TrainingPlanItem{
		PlanID:    planID,
		RouteID:   req.RouteID,
		SortOrder: req.SortOrder,
		Title:     req.Title,
		Notes:     req.Notes,
	}

	if err := h.training.AddItem(r.Context(), item); err != nil {
		Error(w, http.StatusInternalServerError, "failed to add item")
		return
	}

	JSON(w, http.StatusCreated, item)
}

func (h *TrainingHandler) UpdateItem(w http.ResponseWriter, r *http.Request) {
	itemID := chi.URLParam(r, "itemID")

	var req updateItemRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	completed := false
	if req.Completed != nil {
		completed = *req.Completed
	}

	if err := h.training.UpdateItem(r.Context(), itemID, completed, req.Title, req.Notes); err != nil {
		Error(w, http.StatusInternalServerError, "failed to update item")
		return
	}

	JSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *TrainingHandler) MyPlans(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	plans, err := h.training.ListByClimber(r.Context(), userID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	JSON(w, http.StatusOK, plans)
}
