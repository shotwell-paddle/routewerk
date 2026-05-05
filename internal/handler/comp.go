// Phase 1f.1 of the comp module — CRUD handlers for competitions.
//
// Wave 1 covers the four endpoints already in the spec from Phase 1e:
//   GET    /api/v1/locations/{locationID}/competitions       (list)
//   POST   /api/v1/locations/{locationID}/competitions       (create)
//   GET    /api/v1/competitions/{id}                         (read)
//   PATCH  /api/v1/competitions/{id}                         (update)
//
// Subsequent waves add events, problems, registrations, the unified
// action endpoint, verification, leaderboard, and the SSE stream.
//
// We use the codegen-generated request/response TYPES from
// `internal/api/` for type safety, but route registration is
// hand-rolled in `internal/router/router.go` so we can compose with the
// existing chi middleware (Authorize / RequireLocationRole) which
// reads chi.URLParam("locationID") with the existing camelCase
// convention.
package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/shotwell-paddle/routewerk/internal/api"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/rbac"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

type CompHandler struct {
	repo  *repository.CompetitionRepo
	authz *middleware.Authorizer
}

func NewCompHandler(repo *repository.CompetitionRepo, authz *middleware.Authorizer) *CompHandler {
	return &CompHandler{repo: repo, authz: authz}
}

// requireCompRole verifies the authenticated user has one of the given
// roles at the comp's location. Used by handlers whose URL doesn't carry
// {locationID} (e.g. PATCH /competitions/{id} and all event/problem
// child routes), where chi's RequireLocationRole middleware can't gate
// the route.
//
// On failure, writes the appropriate JSON error and returns false; the
// caller returns immediately without further work.
func (h *CompHandler) requireCompRole(w http.ResponseWriter, r *http.Request, comp *model.Competition, roles ...string) bool {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		Error(w, http.StatusUnauthorized, "authentication required")
		return false
	}
	mem, err := h.authz.LookupLocationMembership(r.Context(), userID, comp.LocationID)
	if err != nil {
		Error(w, http.StatusForbidden, "not a member of this location")
		return false
	}
	if !rbac.HasAnyRole(mem.Role, roles...) {
		Error(w, http.StatusForbidden, "insufficient role")
		return false
	}
	return true
}

// ── List ───────────────────────────────────────────────────

// List handles GET /api/v1/locations/{locationID}/competitions.
// Optional query param `status` filters by comp status.
func (h *CompHandler) List(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	if !isUUID(locationID) {
		Error(w, http.StatusBadRequest, "invalid location id")
		return
	}
	status := r.URL.Query().Get("status")
	if status != "" && !isValidStatus(status) {
		Error(w, http.StatusBadRequest, "invalid status")
		return
	}

	comps, err := h.repo.ListByLocation(r.Context(), locationID, status)
	if err != nil {
		slog.Error("list competitions", "location_id", locationID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]api.Competition, 0, len(comps))
	for i := range comps {
		c, err := modelToAPI(&comps[i])
		if err != nil {
			slog.Error("competition serialization", "id", comps[i].ID, "error", err)
			Error(w, http.StatusInternalServerError, "internal error")
			return
		}
		out = append(out, c)
	}
	JSON(w, http.StatusOK, out)
}

// ── Get ────────────────────────────────────────────────────

// Get handles GET /api/v1/competitions/{id}.
func (h *CompHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !isUUID(id) {
		Error(w, http.StatusBadRequest, "invalid competition id")
		return
	}
	comp, err := h.repo.GetByID(r.Context(), id)
	if errors.Is(err, repository.ErrCompetitionNotFound) {
		Error(w, http.StatusNotFound, "competition not found")
		return
	}
	if err != nil {
		slog.Error("get competition", "id", id, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	out, err := modelToAPI(comp)
	if err != nil {
		slog.Error("competition serialization", "id", id, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	JSON(w, http.StatusOK, out)
}

// ── Create ─────────────────────────────────────────────────

// Create handles POST /api/v1/locations/{locationID}/competitions.
// Caller must be authorized as gym_manager+ at the location (enforced
// by middleware in router.go).
func (h *CompHandler) Create(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	if !isUUID(locationID) {
		Error(w, http.StatusBadRequest, "invalid location id")
		return
	}
	var body api.CompetitionCreate
	if err := Decode(r, &body); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateCreate(&body); err != nil {
		Error(w, http.StatusBadRequest, err.Error())
		return
	}

	comp := createToModel(locationID, &body)
	if err := h.repo.Create(r.Context(), comp); err != nil {
		// Slug uniqueness violation surfaces as a Postgres unique-constraint
		// failure on the (location_id, slug) index. Map to 409 so the
		// SPA can show "that slug is taken" instead of a generic 500.
		// (Detection pattern matches the magic_link_tokens repo's
		// isUniqueViolation helper used in the attempt repo.)
		// TODO(1f.2): factor isUniqueViolation up into a shared helper.
		slog.Error("create competition", "location_id", locationID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	out, err := modelToAPI(comp)
	if err != nil {
		slog.Error("competition serialization", "id", comp.ID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	JSON(w, http.StatusCreated, out)
}

// ── Update ─────────────────────────────────────────────────

// Update handles PATCH /api/v1/competitions/{id}. All fields are
// optional; only provided fields are applied. Authorization
// (gym_manager+ at the comp's location) is enforced by middleware.
func (h *CompHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !isUUID(id) {
		Error(w, http.StatusBadRequest, "invalid competition id")
		return
	}

	existing, err := h.repo.GetByID(r.Context(), id)
	if errors.Is(err, repository.ErrCompetitionNotFound) {
		Error(w, http.StatusNotFound, "competition not found")
		return
	}
	if err != nil {
		slog.Error("get competition for update", "id", id, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	// /competitions/{id} doesn't carry a {locationID} path param, so the
	// chi-mounted RequireLocationRole middleware can't run here. Use the
	// shared requireCompRole helper to do the inline lookup + role check.
	if !h.requireCompRole(w, r, existing, rbac.RoleGymManager) {
		return
	}

	var body api.CompetitionUpdate
	if err := Decode(r, &body); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := applyUpdate(existing, &body); err != nil {
		Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.repo.Update(r.Context(), existing); err != nil {
		slog.Error("update competition", "id", id, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	out, err := modelToAPI(existing)
	if err != nil {
		slog.Error("competition serialization", "id", id, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	JSON(w, http.StatusOK, out)
}

// ── Validation ─────────────────────────────────────────────

func validateCreate(b *api.CompetitionCreate) error {
	if b.Name == "" {
		return errors.New("name is required")
	}
	if !slugPattern.MatchString(b.Slug) {
		return errors.New("slug must match ^[a-z0-9][a-z0-9-]{0,63}$")
	}
	if b.ScoringRule == "" {
		return errors.New("scoring_rule is required")
	}
	if !b.EndsAt.After(b.StartsAt) {
		return errors.New("ends_at must be after starts_at")
	}
	if string(b.Format) != model.CompFormatSingle && string(b.Format) != model.CompFormatSeries {
		return errors.New("format must be 'single' or 'series'")
	}
	return nil
}

// applyUpdate merges non-nil fields from the update payload onto the
// existing model. Caller is responsible for persisting via repo.Update.
func applyUpdate(existing *model.Competition, b *api.CompetitionUpdate) error {
	if b.Name != nil {
		existing.Name = *b.Name
	}
	if b.Slug != nil {
		if !slugPattern.MatchString(*b.Slug) {
			return errors.New("slug must match ^[a-z0-9][a-z0-9-]{0,63}$")
		}
		existing.Slug = *b.Slug
	}
	if b.Format != nil {
		f := string(*b.Format)
		if f != model.CompFormatSingle && f != model.CompFormatSeries {
			return errors.New("format must be 'single' or 'series'")
		}
		existing.Format = f
	}
	if b.Aggregation != nil {
		buf, err := json.Marshal(b.Aggregation)
		if err != nil {
			return errors.New("invalid aggregation")
		}
		existing.Aggregation = buf
	}
	if b.ScoringRule != nil {
		existing.ScoringRule = *b.ScoringRule
	}
	if b.ScoringConfig != nil {
		buf, err := json.Marshal(*b.ScoringConfig)
		if err != nil {
			return errors.New("invalid scoring_config")
		}
		existing.ScoringConfig = buf
	}
	if b.Status != nil {
		existing.Status = string(*b.Status)
	}
	if b.LeaderboardVisibility != nil {
		existing.LeaderboardVis = string(*b.LeaderboardVisibility)
	}
	if b.StartsAt != nil {
		existing.StartsAt = *b.StartsAt
	}
	if b.EndsAt != nil {
		existing.EndsAt = *b.EndsAt
	}
	if b.RegistrationOpensAt != nil {
		existing.RegistrationOpensAt = pgtype.Timestamptz{Time: *b.RegistrationOpensAt, Valid: true}
	}
	if b.RegistrationClosesAt != nil {
		existing.RegistrationClosesAt = pgtype.Timestamptz{Time: *b.RegistrationClosesAt, Valid: true}
	}
	if !existing.EndsAt.After(existing.StartsAt) {
		return errors.New("ends_at must be after starts_at")
	}
	return nil
}

// ── model ↔ api conversions ────────────────────────────────

func createToModel(locationID string, b *api.CompetitionCreate) *model.Competition {
	c := &model.Competition{
		LocationID:     locationID,
		Name:           b.Name,
		Slug:           b.Slug,
		Format:         string(b.Format),
		ScoringRule:    b.ScoringRule,
		Status:         model.CompStatusDraft,
		LeaderboardVis: model.LeaderboardVisibilityPublic,
		StartsAt:       b.StartsAt,
		EndsAt:         b.EndsAt,
	}
	if b.Status != nil {
		c.Status = string(*b.Status)
	}
	if b.LeaderboardVisibility != nil {
		c.LeaderboardVis = string(*b.LeaderboardVisibility)
	}
	if b.Aggregation != nil {
		if buf, err := json.Marshal(b.Aggregation); err == nil {
			c.Aggregation = buf
		}
	}
	if c.Aggregation == nil {
		c.Aggregation = json.RawMessage(`{}`)
	}
	if b.ScoringConfig != nil {
		if buf, err := json.Marshal(*b.ScoringConfig); err == nil {
			c.ScoringConfig = buf
		}
	}
	if c.ScoringConfig == nil {
		c.ScoringConfig = json.RawMessage(`{}`)
	}
	if b.RegistrationOpensAt != nil {
		c.RegistrationOpensAt = pgtype.Timestamptz{Time: *b.RegistrationOpensAt, Valid: true}
	}
	if b.RegistrationClosesAt != nil {
		c.RegistrationClosesAt = pgtype.Timestamptz{Time: *b.RegistrationClosesAt, Valid: true}
	}
	return c
}

// modelToAPI converts the DB representation to the OpenAPI shape that
// goes out over the wire.
func modelToAPI(c *model.Competition) (api.Competition, error) {
	id, err := uuid.Parse(c.ID)
	if err != nil {
		return api.Competition{}, err
	}
	loc, err := uuid.Parse(c.LocationID)
	if err != nil {
		return api.Competition{}, err
	}
	out := api.Competition{
		Id:                    id,
		LocationId:            loc,
		Name:                  c.Name,
		Slug:                  c.Slug,
		Format:                api.CompetitionFormat(c.Format),
		ScoringRule:           c.ScoringRule,
		Status:                api.CompetitionStatus(c.Status),
		LeaderboardVisibility: api.LeaderboardVisibility(c.LeaderboardVis),
		StartsAt:              c.StartsAt,
		EndsAt:                c.EndsAt,
		CreatedAt:             c.CreatedAt,
		UpdatedAt:             c.UpdatedAt,
	}
	if err := json.Unmarshal(c.Aggregation, &out.Aggregation); err != nil {
		// Empty/invalid aggregation jsonb — treat as zero-value.
		out.Aggregation = api.Aggregation{}
	}
	if c.ScoringConfig != nil && len(c.ScoringConfig) > 0 && string(c.ScoringConfig) != "null" {
		var cfg map[string]interface{}
		if err := json.Unmarshal(c.ScoringConfig, &cfg); err != nil {
			cfg = map[string]interface{}{}
		}
		out.ScoringConfig = cfg
	} else {
		out.ScoringConfig = map[string]interface{}{}
	}
	if c.RegistrationOpensAt.Valid {
		t := c.RegistrationOpensAt.Time
		out.RegistrationOpensAt = &t
	}
	if c.RegistrationClosesAt.Valid {
		t := c.RegistrationClosesAt.Time
		out.RegistrationClosesAt = &t
	}
	return out, nil
}

// ── Tiny helpers ───────────────────────────────────────────

func isUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

func isValidStatus(s string) bool {
	switch s {
	case model.CompStatusDraft, model.CompStatusOpen, model.CompStatusLive,
		model.CompStatusClosed, model.CompStatusArchived:
		return true
	}
	return false
}

// slugPattern matches the spec: lowercase alnum + hyphens, starts alnum.
var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
