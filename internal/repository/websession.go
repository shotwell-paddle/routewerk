package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

// WebSessionRepo handles CRUD for browser-based sessions.
type WebSessionRepo struct {
	db *pgxpool.Pool
}

func NewWebSessionRepo(db *pgxpool.Pool) *WebSessionRepo {
	return &WebSessionRepo{db: db}
}

// Create inserts a new web session row and returns the populated model.
func (r *WebSessionRepo) Create(ctx context.Context, s *model.WebSession) error {
	return r.db.QueryRow(ctx, `
		INSERT INTO web_sessions (user_id, location_id, token_hash, ip_address, user_agent, expires_at)
		VALUES ($1, $2, $3, $4::inet, $5, $6)
		RETURNING id, created_at, last_seen_at`,
		s.UserID, s.LocationID, s.TokenHash, s.IPAddress, s.UserAgent, s.ExpiresAt,
	).Scan(&s.ID, &s.CreatedAt, &s.LastSeenAt)
}

// GetByTokenHash fetches a non-expired session by its token hash.
// Returns nil if not found or expired.
func (r *WebSessionRepo) GetByTokenHash(ctx context.Context, tokenHash string) (*model.WebSession, error) {
	s := &model.WebSession{}
	err := r.db.QueryRow(ctx, `
		SELECT id, user_id, location_id, token_hash, ip_address::text, user_agent,
		       created_at, expires_at, last_seen_at
		FROM web_sessions
		WHERE token_hash = $1 AND expires_at > NOW()`,
		tokenHash,
	).Scan(
		&s.ID, &s.UserID, &s.LocationID, &s.TokenHash,
		&s.IPAddress, &s.UserAgent,
		&s.CreatedAt, &s.ExpiresAt, &s.LastSeenAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return s, nil
}

// TouchLastSeen bumps the last_seen_at timestamp so we can track active sessions.
func (r *WebSessionRepo) TouchLastSeen(ctx context.Context, sessionID string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE web_sessions SET last_seen_at = NOW() WHERE id = $1`,
		sessionID,
	)
	return err
}

// Delete removes a single session (logout).
func (r *WebSessionRepo) Delete(ctx context.Context, sessionID string) error {
	_, err := r.db.Exec(ctx,
		`DELETE FROM web_sessions WHERE id = $1`,
		sessionID,
	)
	return err
}

// DeleteAllForUser removes every session for a user (e.g. password change).
func (r *WebSessionRepo) DeleteAllForUser(ctx context.Context, userID string) error {
	_, err := r.db.Exec(ctx,
		`DELETE FROM web_sessions WHERE user_id = $1`,
		userID,
	)
	return err
}

// DeleteExpired cleans up expired sessions. Call periodically.
func (r *WebSessionRepo) DeleteExpired(ctx context.Context) (int64, error) {
	tag, err := r.db.Exec(ctx,
		`DELETE FROM web_sessions WHERE expires_at <= NOW()`,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// UpdateLocation switches the active location for a session.
func (r *WebSessionRepo) UpdateLocation(ctx context.Context, sessionID, locationID string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE web_sessions SET location_id = $1 WHERE id = $2`,
		locationID, sessionID,
	)
	return err
}

// ListForUser returns all active sessions for a user, newest first.
func (r *WebSessionRepo) ListForUser(ctx context.Context, userID string) ([]model.WebSession, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, user_id, location_id, ip_address::text, user_agent,
		       created_at, expires_at, last_seen_at
		FROM web_sessions
		WHERE user_id = $1 AND expires_at > NOW()
		ORDER BY last_seen_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []model.WebSession
	for rows.Next() {
		var s model.WebSession
		if err := rows.Scan(
			&s.ID, &s.UserID, &s.LocationID,
			&s.IPAddress, &s.UserAgent,
			&s.CreatedAt, &s.ExpiresAt, &s.LastSeenAt,
		); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// CountForUser returns the number of active sessions for a user.
func (r *WebSessionRepo) CountForUser(ctx context.Context, userID string) (int, error) {
	var count int
	err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM web_sessions WHERE user_id = $1 AND expires_at > NOW()`,
		userID,
	).Scan(&count)
	return count, err
}

// maxSessionsPerUser limits the number of concurrent browser sessions per user.
const maxSessionsPerUser = 10

// EnforceLimit deletes the oldest sessions if the user exceeds the max.
// Call after creating a new session.
func (r *WebSessionRepo) EnforceLimit(ctx context.Context, userID string) error {
	_, err := r.db.Exec(ctx, `
		DELETE FROM web_sessions
		WHERE id IN (
			SELECT id FROM web_sessions
			WHERE user_id = $1 AND expires_at > NOW()
			ORDER BY last_seen_at DESC
			OFFSET $2
		)`,
		userID, maxSessionsPerUser,
	)
	return err
}

// SessionExpiry is a convenience to calculate the absolute expiry from a duration.
func SessionExpiry(maxAge time.Duration) time.Time {
	return time.Now().Add(maxAge)
}
