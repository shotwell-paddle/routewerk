// Competition tracking repositories. Phase 1a of the comp module: covers
// the metadata tables (competitions, events, categories, problems).
// Registrations live in competition_registration.go and attempts in
// competition_attempt.go.
//
// Reads use database.QueryTimeout(TimeoutFast); writes use the request's
// context. No business logic here — validation lives in the model and
// service layers.

package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shotwell-paddle/routewerk/internal/database"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

// ErrNotFound is returned by Get* methods when the row doesn't exist.
// Callers convert this to whatever the protocol expects (HTTP 404, etc.).
var ErrCompetitionNotFound = errors.New("competition not found")

type CompetitionRepo struct {
	db *pgxpool.Pool
}

func NewCompetitionRepo(db *pgxpool.Pool) *CompetitionRepo {
	return &CompetitionRepo{db: db}
}

// ── Competition CRUD ───────────────────────────────────────

func (r *CompetitionRepo) Create(ctx context.Context, c *model.Competition) error {
	query := `
		INSERT INTO competitions (
			location_id, name, slug, format, aggregation, scoring_rule,
			scoring_config, status, leaderboard_visibility,
			starts_at, ends_at, registration_opens_at, registration_closes_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, created_at, updated_at`

	return r.db.QueryRow(ctx, query,
		c.LocationID, c.Name, c.Slug, c.Format, c.Aggregation, c.ScoringRule,
		c.ScoringConfig, c.Status, c.LeaderboardVis,
		c.StartsAt, c.EndsAt, c.RegistrationOpensAt, c.RegistrationClosesAt,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
}

func (r *CompetitionRepo) GetByID(ctx context.Context, id string) (*model.Competition, error) {
	ctx, cancel := database.QueryTimeout(ctx, database.TimeoutFast)
	defer cancel()

	query := `
		SELECT id, location_id, name, slug, format, aggregation, scoring_rule,
			scoring_config, status, leaderboard_visibility,
			starts_at, ends_at, registration_opens_at, registration_closes_at,
			created_at, updated_at
		FROM competitions
		WHERE id = $1`

	c := &model.Competition{}
	err := r.db.QueryRow(ctx, query, id).Scan(
		&c.ID, &c.LocationID, &c.Name, &c.Slug, &c.Format, &c.Aggregation,
		&c.ScoringRule, &c.ScoringConfig, &c.Status, &c.LeaderboardVis,
		&c.StartsAt, &c.EndsAt, &c.RegistrationOpensAt, &c.RegistrationClosesAt,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrCompetitionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get competition by id: %w", err)
	}
	return c, nil
}

// GetBySlug looks up a comp by (location, slug) — the public URL key.
func (r *CompetitionRepo) GetBySlug(ctx context.Context, locationID, slug string) (*model.Competition, error) {
	ctx, cancel := database.QueryTimeout(ctx, database.TimeoutFast)
	defer cancel()

	query := `
		SELECT id, location_id, name, slug, format, aggregation, scoring_rule,
			scoring_config, status, leaderboard_visibility,
			starts_at, ends_at, registration_opens_at, registration_closes_at,
			created_at, updated_at
		FROM competitions
		WHERE location_id = $1 AND slug = $2`

	c := &model.Competition{}
	err := r.db.QueryRow(ctx, query, locationID, slug).Scan(
		&c.ID, &c.LocationID, &c.Name, &c.Slug, &c.Format, &c.Aggregation,
		&c.ScoringRule, &c.ScoringConfig, &c.Status, &c.LeaderboardVis,
		&c.StartsAt, &c.EndsAt, &c.RegistrationOpensAt, &c.RegistrationClosesAt,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrCompetitionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get competition by slug: %w", err)
	}
	return c, nil
}

// ListByLocation returns comps at a location, newest first. Pass an empty
// status to include all statuses.
func (r *CompetitionRepo) ListByLocation(ctx context.Context, locationID, status string) ([]model.Competition, error) {
	ctx, cancel := database.QueryTimeout(ctx, database.TimeoutFast)
	defer cancel()

	var (
		rows pgx.Rows
		err  error
	)
	base := `
		SELECT id, location_id, name, slug, format, aggregation, scoring_rule,
			scoring_config, status, leaderboard_visibility,
			starts_at, ends_at, registration_opens_at, registration_closes_at,
			created_at, updated_at
		FROM competitions
		WHERE location_id = $1`
	if status == "" {
		rows, err = r.db.Query(ctx, base+` ORDER BY starts_at DESC`, locationID)
	} else {
		rows, err = r.db.Query(ctx, base+` AND status = $2 ORDER BY starts_at DESC`, locationID, status)
	}
	if err != nil {
		return nil, fmt.Errorf("list competitions by location: %w", err)
	}
	defer rows.Close()

	var out []model.Competition
	for rows.Next() {
		var c model.Competition
		if err := rows.Scan(
			&c.ID, &c.LocationID, &c.Name, &c.Slug, &c.Format, &c.Aggregation,
			&c.ScoringRule, &c.ScoringConfig, &c.Status, &c.LeaderboardVis,
			&c.StartsAt, &c.EndsAt, &c.RegistrationOpensAt, &c.RegistrationClosesAt,
			&c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan competition: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Update writes the full set of mutable fields. Caller is responsible for
// having loaded + modified a complete Competition (no partial updates).
// updated_at is bumped server-side.
func (r *CompetitionRepo) Update(ctx context.Context, c *model.Competition) error {
	query := `
		UPDATE competitions
		   SET name = $2,
		       slug = $3,
		       format = $4,
		       aggregation = $5,
		       scoring_rule = $6,
		       scoring_config = $7,
		       status = $8,
		       leaderboard_visibility = $9,
		       starts_at = $10,
		       ends_at = $11,
		       registration_opens_at = $12,
		       registration_closes_at = $13,
		       updated_at = now()
		 WHERE id = $1
		 RETURNING updated_at`

	err := r.db.QueryRow(ctx, query,
		c.ID, c.Name, c.Slug, c.Format, c.Aggregation, c.ScoringRule,
		c.ScoringConfig, c.Status, c.LeaderboardVis,
		c.StartsAt, c.EndsAt, c.RegistrationOpensAt, c.RegistrationClosesAt,
	).Scan(&c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrCompetitionNotFound
	}
	if err != nil {
		return fmt.Errorf("update competition: %w", err)
	}
	return nil
}

// ── Events ─────────────────────────────────────────────────

func (r *CompetitionRepo) CreateEvent(ctx context.Context, e *model.CompetitionEvent) error {
	query := `
		INSERT INTO competition_events (
			competition_id, name, sequence, starts_at, ends_at, weight,
			scoring_rule_override, scoring_config_override
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id`

	return r.db.QueryRow(ctx, query,
		e.CompetitionID, e.Name, e.Sequence, e.StartsAt, e.EndsAt, e.Weight,
		e.ScoringRuleOverride, e.ScoringConfigOverride,
	).Scan(&e.ID)
}

// GetEventByID looks up a single event by its UUID. Returns
// ErrCompetitionNotFound (slightly overloaded; the same sentinel covers
// "any comp-tree row not found") when no row matches.
func (r *CompetitionRepo) GetEventByID(ctx context.Context, id string) (*model.CompetitionEvent, error) {
	ctx, cancel := database.QueryTimeout(ctx, database.TimeoutFast)
	defer cancel()

	e := &model.CompetitionEvent{}
	err := r.db.QueryRow(ctx, `
		SELECT id, competition_id, name, sequence, starts_at, ends_at, weight,
			scoring_rule_override, scoring_config_override
		FROM competition_events
		WHERE id = $1`,
		id,
	).Scan(
		&e.ID, &e.CompetitionID, &e.Name, &e.Sequence, &e.StartsAt, &e.EndsAt, &e.Weight,
		&e.ScoringRuleOverride, &e.ScoringConfigOverride,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrCompetitionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get event by id: %w", err)
	}
	return e, nil
}

// GetProblemByID looks up a single problem. See GetEventByID note re:
// the shared ErrCompetitionNotFound sentinel.
func (r *CompetitionRepo) GetProblemByID(ctx context.Context, id string) (*model.CompetitionProblem, error) {
	ctx, cancel := database.QueryTimeout(ctx, database.TimeoutFast)
	defer cancel()

	p := &model.CompetitionProblem{}
	err := r.db.QueryRow(ctx, `
		SELECT id, event_id, route_id, label, points, zone_points, grade, color, sort_order
		FROM competition_problems
		WHERE id = $1`,
		id,
	).Scan(
		&p.ID, &p.EventID, &p.RouteID, &p.Label, &p.Points, &p.ZonePoints,
		&p.Grade, &p.Color, &p.SortOrder,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrCompetitionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get problem by id: %w", err)
	}
	return p, nil
}

func (r *CompetitionRepo) ListEvents(ctx context.Context, competitionID string) ([]model.CompetitionEvent, error) {
	ctx, cancel := database.QueryTimeout(ctx, database.TimeoutFast)
	defer cancel()

	query := `
		SELECT id, competition_id, name, sequence, starts_at, ends_at, weight,
			scoring_rule_override, scoring_config_override
		FROM competition_events
		WHERE competition_id = $1
		ORDER BY sequence`

	rows, err := r.db.Query(ctx, query, competitionID)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()

	var out []model.CompetitionEvent
	for rows.Next() {
		var e model.CompetitionEvent
		if err := rows.Scan(
			&e.ID, &e.CompetitionID, &e.Name, &e.Sequence, &e.StartsAt, &e.EndsAt, &e.Weight,
			&e.ScoringRuleOverride, &e.ScoringConfigOverride,
		); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *CompetitionRepo) UpdateEvent(ctx context.Context, e *model.CompetitionEvent) error {
	query := `
		UPDATE competition_events
		   SET name = $2,
		       sequence = $3,
		       starts_at = $4,
		       ends_at = $5,
		       weight = $6,
		       scoring_rule_override = $7,
		       scoring_config_override = $8
		 WHERE id = $1`
	tag, err := r.db.Exec(ctx, query,
		e.ID, e.Name, e.Sequence, e.StartsAt, e.EndsAt, e.Weight,
		e.ScoringRuleOverride, e.ScoringConfigOverride,
	)
	if err != nil {
		return fmt.Errorf("update event: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrCompetitionNotFound
	}
	return nil
}

// ── Categories ─────────────────────────────────────────────

func (r *CompetitionRepo) CreateCategory(ctx context.Context, c *model.CompetitionCategory) error {
	query := `
		INSERT INTO competition_categories (competition_id, name, sort_order, rules)
		VALUES ($1, $2, $3, $4)
		RETURNING id`
	return r.db.QueryRow(ctx, query,
		c.CompetitionID, c.Name, c.SortOrder, c.Rules,
	).Scan(&c.ID)
}

func (r *CompetitionRepo) ListCategories(ctx context.Context, competitionID string) ([]model.CompetitionCategory, error) {
	ctx, cancel := database.QueryTimeout(ctx, database.TimeoutFast)
	defer cancel()

	query := `
		SELECT id, competition_id, name, sort_order, rules
		FROM competition_categories
		WHERE competition_id = $1
		ORDER BY sort_order, name`

	rows, err := r.db.Query(ctx, query, competitionID)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	defer rows.Close()

	var out []model.CompetitionCategory
	for rows.Next() {
		var c model.CompetitionCategory
		if err := rows.Scan(&c.ID, &c.CompetitionID, &c.Name, &c.SortOrder, &c.Rules); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ── Problems ───────────────────────────────────────────────

func (r *CompetitionRepo) CreateProblem(ctx context.Context, p *model.CompetitionProblem) error {
	query := `
		INSERT INTO competition_problems (
			event_id, route_id, label, points, zone_points, grade, color, sort_order
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id`
	return r.db.QueryRow(ctx, query,
		p.EventID, p.RouteID, p.Label, p.Points, p.ZonePoints, p.Grade, p.Color, p.SortOrder,
	).Scan(&p.ID)
}

func (r *CompetitionRepo) ListProblems(ctx context.Context, eventID string) ([]model.CompetitionProblem, error) {
	ctx, cancel := database.QueryTimeout(ctx, database.TimeoutFast)
	defer cancel()

	query := `
		SELECT id, event_id, route_id, label, points, zone_points, grade, color, sort_order
		FROM competition_problems
		WHERE event_id = $1
		ORDER BY sort_order, label`

	rows, err := r.db.Query(ctx, query, eventID)
	if err != nil {
		return nil, fmt.Errorf("list problems: %w", err)
	}
	defer rows.Close()

	var out []model.CompetitionProblem
	for rows.Next() {
		var p model.CompetitionProblem
		if err := rows.Scan(
			&p.ID, &p.EventID, &p.RouteID, &p.Label, &p.Points, &p.ZonePoints,
			&p.Grade, &p.Color, &p.SortOrder,
		); err != nil {
			return nil, fmt.Errorf("scan problem: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *CompetitionRepo) UpdateProblem(ctx context.Context, p *model.CompetitionProblem) error {
	query := `
		UPDATE competition_problems
		   SET route_id = $2,
		       label = $3,
		       points = $4,
		       zone_points = $5,
		       grade = $6,
		       color = $7,
		       sort_order = $8
		 WHERE id = $1`
	tag, err := r.db.Exec(ctx, query,
		p.ID, p.RouteID, p.Label, p.Points, p.ZonePoints, p.Grade, p.Color, p.SortOrder,
	)
	if err != nil {
		return fmt.Errorf("update problem: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrCompetitionNotFound
	}
	return nil
}
