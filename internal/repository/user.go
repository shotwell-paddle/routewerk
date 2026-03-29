package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

type UserRepo struct {
	db *pgxpool.Pool
}

func NewUserRepo(db *pgxpool.Pool) *UserRepo {
	return &UserRepo{db: db}
}

func (r *UserRepo) Create(ctx context.Context, u *model.User) error {
	query := `
		INSERT INTO users (email, password_hash, display_name, avatar_url, bio)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at`

	return r.db.QueryRow(ctx, query,
		u.Email, u.PasswordHash, u.DisplayName, u.AvatarURL, u.Bio,
	).Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt)
}

func (r *UserRepo) GetByID(ctx context.Context, id string) (*model.User, error) {
	query := `
		SELECT id, email, password_hash, display_name, avatar_url, bio, created_at, updated_at
		FROM users
		WHERE id = $1 AND deleted_at IS NULL`

	u := &model.User{}
	err := r.db.QueryRow(ctx, query, id).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName,
		&u.AvatarURL, &u.Bio, &u.CreatedAt, &u.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return u, nil
}

func (r *UserRepo) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	query := `
		SELECT id, email, password_hash, display_name, avatar_url, bio, created_at, updated_at
		FROM users
		WHERE email = $1 AND deleted_at IS NULL`

	u := &model.User{}
	err := r.db.QueryRow(ctx, query, email).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName,
		&u.AvatarURL, &u.Bio, &u.CreatedAt, &u.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return u, nil
}

func (r *UserRepo) Update(ctx context.Context, u *model.User) error {
	query := `
		UPDATE users
		SET display_name = $2, avatar_url = $3, bio = $4
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING updated_at`

	return r.db.QueryRow(ctx, query,
		u.ID, u.DisplayName, u.AvatarURL, u.Bio,
	).Scan(&u.UpdatedAt)
}

func (r *UserRepo) GetMemberships(ctx context.Context, userID string) ([]model.UserMembership, error) {
	query := `
		SELECT id, user_id, org_id, location_id, role, specialties, created_at, updated_at
		FROM user_memberships
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY created_at`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("get memberships: %w", err)
	}
	defer rows.Close()

	var memberships []model.UserMembership
	for rows.Next() {
		var m model.UserMembership
		if err := rows.Scan(
			&m.ID, &m.UserID, &m.OrgID, &m.LocationID,
			&m.Role, &m.Specialties, &m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan membership: %w", err)
		}
		memberships = append(memberships, m)
	}
	return memberships, nil
}

// SaveRefreshToken stores a hashed refresh token.
func (r *UserRepo) SaveRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt interface{}) error {
	query := `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)`

	_, err := r.db.Exec(ctx, query, userID, tokenHash, expiresAt)
	if err != nil {
		return fmt.Errorf("save refresh token: %w", err)
	}
	return nil
}

// RevokeRefreshTokens revokes all refresh tokens for a user.
func (r *UserRepo) RevokeRefreshTokens(ctx context.Context, userID string) error {
	query := `
		UPDATE refresh_tokens
		SET revoked_at = NOW()
		WHERE user_id = $1 AND revoked_at IS NULL`

	_, err := r.db.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("revoke refresh tokens: %w", err)
	}
	return nil
}

// GetActiveRefreshTokens returns all active (non-revoked, non-expired) refresh tokens for a user.
func (r *UserRepo) GetActiveRefreshTokens(ctx context.Context, userID string) ([]string, error) {
	query := `
		SELECT token_hash
		FROM refresh_tokens
		WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > NOW()`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("get refresh tokens: %w", err)
	}
	defer rows.Close()

	var hashes []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			return nil, fmt.Errorf("scan refresh token: %w", err)
		}
		hashes = append(hashes, h)
	}
	return hashes, nil
}
