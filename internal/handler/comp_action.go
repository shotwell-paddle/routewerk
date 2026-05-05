package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/shotwell-paddle/routewerk/internal/api"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

// Phase 1f wave 3 — POST /api/v1/competitions/{id}/actions, the unified
// climber-action endpoint.
//
// Each action is independent and idempotent. The batch is processed in
// order; failures don't abort. Authorization happens once at the
// request level (caller must own the registration); per-action checks
// (problem belongs to comp, event window not closed) generate per-item
// rejections in the response.
//
// Wave 5 will add SSE publish on success — for now the response payload
// is the only signal back to the SPA.
//
// Per-batch limit (50 actions) is enforced in the spec; we re-check
// here as defense in depth against schema bypass.
const maxActionsPerBatch = 50

// SubmitActions handles POST /api/v1/competitions/{id}/actions.
func (h *CompHandler) SubmitActions(w http.ResponseWriter, r *http.Request) {
	compID := chi.URLParam(r, "id")
	if !isUUID(compID) {
		Error(w, http.StatusBadRequest, "invalid competition id")
		return
	}
	comp, ok := h.loadComp(w, r, compID)
	if !ok {
		return
	}
	if comp.Status != model.CompStatusOpen && comp.Status != model.CompStatusLive {
		Error(w, http.StatusForbidden, "competition is not accepting actions")
		return
	}

	callerID := middleware.GetUserID(r.Context())
	if callerID == "" {
		Error(w, http.StatusUnauthorized, "authentication required")
		return
	}
	reg, err := h.regRepo.GetByCompAndUser(r.Context(), compID, callerID)
	if errors.Is(err, repository.ErrRegistrationNotFound) {
		Error(w, http.StatusForbidden, "not registered for this competition")
		return
	}
	if err != nil {
		slog.Error("registration lookup", "competition_id", compID, "user_id", callerID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !reg.IsActive() {
		Error(w, http.StatusForbidden, "registration is withdrawn")
		return
	}

	var body api.ActionsRequest
	if err := Decode(r, &body); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(body.Actions) == 0 {
		Error(w, http.StatusBadRequest, "actions cannot be empty")
		return
	}
	if len(body.Actions) > maxActionsPerBatch {
		Error(w, http.StatusBadRequest, "too many actions in batch")
		return
	}

	// Pre-load all events + problems for the comp so per-action lookups
	// hit memory. League-night batches are small but the loop's still
	// happier without per-action SQL round-trips.
	events, err := h.repo.ListEvents(r.Context(), compID)
	if err != nil {
		slog.Error("list events", "competition_id", compID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	eventByID := make(map[string]*model.CompetitionEvent, len(events))
	for i := range events {
		eventByID[events[i].ID] = &events[i]
	}

	problemByID := map[string]*model.CompetitionProblem{}
	problemEventID := map[string]string{}
	for i := range events {
		ps, err := h.repo.ListProblems(r.Context(), events[i].ID)
		if err != nil {
			slog.Error("list problems", "event_id", events[i].ID, "error", err)
			Error(w, http.StatusInternalServerError, "internal error")
			return
		}
		for j := range ps {
			problemByID[ps[j].ID] = &ps[j]
			problemEventID[ps[j].ID] = events[i].ID
		}
	}

	resp := api.ActionsResponse{
		Applied:  []api.ActionApplied{},
		Rejected: []api.ActionRejected{},
		State:    []api.AttemptState{},
	}
	touched := map[string]struct{}{} // problem_id → present
	now := time.Now()

	for _, action := range body.Actions {
		problemID := action.ProblemId.String()

		// Idempotency: replay if the key was already used.
		if action.IdempotencyKey != nil {
			if cached, ok := h.replayIfCached(r, action); ok {
				resp.Applied = append(resp.Applied, cached)
				touched[problemID] = struct{}{}
				continue
			}
		}

		problem, exists := problemByID[problemID]
		if !exists {
			resp.Rejected = append(resp.Rejected, rejection(action, api.UnknownProblem))
			continue
		}
		event := eventByID[problemEventID[problem.ID]]
		if event == nil {
			// Defensive — the indexes are built together so this is impossible
			// unless the underlying data is inconsistent.
			resp.Rejected = append(resp.Rejected, rejection(action, api.WrongCompetition))
			continue
		}
		if now.After(event.EndsAt) {
			resp.Rejected = append(resp.Rejected, rejection(action, api.EventClosed))
			continue
		}

		// Compute the new state. Undo can't be expressed by computeNewState
		// (it's a function of log history, not just current+type), so the
		// undo branch handles its own state lookup.
		applied, rej, err := h.applyOne(r, reg, problem, action)
		if err != nil {
			slog.Error("apply action", "registration_id", reg.ID, "problem_id", problemID, "error", err)
			resp.Rejected = append(resp.Rejected, rejection(action, api.ServerError))
			continue
		}
		if rej != nil {
			resp.Rejected = append(resp.Rejected, *rej)
			continue
		}
		resp.Applied = append(resp.Applied, *applied)
		touched[problemID] = struct{}{}
	}

	// State snapshot for every problem touched by this batch.
	for problemID := range touched {
		att, err := h.attemptRepo.Get(r.Context(), reg.ID, problemID)
		if errors.Is(err, repository.ErrAttemptNotFound) {
			// Could happen if a successful undo brought the row back to
			// not-yet-existent state. Surface zero-state.
			resp.State = append(resp.State, zeroState(problemID))
			continue
		}
		if err != nil {
			slog.Error("state snapshot", "registration_id", reg.ID, "problem_id", problemID, "error", err)
			continue
		}
		resp.State = append(resp.State, attemptToAPIState(att))
	}

	JSON(w, http.StatusOK, resp)
}

// applyOne runs a single action: loads current state, computes new state
// (or restores from log for undo), persists via the atomic repo method,
// and returns the applied result. A nil-error nil-rejection combo means
// the action was applied; non-nil rejection means a per-action failure
// the caller should record (other actions in the batch still proceed).
func (h *CompHandler) applyOne(r *http.Request, reg *model.CompetitionRegistration, problem *model.CompetitionProblem, action api.ActionItem) (*api.ActionApplied, *api.ActionRejected, error) {
	current, err := h.attemptRepo.Get(r.Context(), reg.ID, problem.ID)
	notExists := errors.Is(err, repository.ErrAttemptNotFound)
	if err != nil && !notExists {
		return nil, nil, err
	}
	currentState := repository.AttemptState{}
	if !notExists {
		currentState = repository.AttemptState{
			Attempts:     current.Attempts,
			ZoneAttempts: current.ZoneAttempts,
			ZoneReached:  current.ZoneReached,
			TopReached:   current.TopReached,
			Notes:        current.Notes,
		}
	}

	var newState repository.AttemptState

	if action.Type == api.Undo {
		if notExists {
			rej := rejection(action, api.NoHistory)
			return nil, &rej, nil
		}
		latest, err := h.attemptRepo.LatestLogForAttempt(r.Context(), current.ID)
		if errors.Is(err, repository.ErrAttemptLogNotFound) {
			rej := rejection(action, api.NoHistory)
			return nil, &rej, nil
		}
		if err != nil {
			return nil, nil, err
		}
		// Restore the prior state from the log entry being undone.
		var restored repository.AttemptState
		if len(latest.Before) > 0 {
			if err := json.Unmarshal(latest.Before, &restored); err != nil {
				return nil, nil, err
			}
		}
		newState = restored
	} else {
		ns, err := computeNewState(currentState, action.Type)
		if err != nil {
			rej := rejection(action, api.ServerError)
			return nil, &rej, nil
		}
		newState = ns
	}

	beforeJSON, _ := json.Marshal(currentState)
	afterJSON, _ := json.Marshal(newState)

	var idemKey *string
	if action.IdempotencyKey != nil {
		v := action.IdempotencyKey.String()
		idemKey = &v
	}
	actor := middleware.GetUserID(r.Context())

	attempt, _, err := h.attemptRepo.ApplyAction(r.Context(), repository.ApplyActionInput{
		RegistrationID: reg.ID,
		ProblemID:      problem.ID,
		NewState:       newState,
		Action:         string(action.Type),
		ActorUserID:    &actor,
		IdempotencyKey: idemKey,
		BeforeJSON:     beforeJSON,
		AfterJSON:      afterJSON,
	})
	if errors.Is(err, repository.ErrIdempotencyKeyExists) {
		// Concurrent retry won the race; replay from the log. Should be
		// rare given the upfront idempotency check earlier in the loop.
		if idemKey == nil {
			return nil, nil, err // shouldn't happen — only keyed actions can collide
		}
		entry, lerr := h.attemptRepo.GetLogByIdempotencyKey(r.Context(), *idemKey)
		if lerr != nil {
			return nil, nil, lerr
		}
		var cachedState repository.AttemptState
		if len(entry.After) > 0 {
			_ = json.Unmarshal(entry.After, &cachedState)
		}
		replayed := true
		applied := api.ActionApplied{
			IdempotencyKey: action.IdempotencyKey,
			ProblemId:      action.ProblemId,
			State:          stateToAPI(action.ProblemId, cachedState),
			Replayed:       &replayed,
		}
		return &applied, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}

	applied := api.ActionApplied{
		IdempotencyKey: action.IdempotencyKey,
		ProblemId:      action.ProblemId,
		State:          attemptToAPIState(attempt),
	}
	return &applied, nil, nil
}

// replayIfCached returns (applied, true) if the action's idempotency key
// matches a prior log entry; (zero, false) otherwise.
func (h *CompHandler) replayIfCached(r *http.Request, action api.ActionItem) (api.ActionApplied, bool) {
	key := action.IdempotencyKey.String()
	entry, err := h.attemptRepo.GetLogByIdempotencyKey(r.Context(), key)
	if errors.Is(err, repository.ErrAttemptLogNotFound) {
		return api.ActionApplied{}, false
	}
	if err != nil {
		// Best-effort: any error here just means we proceed to apply
		// fresh; the unique-key constraint catches it if it would dupe.
		return api.ActionApplied{}, false
	}
	var cached repository.AttemptState
	if len(entry.After) > 0 {
		_ = json.Unmarshal(entry.After, &cached)
	}
	replayed := true
	return api.ActionApplied{
		IdempotencyKey: action.IdempotencyKey,
		ProblemId:      action.ProblemId,
		State:          stateToAPI(action.ProblemId, cached),
		Replayed:       &replayed,
	}, true
}

// ── State computation (pure function — testable) ───────────

// computeNewState applies an action to the current state. Pure function;
// rigorously tested. Undo is handled outside this — it requires log
// history, not just (current, type).
func computeNewState(current repository.AttemptState, action api.ActionType) (repository.AttemptState, error) {
	switch action {
	case api.Increment:
		next := current
		next.Attempts++
		return next, nil
	case api.Zone:
		next := current
		// Zone marks "I just reached the zone on the current attempt count."
		// If the climber forgot to log "+1 attempt" first, we coerce to 1
		// so the recorded zone_attempts is at least plausible.
		if next.Attempts < 1 {
			next.Attempts = 1
		}
		next.ZoneReached = true
		zoneAttempts := next.Attempts
		next.ZoneAttempts = &zoneAttempts
		return next, nil
	case api.Top:
		next := current
		if next.Attempts < 1 {
			next.Attempts = 1
		}
		next.TopReached = true
		// Top implies zone (you climbed past it). Set zone_reached if
		// not already, with zone_attempts at the current attempt count.
		if !next.ZoneReached {
			next.ZoneReached = true
			zoneAttempts := next.Attempts
			next.ZoneAttempts = &zoneAttempts
		}
		return next, nil
	case api.Reset:
		// Zero everything for this problem. Climber within grace window,
		// or staff via override; the route registration enforces who can
		// hit this endpoint at all.
		return repository.AttemptState{}, nil
	case api.Undo:
		return current, errors.New("undo cannot be computed from (current, type) alone")
	default:
		return current, errors.New("unknown action type: " + string(action))
	}
}

// ── Conversion helpers ─────────────────────────────────────

func attemptToAPIState(a *model.CompetitionAttempt) api.AttemptState {
	pid, _ := uuid.Parse(a.ProblemID)
	return api.AttemptState{
		ProblemId:    pid,
		Attempts:     a.Attempts,
		ZoneAttempts: a.ZoneAttempts,
		ZoneReached:  a.ZoneReached,
		TopReached:   a.TopReached,
	}
}

func stateToAPI(problemID uuid.UUID, s repository.AttemptState) api.AttemptState {
	return api.AttemptState{
		ProblemId:    problemID,
		Attempts:     s.Attempts,
		ZoneAttempts: s.ZoneAttempts,
		ZoneReached:  s.ZoneReached,
		TopReached:   s.TopReached,
	}
}

func zeroState(problemID string) api.AttemptState {
	pid, _ := uuid.Parse(problemID)
	return api.AttemptState{ProblemId: pid}
}

func rejection(action api.ActionItem, reason api.ActionRejectedReason) api.ActionRejected {
	return api.ActionRejected{
		IdempotencyKey: action.IdempotencyKey,
		ProblemId:      action.ProblemId,
		Reason:         reason,
	}
}
