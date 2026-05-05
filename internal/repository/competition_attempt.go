package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shotwell-paddle/routewerk/internal/database"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

var (
	ErrAttemptNotFound      = errors.New("attempt not found")
	ErrAttemptLogNotFound   = errors.New("attempt log entry not found")
	ErrIdempotencyKeyExists = errors.New("idempotency key already used")
)

type CompetitionAttemptRepo struct {
	db *pgxpool.Pool
}

func NewCompetitionAttemptRepo(db *pgxpool.Pool) *CompetitionAttemptRepo {
	return &CompetitionAttemptRepo{db: db}
}

// ── Reads ──────────────────────────────────────────────────

// GetByID returns an attempt by its UUID. Used by staff verify/override
// flows where we have an attempt id from the leaderboard but not the
// (registration, problem) pair.
func (r *CompetitionAttemptRepo) GetByID(ctx context.Context, id string) (*model.CompetitionAttempt, error) {
	ctx, cancel := database.QueryTimeout(ctx, database.TimeoutFast)
	defer cancel()

	a := &model.CompetitionAttempt{}
	err := r.db.QueryRow(ctx, `
		SELECT id, registration_id, problem_id, attempts, zone_attempts,
			zone_reached, top_reached, notes, logged_at, updated_at,
			verified_by, verified_at
		FROM competition_attempts
		WHERE id = $1`,
		id,
	).Scan(
		&a.ID, &a.RegistrationID, &a.ProblemID, &a.Attempts, &a.ZoneAttempts,
		&a.ZoneReached, &a.TopReached, &a.Notes, &a.LoggedAt, &a.UpdatedAt,
		&a.VerifiedBy, &a.VerifiedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrAttemptNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get attempt by id: %w", err)
	}
	return a, nil
}

// Get returns the current attempt state for (registration, problem).
// Returns ErrAttemptNotFound when the climber hasn't touched the problem;
// callers typically treat that as "all-zero state" rather than a hard error.
func (r *CompetitionAttemptRepo) Get(ctx context.Context, registrationID, problemID string) (*model.CompetitionAttempt, error) {
	ctx, cancel := database.QueryTimeout(ctx, database.TimeoutFast)
	defer cancel()

	a := &model.CompetitionAttempt{}
	err := r.db.QueryRow(ctx, `
		SELECT id, registration_id, problem_id, attempts, zone_attempts,
			zone_reached, top_reached, notes, logged_at, updated_at,
			verified_by, verified_at
		FROM competition_attempts
		WHERE registration_id = $1 AND problem_id = $2`,
		registrationID, problemID,
	).Scan(
		&a.ID, &a.RegistrationID, &a.ProblemID, &a.Attempts, &a.ZoneAttempts,
		&a.ZoneReached, &a.TopReached, &a.Notes, &a.LoggedAt, &a.UpdatedAt,
		&a.VerifiedBy, &a.VerifiedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrAttemptNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get attempt: %w", err)
	}
	return a, nil
}

// ListByRegistration returns the climber's full scorecard for a comp —
// every problem they've touched. Caller joins against the problem set to
// render untouched problems too.
func (r *CompetitionAttemptRepo) ListByRegistration(ctx context.Context, registrationID string) ([]model.CompetitionAttempt, error) {
	ctx, cancel := database.QueryTimeout(ctx, database.TimeoutFast)
	defer cancel()

	rows, err := r.db.Query(ctx, `
		SELECT id, registration_id, problem_id, attempts, zone_attempts,
			zone_reached, top_reached, notes, logged_at, updated_at,
			verified_by, verified_at
		FROM competition_attempts
		WHERE registration_id = $1`,
		registrationID,
	)
	if err != nil {
		return nil, fmt.Errorf("list attempts by registration: %w", err)
	}
	defer rows.Close()

	return scanAttempts(rows)
}

// ListByCompetition returns every attempt across active registrations for
// a comp, optionally restricted to one category. This is the read the
// scorer consumes when computing the leaderboard. Withdrawn registrations
// are excluded.
func (r *CompetitionAttemptRepo) ListByCompetition(ctx context.Context, competitionID, categoryID string) ([]model.CompetitionAttempt, error) {
	ctx, cancel := database.QueryTimeout(ctx, database.TimeoutFast)
	defer cancel()

	var (
		rows pgx.Rows
		err  error
	)
	base := `
		SELECT a.id, a.registration_id, a.problem_id, a.attempts, a.zone_attempts,
			a.zone_reached, a.top_reached, a.notes, a.logged_at, a.updated_at,
			a.verified_by, a.verified_at
		FROM competition_attempts a
		JOIN competition_registrations r ON r.id = a.registration_id
		WHERE r.competition_id = $1 AND r.withdrawn_at IS NULL`
	if categoryID == "" {
		rows, err = r.db.Query(ctx, base, competitionID)
	} else {
		rows, err = r.db.Query(ctx, base+` AND r.category_id = $2`, competitionID, categoryID)
	}
	if err != nil {
		return nil, fmt.Errorf("list attempts by competition: %w", err)
	}
	defer rows.Close()

	return scanAttempts(rows)
}

func scanAttempts(rows pgx.Rows) ([]model.CompetitionAttempt, error) {
	var out []model.CompetitionAttempt
	for rows.Next() {
		var a model.CompetitionAttempt
		if err := rows.Scan(
			&a.ID, &a.RegistrationID, &a.ProblemID, &a.Attempts, &a.ZoneAttempts,
			&a.ZoneReached, &a.TopReached, &a.Notes, &a.LoggedAt, &a.UpdatedAt,
			&a.VerifiedBy, &a.VerifiedAt,
		); err != nil {
			return nil, fmt.Errorf("scan attempt: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// LatestLogForAttempt returns the most recent log entry, used to drive
// undo (the caller restores `before` as the new attempt state).
func (r *CompetitionAttemptRepo) LatestLogForAttempt(ctx context.Context, attemptID string) (*model.CompetitionAttemptLog, error) {
	ctx, cancel := database.QueryTimeout(ctx, database.TimeoutFast)
	defer cancel()

	return r.scanOneLog(ctx, `
		SELECT id, attempt_id, actor_user_id, action, before, after, idempotency_key, at
		FROM competition_attempt_log
		WHERE attempt_id = $1
		ORDER BY at DESC, id DESC
		LIMIT 1`,
		attemptID,
	)
}

// GetLogByIdempotencyKey returns the prior log entry for a given key,
// used after ApplyAction returns ErrIdempotencyKeyExists.
func (r *CompetitionAttemptRepo) GetLogByIdempotencyKey(ctx context.Context, key string) (*model.CompetitionAttemptLog, error) {
	ctx, cancel := database.QueryTimeout(ctx, database.TimeoutFast)
	defer cancel()

	return r.scanOneLog(ctx, `
		SELECT id, attempt_id, actor_user_id, action, before, after, idempotency_key, at
		FROM competition_attempt_log
		WHERE idempotency_key = $1`,
		key,
	)
}

func (r *CompetitionAttemptRepo) scanOneLog(ctx context.Context, query string, args ...any) (*model.CompetitionAttemptLog, error) {
	l := &model.CompetitionAttemptLog{}
	err := r.db.QueryRow(ctx, query, args...).Scan(
		&l.ID, &l.AttemptID, &l.ActorUserID, &l.Action,
		&l.Before, &l.After, &l.IdempotencyKey, &l.At,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrAttemptLogNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan log: %w", err)
	}
	return l, nil
}

// ── Atomic write: ApplyAction ──────────────────────────────

// ApplyActionInput is the full payload for one action endpoint call. The
// service layer is responsible for computing NewState from the climber's
// raw action (increment, top, etc.) plus the prior attempt state; this
// repo just persists what it's told.
type ApplyActionInput struct {
	RegistrationID string
	ProblemID      string
	NewState       AttemptState
	Action         string  // see model.CompAction* constants
	ActorUserID    *string // climber for self-score, staff for verify/override
	IdempotencyKey *string // recommended for climber actions; required by undo
	BeforeJSON     json.RawMessage
	AfterJSON      json.RawMessage
}

// AttemptState is the mutable subset of CompetitionAttempt that ApplyAction
// upserts. Decoupled from the model so the service layer doesn't have to
// construct a full *CompetitionAttempt for a write.
type AttemptState struct {
	Attempts     int
	ZoneAttempts *int
	ZoneReached  bool
	TopReached   bool
	Notes        *string
}

// ApplyAction writes a new attempt state and appends the log entry in a
// single transaction. Returns the resulting attempt and log row.
//
// Idempotency: if an entry with the same IdempotencyKey already exists,
// returns ErrIdempotencyKeyExists; the caller should re-fetch via
// GetLogByIdempotencyKey to return the cached result.
func (r *CompetitionAttemptRepo) ApplyAction(ctx context.Context, in ApplyActionInput) (*model.CompetitionAttempt, *model.CompetitionAttemptLog, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	a := &model.CompetitionAttempt{
		RegistrationID: in.RegistrationID,
		ProblemID:      in.ProblemID,
		Attempts:       in.NewState.Attempts,
		ZoneAttempts:   in.NewState.ZoneAttempts,
		ZoneReached:    in.NewState.ZoneReached,
		TopReached:     in.NewState.TopReached,
		Notes:          in.NewState.Notes,
	}

	if err := tx.QueryRow(ctx, `
		INSERT INTO competition_attempts (
			registration_id, problem_id, attempts, zone_attempts,
			zone_reached, top_reached, notes
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (registration_id, problem_id) DO UPDATE SET
			attempts      = EXCLUDED.attempts,
			zone_attempts = EXCLUDED.zone_attempts,
			zone_reached  = EXCLUDED.zone_reached,
			top_reached   = EXCLUDED.top_reached,
			notes         = EXCLUDED.notes,
			updated_at    = now()
		RETURNING id, logged_at, updated_at`,
		a.RegistrationID, a.ProblemID, a.Attempts, a.ZoneAttempts,
		a.ZoneReached, a.TopReached, a.Notes,
	).Scan(&a.ID, &a.LoggedAt, &a.UpdatedAt); err != nil {
		return nil, nil, fmt.Errorf("upsert attempt: %w", err)
	}

	l := &model.CompetitionAttemptLog{
		AttemptID:      a.ID,
		ActorUserID:    in.ActorUserID,
		Action:         in.Action,
		Before:         in.BeforeJSON,
		After:          in.AfterJSON,
		IdempotencyKey: in.IdempotencyKey,
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO competition_attempt_log (
			attempt_id, actor_user_id, action, before, after, idempotency_key
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, at`,
		l.AttemptID, l.ActorUserID, l.Action, l.Before, l.After, l.IdempotencyKey,
	).Scan(&l.ID, &l.At)
	if err != nil {
		if isUniqueViolation(err, "competition_attempt_log_idem_idx") {
			return nil, nil, ErrIdempotencyKeyExists
		}
		return nil, nil, fmt.Errorf("append log: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, fmt.Errorf("commit: %w", err)
	}
	return a, l, nil
}

// Verify stamps verified_by + verified_at on an attempt. Used by the
// staff verification queue (setter+ role). The verification action is
// also recorded in the attempt log; callers should follow up with
// ApplyAction(action=verify) to keep the audit trail complete.
func (r *CompetitionAttemptRepo) Verify(ctx context.Context, attemptID, verifierUserID string) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE competition_attempts
		    SET verified_by = $2, verified_at = now()
		  WHERE id = $1`,
		attemptID, verifierUserID,
	)
	if err != nil {
		return fmt.Errorf("verify attempt: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrAttemptNotFound
	}
	return nil
}

// ── Helpers ────────────────────────────────────────────────

// isUniqueViolation reports whether err is a Postgres unique_violation
// (SQLSTATE 23505) on the named constraint or index. Constraint name is
// matched as a substring of pgErr.ConstraintName so partial-index names
// (which Postgres reports as the index name) work.
func isUniqueViolation(err error, name string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	if pgErr.Code != pgerrcode.UniqueViolation {
		return false
	}
	return name == "" || pgErr.ConstraintName == name
}
