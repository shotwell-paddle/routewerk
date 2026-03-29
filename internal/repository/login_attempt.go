package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// LoginAttemptRepo tracks failed login attempts for account lockout.
// The backing table is:
//
//	CREATE TABLE login_attempts (
//	    email           TEXT NOT NULL,
//	    failed_count    INT NOT NULL DEFAULT 0,
//	    locked_until    TIMESTAMPTZ,
//	    last_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
//	    PRIMARY KEY (email)
//	);
type LoginAttemptRepo struct {
	db *pgxpool.Pool
}

func NewLoginAttemptRepo(db *pgxpool.Pool) *LoginAttemptRepo {
	return &LoginAttemptRepo{db: db}
}

// IsLocked returns true if the given email is currently locked out.
func (r *LoginAttemptRepo) IsLocked(ctx context.Context, email string) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM login_attempts
			WHERE email = $1 AND locked_until IS NOT NULL AND locked_until > NOW()
		)`

	var locked bool
	err := r.db.QueryRow(ctx, query, email).Scan(&locked)
	if err != nil {
		return false, fmt.Errorf("check lockout: %w", err)
	}
	return locked, nil
}

// RecordFailure increments the failure count and locks the account if the
// threshold is reached. Returns the new failure count and the lock-until
// time (zero if not locked).
func (r *LoginAttemptRepo) RecordFailure(ctx context.Context, email string, maxAttempts int, lockDuration time.Duration) (int, time.Time, error) {
	// Upsert: increment counter, auto-lock when threshold reached
	query := `
		INSERT INTO login_attempts (email, failed_count, last_attempt_at)
		VALUES ($1, 1, NOW())
		ON CONFLICT (email) DO UPDATE SET
			failed_count = CASE
				-- Reset counter if last attempt was before the lock window expired
				WHEN login_attempts.locked_until IS NOT NULL AND login_attempts.locked_until < NOW()
					THEN 1
				ELSE login_attempts.failed_count + 1
			END,
			locked_until = CASE
				WHEN (CASE
						WHEN login_attempts.locked_until IS NOT NULL AND login_attempts.locked_until < NOW()
							THEN 1
						ELSE login_attempts.failed_count + 1
					END) >= $2
					THEN NOW() + $3::interval
				ELSE login_attempts.locked_until
			END,
			last_attempt_at = NOW()
		RETURNING failed_count, COALESCE(locked_until, '1970-01-01'::timestamptz)`

	var count int
	var lockedUntil time.Time
	err := r.db.QueryRow(ctx, query, email, maxAttempts, lockDuration.String()).Scan(&count, &lockedUntil)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("record failure: %w", err)
	}

	if lockedUntil.Before(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)) {
		lockedUntil = time.Time{}
	}

	return count, lockedUntil, nil
}

// ClearFailures resets the counter after a successful login.
func (r *LoginAttemptRepo) ClearFailures(ctx context.Context, email string) error {
	query := `DELETE FROM login_attempts WHERE email = $1`
	_, err := r.db.Exec(ctx, query, email)
	if err != nil {
		return fmt.Errorf("clear failures: %w", err)
	}
	return nil
}
