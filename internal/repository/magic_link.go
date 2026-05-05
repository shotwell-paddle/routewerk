package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shotwell-paddle/routewerk/internal/database"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

var ErrMagicLinkNotFound = errors.New("magic link token not found")

type MagicLinkRepo struct {
	db *pgxpool.Pool
}

func NewMagicLinkRepo(db *pgxpool.Pool) *MagicLinkRepo {
	return &MagicLinkRepo{db: db}
}

// Create inserts a new magic-link token row. id, created_at are
// returned populated.
func (r *MagicLinkRepo) Create(ctx context.Context, t *model.MagicLinkToken) error {
	return r.db.QueryRow(ctx, `
		INSERT INTO magic_link_tokens (
			email, user_id, token_hash, next_path,
			requested_ip, user_agent, expires_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at`,
		t.Email, t.UserID, t.TokenHash, t.NextPath,
		t.RequestedIP, t.UserAgent, t.ExpiresAt,
	).Scan(&t.ID, &t.CreatedAt)
}

// GetByHash looks up a token by its sha256 hash. Returns ErrMagicLinkNotFound
// if no row matches. Does NOT enforce expiry or consumed state — callers
// check those separately so they can produce specific error messages.
func (r *MagicLinkRepo) GetByHash(ctx context.Context, hash []byte) (*model.MagicLinkToken, error) {
	ctx, cancel := database.QueryTimeout(ctx, database.TimeoutFast)
	defer cancel()

	t := &model.MagicLinkToken{}
	err := r.db.QueryRow(ctx, `
		SELECT id, email, user_id, token_hash, next_path,
			requested_ip, user_agent, expires_at, consumed_at, created_at
		FROM magic_link_tokens
		WHERE token_hash = $1`,
		hash,
	).Scan(
		&t.ID, &t.Email, &t.UserID, &t.TokenHash, &t.NextPath,
		&t.RequestedIP, &t.UserAgent, &t.ExpiresAt, &t.ConsumedAt, &t.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrMagicLinkNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get magic link by hash: %w", err)
	}
	return t, nil
}

// Consume atomically marks a token as consumed and returns the token row.
// Returns ErrMagicLinkNotFound if the token is unknown, already consumed,
// or expired — distinguishing these would let an attacker probe whether a
// token was valid.
//
// The single-use guarantee comes from the WHERE clause: only a row whose
// consumed_at is still NULL gets the update. Two concurrent verifies for
// the same token will see one win and one return ErrMagicLinkNotFound.
func (r *MagicLinkRepo) Consume(ctx context.Context, hash []byte, now time.Time) (*model.MagicLinkToken, error) {
	t := &model.MagicLinkToken{}
	err := r.db.QueryRow(ctx, `
		UPDATE magic_link_tokens
		   SET consumed_at = $2
		 WHERE token_hash = $1
		   AND consumed_at IS NULL
		   AND expires_at > $2
		RETURNING id, email, user_id, token_hash, next_path,
			requested_ip, user_agent, expires_at, consumed_at, created_at`,
		hash, now,
	).Scan(
		&t.ID, &t.Email, &t.UserID, &t.TokenHash, &t.NextPath,
		&t.RequestedIP, &t.UserAgent, &t.ExpiresAt, &t.ConsumedAt, &t.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrMagicLinkNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("consume magic link: %w", err)
	}
	return t, nil
}

// CountRecentByEmail returns how many tokens have been created for the
// given email within the last `since` window. Used to enforce the
// per-email rate limit (e.g. max 3 in 15 minutes) at the service layer.
//
// We could enforce in middleware via LimitByKey, but doing it at the
// repo level lets us count even consumed tokens (which prevents a user
// who consumed a token from immediately requesting another).
func (r *MagicLinkRepo) CountRecentByEmail(ctx context.Context, email string, since time.Time) (int, error) {
	ctx, cancel := database.QueryTimeout(ctx, database.TimeoutFast)
	defer cancel()

	var count int
	err := r.db.QueryRow(ctx,
		`SELECT count(*) FROM magic_link_tokens WHERE email = $1 AND created_at >= $2`,
		email, since,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count recent magic links: %w", err)
	}
	return count, nil
}

// DeleteExpired removes consumed and expired rows older than `before`.
// Intended for periodic cleanup; the table is small in normal operation
// (one row per request) but unbounded over time without this.
func (r *MagicLinkRepo) DeleteExpired(ctx context.Context, before time.Time) (int64, error) {
	tag, err := r.db.Exec(ctx,
		`DELETE FROM magic_link_tokens WHERE expires_at < $1 OR consumed_at < $1`,
		before,
	)
	if err != nil {
		return 0, fmt.Errorf("delete expired magic links: %w", err)
	}
	return tag.RowsAffected(), nil
}
