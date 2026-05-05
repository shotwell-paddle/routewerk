package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/shotwell-paddle/routewerk/internal/api"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/rbac"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

// Phase 1f wave 3 — registration endpoints.
//
//   GET    /api/v1/competitions/{id}/registrations   any auth
//   POST   /api/v1/competitions/{id}/registrations   self, or staff for other
//   DELETE /api/v1/registrations/{id}                self or staff

// ListRegistrations handles GET /api/v1/competitions/{id}/registrations.
func (h *CompHandler) ListRegistrations(w http.ResponseWriter, r *http.Request) {
	compID := chi.URLParam(r, "id")
	if !isUUID(compID) {
		Error(w, http.StatusBadRequest, "invalid competition id")
		return
	}
	if _, ok := h.loadComp(w, r, compID); !ok {
		return
	}
	regs, err := h.regRepo.ListByCompetition(r.Context(), compID, "")
	if err != nil {
		slog.Error("list registrations", "competition_id", compID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]api.CompetitionRegistration, 0, len(regs))
	for i := range regs {
		reg, err := registrationToAPI(&regs[i])
		if err != nil {
			slog.Error("registration serialization", "id", regs[i].ID, "error", err)
			Error(w, http.StatusInternalServerError, "internal error")
			return
		}
		out = append(out, reg)
	}
	JSON(w, http.StatusOK, out)
}

// CreateRegistration handles POST /api/v1/competitions/{id}/registrations.
//
// Caller may register themselves with no `user_id` field. Setting a
// different `user_id` requires `gym_manager+` at the comp's location
// (so staff can register climbers walk-up if needed; the *flow*
// pre-1d called for true walk-ins which we cut, but staff-assisted
// registration is still useful for league night sign-ups).
func (h *CompHandler) CreateRegistration(w http.ResponseWriter, r *http.Request) {
	compID := chi.URLParam(r, "id")
	if !isUUID(compID) {
		Error(w, http.StatusBadRequest, "invalid competition id")
		return
	}
	comp, ok := h.loadComp(w, r, compID)
	if !ok {
		return
	}
	callerID := middleware.GetUserID(r.Context())
	if callerID == "" {
		Error(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var body api.RegistrationCreate
	if err := Decode(r, &body); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	targetUserID := callerID
	if body.UserId != nil && body.UserId.String() != callerID {
		// Staff registering someone else.
		if !h.requireCompRole(w, r, comp, rbac.RoleGymManager) {
			return
		}
		targetUserID = body.UserId.String()
	}

	user, err := h.userRepo.GetByID(r.Context(), targetUserID)
	if err != nil {
		slog.Error("user lookup for registration", "user_id", targetUserID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if user == nil {
		Error(w, http.StatusNotFound, "user not found")
		return
	}

	displayName := user.DisplayName
	if body.DisplayName != nil && *body.DisplayName != "" {
		displayName = *body.DisplayName
	}

	reg := &model.CompetitionRegistration{
		CompetitionID: compID,
		CategoryID:    body.CategoryId.String(),
		UserID:        targetUserID,
		DisplayName:   displayName,
		BibNumber:     body.BibNumber,
	}

	if err := h.regRepo.Create(r.Context(), reg); err != nil {
		// Unique-violation on (competition_id, user_id) → already registered.
		// We don't have a shared isUniqueViolation helper at handler scope yet
		// (TODO: factor out from competition_attempt.go); for now we surface
		// any error as 500 except the very common "already registered" path
		// — staff/SPA will retry with the existing registration ID.
		slog.Error("create registration", "competition_id", compID, "user_id", targetUserID, "error", err)
		Error(w, http.StatusInternalServerError, "could not create registration")
		return
	}

	out, err := registrationToAPI(reg)
	if err != nil {
		slog.Error("registration serialization", "id", reg.ID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	JSON(w, http.StatusCreated, out)
}

// WithdrawRegistration handles DELETE /api/v1/registrations/{id}.
func (h *CompHandler) WithdrawRegistration(w http.ResponseWriter, r *http.Request) {
	regID := chi.URLParam(r, "id")
	if !isUUID(regID) {
		Error(w, http.StatusBadRequest, "invalid registration id")
		return
	}
	reg, err := h.regRepo.GetByID(r.Context(), regID)
	if errors.Is(err, repository.ErrRegistrationNotFound) {
		Error(w, http.StatusNotFound, "registration not found")
		return
	}
	if err != nil {
		slog.Error("get registration", "id", regID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	callerID := middleware.GetUserID(r.Context())
	if callerID == "" {
		Error(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if reg.UserID != callerID {
		// Not self → must be staff at the comp's location.
		comp, ok := h.loadComp(w, r, reg.CompetitionID)
		if !ok {
			return
		}
		if !h.requireCompRole(w, r, comp, rbac.RoleGymManager) {
			return
		}
	}

	if err := h.regRepo.Withdraw(r.Context(), regID); err != nil {
		if errors.Is(err, repository.ErrRegistrationNotFound) {
			// Already withdrawn — treat as idempotent success.
			w.WriteHeader(http.StatusNoContent)
			return
		}
		slog.Error("withdraw registration", "id", regID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Conversion ─────────────────────────────────────────────

func registrationToAPI(r *model.CompetitionRegistration) (api.CompetitionRegistration, error) {
	id, err := uuid.Parse(r.ID)
	if err != nil {
		return api.CompetitionRegistration{}, err
	}
	cid, err := uuid.Parse(r.CompetitionID)
	if err != nil {
		return api.CompetitionRegistration{}, err
	}
	catID, err := uuid.Parse(r.CategoryID)
	if err != nil {
		return api.CompetitionRegistration{}, err
	}
	uid, err := uuid.Parse(r.UserID)
	if err != nil {
		return api.CompetitionRegistration{}, err
	}
	out := api.CompetitionRegistration{
		Id:            id,
		CompetitionId: cid,
		CategoryId:    catID,
		UserId:        uid,
		DisplayName:   r.DisplayName,
		BibNumber:     r.BibNumber,
		CreatedAt:     r.CreatedAt,
	}
	if t := nilableTime(r.WaiverSignedAt); t != nil {
		out.WaiverSignedAt = t
	}
	if t := nilableTime(r.PaidAt); t != nil {
		out.PaidAt = t
	}
	if t := nilableTime(r.WithdrawnAt); t != nil {
		out.WithdrawnAt = t
	}
	return out, nil
}

// nilableTime returns a pointer to the timestamptz's value, or nil if
// the value is invalid (i.e. the row's column is NULL).
func nilableTime(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	t := ts.Time
	return &t
}
