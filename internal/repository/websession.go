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

// GetByTokenHash fetches a non-expired, non-revoked session by its token hash.
// Returns nil if not found, expired, or revoked.
func (r *WebSessionRepo) GetByTokenHash(ctx context.Context, tokenHash string) (*model.WebSession, error) {
	s := &model.WebSession{}
	err := r.db.QueryRow(ctx, `
		SELECT id, user_id, location_id, token_hash, ip_address::text, user_agent,
		       created_at, expires_at, last_seen_at
		FROM web_sessions
		WHERE token_hash = $1 AND expires_at > NOW() AND revoked_at IS NULL`,
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

// AuthContext bundles the session + user + memberships loaded in one
// round-trip by GetAuthContextByTokenHash. It's the hot-path struct for
// RequireSession middleware: three dependent table reads collapsed into a
// single pool acquire.
type AuthContext struct {
	Session     *model.WebSession
	User        *model.User
	Memberships []model.UserMembership
}

// GetAuthContextByTokenHash resolves the full web-auth context (session,
// user, and all non-deleted memberships) in a single query.
//
// Before this method existed, the middleware fired three serial queries
// against a pool of 5 connections on Fly shared PG — see the 2026-04-22
// perf audit, finding #1. The row-expansion on memberships is 1-to-N per
// session, but sessions are short-lived and users rarely belong to more
// than a handful of locations, so the expansion is bounded.
//
// Returns (nil, nil) when the session is missing, expired, revoked, or
// the user has been soft-deleted. Callers should treat that as a signed-out
// state.
func (r *WebSessionRepo) GetAuthContextByTokenHash(ctx context.Context, tokenHash string) (*AuthContext, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			s.id, s.user_id, s.location_id, s.token_hash, s.ip_address::text, s.user_agent,
			s.created_at, s.expires_at, s.last_seen_at,
			u.email, u.password_hash, u.display_name, u.avatar_url, u.bio, u.is_app_admin,
			u.created_at, u.updated_at,
			m.id, m.org_id, m.location_id, m.role, m.specialties, m.created_at, m.updated_at
		FROM web_sessions s
		JOIN users u ON u.id = s.user_id AND u.deleted_at IS NULL
		LEFT JOIN user_memberships m
			ON m.user_id = u.id AND m.deleted_at IS NULL
		WHERE s.token_hash = $1 AND s.expires_at > NOW() AND s.revoked_at IS NULL
		ORDER BY m.created_at NULLS FIRST`,
		tokenHash,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var auth *AuthContext
	for rows.Next() {
		var (
			s             model.WebSession
			u             model.User
			mID           *string
			mOrgID        *string
			mLocationID   *string
			mRole         *string
			mSpecialties  []string
			mCreatedAt    *time.Time
			mUpdatedAt    *time.Time
		)

		if err := rows.Scan(
			&s.ID, &s.UserID, &s.LocationID, &s.TokenHash, &s.IPAddress, &s.UserAgent,
			&s.CreatedAt, &s.ExpiresAt, &s.LastSeenAt,
			&u.Email, &u.PasswordHash, &u.DisplayName, &u.AvatarURL, &u.Bio, &u.IsAppAdmin,
			&u.CreatedAt, &u.UpdatedAt,
			&mID, &mOrgID, &mLocationID, &mRole, &mSpecialties, &mCreatedAt, &mUpdatedAt,
		); err != nil {
			return nil, err
		}

		if auth == nil {
			// First row — materialise the session + user. Subsequent rows
			// will only contribute membership rows (the LEFT JOIN fans out).
			u.ID = s.UserID
			sessCopy := s
			userCopy := u
			auth = &AuthContext{Session: &sessCopy, User: &userCopy}
		}

		// LEFT JOIN may yield a single row with NULL membership columns when
		// the user has no memberships yet. Skip those.
		if mID != nil && mOrgID != nil && mRole != nil && mCreatedAt != nil && mUpdatedAt != nil {
			auth.Memberships = append(auth.Memberships, model.UserMembership{
				ID:          *mID,
				UserID:      s.UserID,
				OrgID:       *mOrgID,
				LocationID:  mLocationID,
				Role:        *mRole,
				Specialties: mSpecialties,
				CreatedAt:   *mCreatedAt,
				UpdatedAt:   *mUpdatedAt,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return auth, nil
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

// RevokeAllForUser soft-revokes every active session for a user by setting
// revoked_at. Use this on password change, role change, or forced logout so
// the sessions remain in the DB for audit purposes but are no longer valid.
func (r *WebSessionRepo) RevokeAllForUser(ctx context.Context, userID string) (int64, error) {
	tag, err := r.db.Exec(ctx,
		`UPDATE web_sessions SET revoked_at = NOW() WHERE user_id = $1 AND revoked_at IS NULL`,
		userID,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// RevokeAllForUserExcept soft-revokes all sessions for a user except the given
// session ID. Use this when a user changes their password — invalidate all
// other sessions but keep the current one alive.
func (r *WebSessionRepo) RevokeAllForUserExcept(ctx context.Context, userID, keepSessionID string) (int64, error) {
	tag, err := r.db.Exec(ctx,
		`UPDATE web_sessions SET revoked_at = NOW()
		 WHERE user_id = $1 AND id != $2 AND revoked_at IS NULL`,
		userID, keepSessionID,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
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
		WHERE user_id = $1 AND expires_at > NOW() AND revoked_at IS NULL
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
		`SELECT COUNT(*) FROM web_sessions WHERE user_id = $1 AND expires_at > NOW() AND revoked_at IS NULL`,
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
			WHERE user_id = $1 AND expires_at > NOW() AND revoked_at IS NULL
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
