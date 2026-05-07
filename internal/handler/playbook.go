package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

// PlaybookHandler exposes the location playbook (default session
// checklist template) as JSON. Mirrors the HTMX setter pages at
// internal/handler/web/sessions_lifecycle.go::Playbook* and the
// existing SessionRepo CRUD methods.
type PlaybookHandler struct {
	sessions *repository.SessionRepo
}

func NewPlaybookHandler(sessions *repository.SessionRepo) *PlaybookHandler {
	return &PlaybookHandler{sessions: sessions}
}

// List — GET /api/v1/locations/{locationID}/playbook.
//
// Any setter+ at the location can read; checklist templates are
// shop-internal but no climber-facing data lives in them. Empty list
// when the location hasn't customized its playbook yet.
func (h *PlaybookHandler) List(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	if !isUUID(locationID) {
		Error(w, http.StatusBadRequest, "invalid location id")
		return
	}
	steps, err := h.sessions.ListPlaybookSteps(r.Context(), locationID)
	if err != nil {
		slog.Error("list playbook", "location_id", locationID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if steps == nil {
		steps = []model.LocationPlaybookStep{}
	}
	JSON(w, http.StatusOK, steps)
}

type playbookStepRequest struct {
	Title string `json:"title"`
}

// Create — POST /api/v1/locations/{locationID}/playbook { title }.
// Appends a step to the end of the playbook (sort_order auto-derived).
// head_setter+ enforced by router middleware.
func (h *PlaybookHandler) Create(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	if !isUUID(locationID) {
		Error(w, http.StatusBadRequest, "invalid location id")
		return
	}
	var req playbookStepRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		Error(w, http.StatusBadRequest, "title is required")
		return
	}
	if len(title) > 200 {
		Error(w, http.StatusBadRequest, "title too long (max 200 chars)")
		return
	}
	step, err := h.sessions.CreatePlaybookStep(r.Context(), locationID, title)
	if err != nil {
		slog.Error("create playbook step", "location_id", locationID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	JSON(w, http.StatusCreated, step)
}

// resolveStep is a small helper that pulls the step + verifies it
// belongs to the URL's locationID, returning false (after writing the
// response) on mismatch.
func (h *PlaybookHandler) resolveStep(w http.ResponseWriter, r *http.Request, locationID, stepID string) (*model.LocationPlaybookStep, bool) {
	step, err := h.sessions.GetPlaybookStep(r.Context(), stepID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return nil, false
	}
	if step == nil || step.LocationID != locationID {
		Error(w, http.StatusNotFound, "step not found")
		return nil, false
	}
	return step, true
}

// Update — PATCH /api/v1/locations/{locationID}/playbook/{stepID} { title }.
// head_setter+ enforced by router middleware on the parent block.
func (h *PlaybookHandler) Update(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	stepID := chi.URLParam(r, "stepID")
	if !isUUID(locationID) || !isUUID(stepID) {
		Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	if _, ok := h.resolveStep(w, r, locationID, stepID); !ok {
		return
	}
	var req playbookStepRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		Error(w, http.StatusBadRequest, "title is required")
		return
	}
	if len(title) > 200 {
		Error(w, http.StatusBadRequest, "title too long (max 200 chars)")
		return
	}
	if err := h.sessions.UpdatePlaybookStep(r.Context(), stepID, title); err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Delete — DELETE /api/v1/locations/{locationID}/playbook/{stepID}.
// Step rows aren't foreign-keyed by checklist items, so deletes don't
// cascade. head_setter+.
func (h *PlaybookHandler) Delete(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	stepID := chi.URLParam(r, "stepID")
	if !isUUID(locationID) || !isUUID(stepID) {
		Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	if _, ok := h.resolveStep(w, r, locationID, stepID); !ok {
		return
	}
	if err := h.sessions.DeletePlaybookStep(r.Context(), stepID); err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type movePlaybookRequest struct {
	Direction string `json:"direction"`
}

// Move — POST /api/v1/locations/{locationID}/playbook/{stepID}/move
// { direction }. Swaps with neighbor "up" or "down" in sort_order.
func (h *PlaybookHandler) Move(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	stepID := chi.URLParam(r, "stepID")
	if !isUUID(locationID) || !isUUID(stepID) {
		Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	if _, ok := h.resolveStep(w, r, locationID, stepID); !ok {
		return
	}
	var req movePlaybookRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Direction != "up" && req.Direction != "down" {
		Error(w, http.StatusBadRequest, "direction must be 'up' or 'down'")
		return
	}
	if err := h.sessions.MovePlaybookStep(r.Context(), stepID, req.Direction); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			Error(w, http.StatusNotFound, "step not found")
			return
		}
		slog.Error("move playbook step", "step_id", stepID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
