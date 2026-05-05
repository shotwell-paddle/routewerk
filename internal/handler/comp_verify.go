package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/shotwell-paddle/routewerk/internal/api"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/rbac"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

// Phase 1f wave 4 — staff verify + override endpoints.
//
//   POST /api/v1/attempts/{id}/verify    setter+ stamps verified_by
//   POST /api/v1/attempts/{id}/override  setter+ replaces the attempt state

// VerifyAttempt handles POST /api/v1/attempts/{id}/verify.
func (h *CompHandler) VerifyAttempt(w http.ResponseWriter, r *http.Request) {
	att, comp, ok := h.loadAttemptForStaff(w, r, rbac.RoleSetter)
	if !ok {
		return
	}

	actor := middleware.GetUserID(r.Context())
	if err := h.attemptRepo.Verify(r.Context(), att.ID, actor); err != nil {
		slog.Error("verify attempt", "id", att.ID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	// Re-fetch to return the verified_at/by populated.
	updated, err := h.attemptRepo.GetByID(r.Context(), att.ID)
	if err != nil {
		slog.Error("get attempt after verify", "id", att.ID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	// Bust the leaderboard cache for this comp — verification doesn't
	// change scoring fields but it does change attempt metadata that
	// callers may reflect in the response.
	h.cache.invalidate(comp.ID)
	JSON(w, http.StatusOK, attemptToAPI(updated))
}

// OverrideAttempt handles POST /api/v1/attempts/{id}/override.
//
// Wholesale state replacement. Staff is asserting "this attempt is now
// exactly this." A log entry with action=override is appended via
// repo.ApplyAction so the audit trail captures who did what.
func (h *CompHandler) OverrideAttempt(w http.ResponseWriter, r *http.Request) {
	att, comp, ok := h.loadAttemptForStaff(w, r, rbac.RoleSetter)
	if !ok {
		return
	}

	var body api.AttemptOverride
	if err := Decode(r, &body); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Attempts < 0 {
		Error(w, http.StatusBadRequest, "attempts must be >= 0")
		return
	}
	if body.ZoneAttempts != nil && *body.ZoneAttempts < 0 {
		Error(w, http.StatusBadRequest, "zone_attempts must be >= 0")
		return
	}

	currentState := repository.AttemptState{
		Attempts:     att.Attempts,
		ZoneAttempts: att.ZoneAttempts,
		ZoneReached:  att.ZoneReached,
		TopReached:   att.TopReached,
		Notes:        att.Notes,
	}
	newState := repository.AttemptState{
		Attempts:     body.Attempts,
		ZoneAttempts: body.ZoneAttempts,
		ZoneReached:  body.ZoneReached,
		TopReached:   body.TopReached,
	}
	if body.Notes != nil {
		newState.Notes = body.Notes
	}

	beforeJSON, _ := json.Marshal(currentState)
	afterJSON, _ := json.Marshal(newState)

	actor := middleware.GetUserID(r.Context())
	updated, _, err := h.attemptRepo.ApplyAction(r.Context(), repository.ApplyActionInput{
		RegistrationID: att.RegistrationID,
		ProblemID:      att.ProblemID,
		NewState:       newState,
		Action:         model.CompActionOverride,
		ActorUserID:    &actor,
		BeforeJSON:     beforeJSON,
		AfterJSON:      afterJSON,
	})
	if err != nil {
		slog.Error("apply override", "id", att.ID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	h.cache.invalidate(comp.ID)
	JSON(w, http.StatusOK, attemptToAPI(updated))
}

// loadAttemptForStaff resolves: attempt → registration → comp → location;
// then enforces the staff role at the location. Common helper for both
// verify and override.
func (h *CompHandler) loadAttemptForStaff(w http.ResponseWriter, r *http.Request, role string) (*model.CompetitionAttempt, *model.Competition, bool) {
	attemptID := chi.URLParam(r, "id")
	if !isUUID(attemptID) {
		Error(w, http.StatusBadRequest, "invalid attempt id")
		return nil, nil, false
	}
	att, err := h.attemptRepo.GetByID(r.Context(), attemptID)
	if errors.Is(err, repository.ErrAttemptNotFound) {
		Error(w, http.StatusNotFound, "attempt not found")
		return nil, nil, false
	}
	if err != nil {
		slog.Error("get attempt", "id", attemptID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return nil, nil, false
	}
	reg, err := h.regRepo.GetByID(r.Context(), att.RegistrationID)
	if err != nil {
		slog.Error("get registration for attempt", "id", att.RegistrationID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return nil, nil, false
	}
	comp, ok := h.loadComp(w, r, reg.CompetitionID)
	if !ok {
		return nil, nil, false
	}
	if !h.requireCompRole(w, r, comp, role) {
		return nil, nil, false
	}
	return att, comp, true
}

// attemptToAPI converts a model.CompetitionAttempt to the wire shape.
// This is the full attempt (verify/override return shape); the climber
// action endpoint uses the slimmer attemptToAPIState (in comp_action.go)
// for its per-action results.
func attemptToAPI(a *model.CompetitionAttempt) api.CompetitionAttempt {
	id, _ := uuid.Parse(a.ID)
	rid, _ := uuid.Parse(a.RegistrationID)
	pid, _ := uuid.Parse(a.ProblemID)
	out := api.CompetitionAttempt{
		Id:             id,
		RegistrationId: rid,
		ProblemId:      pid,
		Attempts:       a.Attempts,
		ZoneAttempts:   a.ZoneAttempts,
		ZoneReached:    a.ZoneReached,
		TopReached:     a.TopReached,
		Notes:          a.Notes,
		LoggedAt:       a.LoggedAt,
		UpdatedAt:      a.UpdatedAt,
	}
	if a.VerifiedBy != nil {
		if u, err := uuid.Parse(*a.VerifiedBy); err == nil {
			out.VerifiedBy = &u
		}
	}
	if a.VerifiedAt.Valid {
		t := a.VerifiedAt.Time
		out.VerifiedAt = &t
	}
	return out
}
