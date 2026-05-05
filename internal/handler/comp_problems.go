package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/shotwell-paddle/routewerk/internal/api"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/rbac"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

// Phase 1f wave 2 — competition problems CRUD.
//
// Routes (wired in router.go):
//   GET   /api/v1/events/{id}/problems     any auth
//   POST  /api/v1/events/{id}/problems     head_setter+
//   PATCH /api/v1/problems/{id}            head_setter+
//
// Authz on writes resolves through the parent event → comp.

// ListProblems handles GET /api/v1/events/{id}/problems.
func (h *CompHandler) ListProblems(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "id")
	if !isUUID(eventID) {
		Error(w, http.StatusBadRequest, "invalid event id")
		return
	}
	if _, _, ok := h.findEventAndComp(w, r, eventID); !ok {
		return
	}
	problems, err := h.repo.ListProblems(r.Context(), eventID)
	if err != nil {
		slog.Error("list problems", "event_id", eventID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]api.CompetitionProblem, 0, len(problems))
	for i := range problems {
		p, err := problemToAPI(&problems[i])
		if err != nil {
			slog.Error("problem serialization", "id", problems[i].ID, "error", err)
			Error(w, http.StatusInternalServerError, "internal error")
			return
		}
		out = append(out, p)
	}
	JSON(w, http.StatusOK, out)
}

// CreateProblem handles POST /api/v1/events/{id}/problems.
func (h *CompHandler) CreateProblem(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "id")
	if !isUUID(eventID) {
		Error(w, http.StatusBadRequest, "invalid event id")
		return
	}
	_, comp, ok := h.findEventAndComp(w, r, eventID)
	if !ok {
		return
	}
	if !h.requireCompRole(w, r, comp, rbac.RoleHeadSetter) {
		return
	}
	var body api.ProblemCreate
	if err := Decode(r, &body); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Label == "" {
		Error(w, http.StatusBadRequest, "label is required")
		return
	}
	p := problemCreateToModel(eventID, &body)
	if err := h.repo.CreateProblem(r.Context(), p); err != nil {
		slog.Error("create problem", "event_id", eventID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	out, err := problemToAPI(p)
	if err != nil {
		slog.Error("problem serialization", "id", p.ID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	JSON(w, http.StatusCreated, out)
}

// UpdateProblem handles PATCH /api/v1/problems/{id}.
func (h *CompHandler) UpdateProblem(w http.ResponseWriter, r *http.Request) {
	problemID := chi.URLParam(r, "id")
	if !isUUID(problemID) {
		Error(w, http.StatusBadRequest, "invalid problem id")
		return
	}
	existing, err := h.repo.GetProblemByID(r.Context(), problemID)
	if errors.Is(err, repository.ErrCompetitionNotFound) {
		Error(w, http.StatusNotFound, "problem not found")
		return
	}
	if err != nil {
		slog.Error("get problem", "id", problemID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	// Resolve event → comp for authz.
	_, comp, ok := h.findEventAndComp(w, r, existing.EventID)
	if !ok {
		return
	}
	if !h.requireCompRole(w, r, comp, rbac.RoleHeadSetter) {
		return
	}
	var body api.ProblemUpdate
	if err := Decode(r, &body); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	applyProblemUpdate(existing, &body)
	if err := h.repo.UpdateProblem(r.Context(), existing); err != nil {
		slog.Error("update problem", "id", problemID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	out, err := problemToAPI(existing)
	if err != nil {
		slog.Error("problem serialization", "id", problemID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	JSON(w, http.StatusOK, out)
}

// ── Conversion ─────────────────────────────────────────────

func problemCreateToModel(eventID string, b *api.ProblemCreate) *model.CompetitionProblem {
	p := &model.CompetitionProblem{
		EventID: eventID,
		Label:   b.Label,
	}
	if b.RouteId != nil {
		v := b.RouteId.String()
		p.RouteID = &v
	}
	if b.Points != nil {
		v := float64(*b.Points)
		p.Points = &v
	}
	if b.ZonePoints != nil {
		v := float64(*b.ZonePoints)
		p.ZonePoints = &v
	}
	if b.Grade != nil {
		v := *b.Grade
		p.Grade = &v
	}
	if b.Color != nil {
		v := *b.Color
		p.Color = &v
	}
	if b.SortOrder != nil {
		p.SortOrder = *b.SortOrder
	}
	return p
}

func applyProblemUpdate(existing *model.CompetitionProblem, b *api.ProblemUpdate) {
	if b.Label != nil {
		existing.Label = *b.Label
	}
	if b.RouteId != nil {
		v := b.RouteId.String()
		existing.RouteID = &v
	}
	if b.Points != nil {
		v := float64(*b.Points)
		existing.Points = &v
	}
	if b.ZonePoints != nil {
		v := float64(*b.ZonePoints)
		existing.ZonePoints = &v
	}
	if b.Grade != nil {
		v := *b.Grade
		existing.Grade = &v
	}
	if b.Color != nil {
		v := *b.Color
		existing.Color = &v
	}
	if b.SortOrder != nil {
		existing.SortOrder = *b.SortOrder
	}
}

func problemToAPI(p *model.CompetitionProblem) (api.CompetitionProblem, error) {
	id, err := uuid.Parse(p.ID)
	if err != nil {
		return api.CompetitionProblem{}, err
	}
	eid, err := uuid.Parse(p.EventID)
	if err != nil {
		return api.CompetitionProblem{}, err
	}
	out := api.CompetitionProblem{
		Id:        id,
		EventId:   eid,
		Label:     p.Label,
		SortOrder: p.SortOrder,
	}
	if p.RouteID != nil {
		if rid, err := uuid.Parse(*p.RouteID); err == nil {
			out.RouteId = &rid
		}
	}
	if p.Points != nil {
		v := float32(*p.Points)
		out.Points = &v
	}
	if p.ZonePoints != nil {
		v := float32(*p.ZonePoints)
		out.ZonePoints = &v
	}
	if p.Grade != nil {
		v := *p.Grade
		out.Grade = &v
	}
	if p.Color != nil {
		v := *p.Color
		out.Color = &v
	}
	return out, nil
}
