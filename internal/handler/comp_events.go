package handler

import (
	"encoding/json"
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

// Phase 1f wave 2 — competition events CRUD.
//
// Routes (wired in router.go):
//   GET   /api/v1/competitions/{id}/events              any auth
//   POST  /api/v1/competitions/{id}/events              head_setter+
//   PATCH /api/v1/events/{id}                           head_setter+
//
// Authz for write operations runs through requireCompRole — the events
// table doesn't carry the location directly, so we resolve via the
// parent competition.

// ListEvents handles GET /api/v1/competitions/{id}/events.
func (h *CompHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	compID := chi.URLParam(r, "id")
	if !isUUID(compID) {
		Error(w, http.StatusBadRequest, "invalid competition id")
		return
	}
	if _, ok := h.loadComp(w, r, compID); !ok {
		return
	}
	events, err := h.repo.ListEvents(r.Context(), compID)
	if err != nil {
		slog.Error("list events", "competition_id", compID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]api.CompetitionEvent, 0, len(events))
	for i := range events {
		ev, err := eventToAPI(&events[i])
		if err != nil {
			slog.Error("event serialization", "id", events[i].ID, "error", err)
			Error(w, http.StatusInternalServerError, "internal error")
			return
		}
		out = append(out, ev)
	}
	JSON(w, http.StatusOK, out)
}

// CreateEvent handles POST /api/v1/competitions/{id}/events.
func (h *CompHandler) CreateEvent(w http.ResponseWriter, r *http.Request) {
	compID := chi.URLParam(r, "id")
	if !isUUID(compID) {
		Error(w, http.StatusBadRequest, "invalid competition id")
		return
	}
	comp, ok := h.loadComp(w, r, compID)
	if !ok {
		return
	}
	if !h.requireCompRole(w, r, comp, rbac.RoleHeadSetter) {
		return
	}
	var body api.EventCreate
	if err := Decode(r, &body); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateEventCreate(&body); err != nil {
		Error(w, http.StatusBadRequest, err.Error())
		return
	}
	ev := eventCreateToModel(compID, &body)
	if err := h.repo.CreateEvent(r.Context(), ev); err != nil {
		slog.Error("create event", "competition_id", compID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	out, err := eventToAPI(ev)
	if err != nil {
		slog.Error("event serialization", "id", ev.ID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	JSON(w, http.StatusCreated, out)
}

// UpdateEvent handles PATCH /api/v1/events/{id}.
//
// To find the comp (and therefore the location for authz), we list
// events under each comp until we find this one. Cleaner long-term:
// add CompetitionRepo.GetEventByID(ctx, eventID) — left as a TODO for
// when the volume of events per comp grows enough that per-event lookup
// matters.
//
// For now: load the event by parent comp via existing repo methods.
func (h *CompHandler) UpdateEvent(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "id")
	if !isUUID(eventID) {
		Error(w, http.StatusBadRequest, "invalid event id")
		return
	}
	existing, comp, ok := h.findEventAndComp(w, r, eventID)
	if !ok {
		return
	}
	if !h.requireCompRole(w, r, comp, rbac.RoleHeadSetter) {
		return
	}
	var body api.EventUpdate
	if err := Decode(r, &body); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := applyEventUpdate(existing, &body); err != nil {
		Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.repo.UpdateEvent(r.Context(), existing); err != nil {
		slog.Error("update event", "id", eventID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	out, err := eventToAPI(existing)
	if err != nil {
		slog.Error("event serialization", "id", eventID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	JSON(w, http.StatusOK, out)
}

// loadComp is a small helper used by every comp-child handler. Loads
// the comp by ID; writes 404/500 + returns false on failure.
func (h *CompHandler) loadComp(w http.ResponseWriter, r *http.Request, compID string) (*model.Competition, bool) {
	comp, err := h.repo.GetByID(r.Context(), compID)
	if errors.Is(err, repository.ErrCompetitionNotFound) {
		Error(w, http.StatusNotFound, "competition not found")
		return nil, false
	}
	if err != nil {
		slog.Error("load competition", "id", compID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return nil, false
	}
	return comp, true
}

// findEventAndComp loads an event by ID and its parent comp. The comp
// is needed because the event itself doesn't carry the location, and
// authz checks resolve through location membership.
func (h *CompHandler) findEventAndComp(w http.ResponseWriter, r *http.Request, eventID string) (*model.CompetitionEvent, *model.Competition, bool) {
	ev, err := h.repo.GetEventByID(r.Context(), eventID)
	if errors.Is(err, repository.ErrCompetitionNotFound) {
		Error(w, http.StatusNotFound, "event not found")
		return nil, nil, false
	}
	if err != nil {
		slog.Error("get event", "id", eventID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return nil, nil, false
	}
	comp, ok := h.loadComp(w, r, ev.CompetitionID)
	if !ok {
		return nil, nil, false
	}
	return ev, comp, true
}

// ── Validation ─────────────────────────────────────────────

func validateEventCreate(b *api.EventCreate) error {
	if b.Name == "" {
		return errors.New("name is required")
	}
	if b.Sequence < 1 {
		return errors.New("sequence must be >= 1")
	}
	if !b.EndsAt.After(b.StartsAt) {
		return errors.New("ends_at must be after starts_at")
	}
	return nil
}

func applyEventUpdate(existing *model.CompetitionEvent, b *api.EventUpdate) error {
	if b.Name != nil {
		if *b.Name == "" {
			return errors.New("name cannot be empty")
		}
		existing.Name = *b.Name
	}
	if b.Sequence != nil {
		if *b.Sequence < 1 {
			return errors.New("sequence must be >= 1")
		}
		existing.Sequence = *b.Sequence
	}
	if b.StartsAt != nil {
		existing.StartsAt = *b.StartsAt
	}
	if b.EndsAt != nil {
		existing.EndsAt = *b.EndsAt
	}
	if b.Weight != nil {
		existing.Weight = float64(*b.Weight)
	}
	if b.ScoringRuleOverride != nil {
		v := *b.ScoringRuleOverride
		if v == "" {
			existing.ScoringRuleOverride = nil
		} else {
			existing.ScoringRuleOverride = &v
		}
	}
	if b.ScoringConfigOverride != nil {
		buf, err := json.Marshal(*b.ScoringConfigOverride)
		if err != nil {
			return errors.New("invalid scoring_config_override")
		}
		existing.ScoringConfigOverride = buf
	}
	if !existing.EndsAt.After(existing.StartsAt) {
		return errors.New("ends_at must be after starts_at")
	}
	return nil
}

// ── Conversion ─────────────────────────────────────────────

func eventCreateToModel(compID string, b *api.EventCreate) *model.CompetitionEvent {
	ev := &model.CompetitionEvent{
		CompetitionID: compID,
		Name:          b.Name,
		Sequence:      b.Sequence,
		StartsAt:      b.StartsAt,
		EndsAt:        b.EndsAt,
		Weight:        1.0,
	}
	if b.Weight != nil {
		ev.Weight = float64(*b.Weight)
	}
	if b.ScoringRuleOverride != nil && *b.ScoringRuleOverride != "" {
		v := *b.ScoringRuleOverride
		ev.ScoringRuleOverride = &v
	}
	if b.ScoringConfigOverride != nil {
		if buf, err := json.Marshal(*b.ScoringConfigOverride); err == nil {
			ev.ScoringConfigOverride = buf
		}
	}
	return ev
}

func eventToAPI(ev *model.CompetitionEvent) (api.CompetitionEvent, error) {
	id, err := uuid.Parse(ev.ID)
	if err != nil {
		return api.CompetitionEvent{}, err
	}
	cid, err := uuid.Parse(ev.CompetitionID)
	if err != nil {
		return api.CompetitionEvent{}, err
	}
	weight := float32(ev.Weight)
	out := api.CompetitionEvent{
		Id:            id,
		CompetitionId: cid,
		Name:          ev.Name,
		Sequence:      ev.Sequence,
		StartsAt:      ev.StartsAt,
		EndsAt:        ev.EndsAt,
		Weight:        weight,
	}
	if ev.ScoringRuleOverride != nil && *ev.ScoringRuleOverride != "" {
		v := *ev.ScoringRuleOverride
		out.ScoringRuleOverride = &v
	}
	if len(ev.ScoringConfigOverride) > 0 && string(ev.ScoringConfigOverride) != "null" {
		var cfg map[string]interface{}
		if err := json.Unmarshal(ev.ScoringConfigOverride, &cfg); err == nil {
			out.ScoringConfigOverride = &cfg
		}
	}
	return out, nil
}
