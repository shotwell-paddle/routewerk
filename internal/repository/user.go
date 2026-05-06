package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shotwell-paddle/routewerk/internal/database"
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
	ctx, cancel := database.QueryTimeout(ctx, database.TimeoutFast)
	defer cancel()

	query := `
		SELECT id, email, password_hash, display_name, avatar_url, bio, is_app_admin, created_at, updated_at
		FROM users
		WHERE id = $1 AND deleted_at IS NULL`

	u := &model.User{}
	err := r.db.QueryRow(ctx, query, id).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName,
		&u.AvatarURL, &u.Bio, &u.IsAppAdmin, &u.CreatedAt, &u.UpdatedAt,
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
		SELECT id, email, password_hash, display_name, avatar_url, bio, is_app_admin, created_at, updated_at
		FROM users
		WHERE email = $1 AND deleted_at IS NULL`

	u := &model.User{}
	err := r.db.QueryRow(ctx, query, email).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName,
		&u.AvatarURL, &u.Bio, &u.IsAppAdmin, &u.CreatedAt, &u.UpdatedAt,
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

// UpdatePassword sets a new bcrypt hash for the given user.
func (r *UserRepo) UpdatePassword(ctx context.Context, userID, passwordHash string) error {
	query := `
		UPDATE users SET password_hash = $2
		WHERE id = $1 AND deleted_at IS NULL`
	ct, err := r.db.Exec(ctx, query, userID, passwordHash)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
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
	return memberships, rows.Err()
}

// SaveRefreshToken stores a hashed refresh token with the given hash scheme.
// scheme must be either "bcrypt" (legacy) or "hmac" (current). All new tokens
// written by the auth service should use "hmac"; the bcrypt branch exists
// only so rows written before the C4 migration can still be looked up.
func (r *UserRepo) SaveRefreshToken(ctx context.Context, userID, tokenHash, scheme string, expiresAt interface{}) error {
	query := `
		INSERT INTO refresh_tokens (user_id, token_hash, hash_scheme, expires_at)
		VALUES ($1, $2, $3, $4)`

	_, err := r.db.Exec(ctx, query, userID, tokenHash, scheme, expiresAt)
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

// RevokeRefreshToken atomically revokes a single refresh token by its hash
// and scheme. Returns true if the token was revoked, false if it was already
// consumed, expired, or the scheme didn't match. Scoping by scheme avoids
// the (astronomically unlikely) case where an HMAC hex digest collides with
// a bcrypt hash string — the two alphabets overlap (both contain hex chars).
func (r *UserRepo) RevokeRefreshToken(ctx context.Context, tokenHash, scheme string) (bool, error) {
	query := `
		UPDATE refresh_tokens
		SET revoked_at = NOW()
		WHERE token_hash = $1 AND hash_scheme = $2
		  AND revoked_at IS NULL AND expires_at > NOW()`

	ct, err := r.db.Exec(ctx, query, tokenHash, scheme)
	if err != nil {
		return false, fmt.Errorf("revoke refresh token: %w", err)
	}
	return ct.RowsAffected() > 0, nil
}

// FindActiveRefreshTokenHMAC looks up an active HMAC-scheme refresh token by
// exact hash match. Returns ("", nil) when no such token exists. This is the
// fast path — an indexed equality lookup — that replaces bcrypt-scanning
// every token for the user.
func (r *UserRepo) FindActiveRefreshTokenHMAC(ctx context.Context, tokenHash string) (string, error) {
	query := `
		SELECT token_hash
		FROM refresh_tokens
		WHERE token_hash = $1 AND hash_scheme = 'hmac'
		  AND revoked_at IS NULL AND expires_at > NOW()`

	var found string
	err := r.db.QueryRow(ctx, query, tokenHash).Scan(&found)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("find hmac refresh token: %w", err)
	}
	return found, nil
}

// GetActiveBcryptRefreshTokens returns all legacy bcrypt-scheme refresh
// token hashes for a user that are still active. Used only by the dual-scheme
// fallback path in Refresh(); once all bcrypt rows have aged out past
// REFRESH_TOKEN_EXPIRY this method (and its caller) can be dropped.
func (r *UserRepo) GetActiveBcryptRefreshTokens(ctx context.Context, userID string) ([]string, error) {
	query := `
		SELECT token_hash
		FROM refresh_tokens
		WHERE user_id = $1 AND hash_scheme = 'bcrypt'
		  AND revoked_at IS NULL AND expires_at > NOW()`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("get bcrypt refresh tokens: %w", err)
	}
	defer rows.Close()

	var hashes []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			return nil, fmt.Errorf("scan bcrypt refresh token: %w", err)
		}
		hashes = append(hashes, h)
	}
	return hashes, rows.Err()
}

// SetterAtLocation holds a setter's basic info for assignment dropdowns.
type SetterAtLocation struct {
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

// ListSettersByLocation returns users with setter, head_setter, or org_admin
// roles at the given location. Used for the session assignment UI.
func (r *UserRepo) ListSettersByLocation(ctx context.Context, locationID string) ([]SetterAtLocation, error) {
	// Include location-specific setters and org-wide admins/managers whose
	// org owns this location (their memberships may have location_id IS NULL).
	// DISTINCT ON picks the highest-ranked role per user via CASE ordering.
	query := `
		SELECT DISTINCT ON (u.id) u.id, u.display_name, um.role
		FROM users u
		JOIN user_memberships um ON um.user_id = u.id
		WHERE (um.location_id = $1
			OR (um.location_id IS NULL AND um.role IN ('org_admin', 'gym_manager')
				AND um.org_id = (SELECT org_id FROM locations WHERE id = $1 AND deleted_at IS NULL)))
		  AND um.role IN ('setter', 'head_setter', 'gym_manager', 'org_admin')
		  AND um.deleted_at IS NULL
		  AND u.deleted_at IS NULL
		ORDER BY u.id, CASE um.role
			WHEN 'org_admin' THEN 1
			WHEN 'gym_manager' THEN 2
			WHEN 'head_setter' THEN 3
			WHEN 'setter' THEN 4
			ELSE 5
		END`

	rows, err := r.db.Query(ctx, query, locationID)
	if err != nil {
		return nil, fmt.Errorf("list setters by location: %w", err)
	}
	defer rows.Close()

	var setters []SetterAtLocation
	for rows.Next() {
		var s SetterAtLocation
		if err := rows.Scan(&s.UserID, &s.DisplayName, &s.Role); err != nil {
			return nil, fmt.Errorf("scan setter: %w", err)
		}
		setters = append(setters, s)
	}
	return setters, rows.Err()
}

// LocationMember holds a user's membership info for the team management page.
type LocationMember struct {
	MembershipID string `json:"membership_id"`
	UserID       string `json:"user_id"`
	DisplayName  string `json:"display_name"`
	Email        string `json:"email"`
	Role         string `json:"role"`
}

// MemberSearchParams controls filtering & pagination for team listing.
type MemberSearchParams struct {
	Query      string // search by name or email (ILIKE)
	RoleFilter string // filter to a specific role, empty = all
	Limit      int
	Offset     int
}

// MemberSearchResult holds a page of members plus the total count.
type MemberSearchResult struct {
	Members    []LocationMember
	TotalCount int
}

// SearchMembersByLocation returns a paginated, searchable list of members at a location.
func (r *UserRepo) SearchMembersByLocation(ctx context.Context, locationID string, p MemberSearchParams) (MemberSearchResult, error) {
	if p.Limit <= 0 {
		p.Limit = 50
	}

	// CTE to deduplicate memberships (pick highest role per user)
	baseCTE := `
	WITH ranked AS (
		SELECT DISTINCT ON (u.id) um.id AS membership_id, u.id AS user_id,
			u.display_name, u.email, um.role,
			CASE um.role
				WHEN 'org_admin' THEN 1 WHEN 'gym_manager' THEN 2
				WHEN 'head_setter' THEN 3 WHEN 'setter' THEN 4
				WHEN 'climber' THEN 5 ELSE 6
			END AS role_rank
		FROM users u
		JOIN user_memberships um ON um.user_id = u.id
		WHERE (um.location_id = $1
			OR (um.location_id IS NULL
				AND um.org_id = (SELECT org_id FROM locations WHERE id = $1 AND deleted_at IS NULL)))
		  AND um.deleted_at IS NULL AND u.deleted_at IS NULL
		ORDER BY u.id, CASE um.role
			WHEN 'org_admin' THEN 1 WHEN 'gym_manager' THEN 2
			WHEN 'head_setter' THEN 3 WHEN 'setter' THEN 4
			WHEN 'climber' THEN 5 ELSE 6
		END
	)`

	// Build WHERE filters
	args := []interface{}{locationID}
	where := ""
	argIdx := 2

	if p.Query != "" {
		where += fmt.Sprintf(" AND (display_name ILIKE $%d OR email ILIKE $%d)", argIdx, argIdx)
		args = append(args, "%"+p.Query+"%")
		argIdx++
	}
	if p.RoleFilter != "" {
		where += fmt.Sprintf(" AND role = $%d::user_role", argIdx)
		args = append(args, p.RoleFilter)
		argIdx++
	}

	// Single query: rows + window count. Replaces the prior separate
	// COUNT + SELECT over the same CTE. See perf audit 2026-04-22 #2.
	dataQuery := baseCTE + fmt.Sprintf(
		" SELECT membership_id, user_id, display_name, email, role, COUNT(*) OVER () AS total_count"+
			" FROM ranked WHERE 1=1%s ORDER BY role_rank, display_name LIMIT $%d OFFSET $%d",
		where, argIdx, argIdx+1,
	)
	args = append(args, p.Limit, p.Offset)

	rows, err := r.db.Query(ctx, dataQuery, args...)
	if err != nil {
		return MemberSearchResult{}, fmt.Errorf("search members: %w", err)
	}
	defer rows.Close()

	var (
		members []LocationMember
		total   int
	)
	for rows.Next() {
		var m LocationMember
		if err := rows.Scan(&m.MembershipID, &m.UserID, &m.DisplayName, &m.Email, &m.Role, &total); err != nil {
			return MemberSearchResult{}, fmt.Errorf("scan member: %w", err)
		}
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		return MemberSearchResult{}, err
	}
	return MemberSearchResult{Members: members, TotalCount: total}, nil
}

// SearchMembersByOrg returns a paginated, searchable list of members across an org.
func (r *UserRepo) SearchMembersByOrg(ctx context.Context, orgID string, p MemberSearchParams) (MemberSearchResult, error) {
	if p.Limit <= 0 {
		p.Limit = 50
	}

	baseCTE := `
	WITH ranked AS (
		SELECT DISTINCT ON (u.id) um.id AS membership_id, u.id AS user_id,
			u.display_name, u.email, um.role,
			CASE um.role
				WHEN 'org_admin' THEN 1 WHEN 'gym_manager' THEN 2
				WHEN 'head_setter' THEN 3 WHEN 'setter' THEN 4
				WHEN 'climber' THEN 5 ELSE 6
			END AS role_rank
		FROM users u
		JOIN user_memberships um ON um.user_id = u.id
		WHERE um.org_id = $1
		  AND um.deleted_at IS NULL AND u.deleted_at IS NULL
		ORDER BY u.id, CASE um.role
			WHEN 'org_admin' THEN 1 WHEN 'gym_manager' THEN 2
			WHEN 'head_setter' THEN 3 WHEN 'setter' THEN 4
			WHEN 'climber' THEN 5 ELSE 6
		END
	)`

	args := []interface{}{orgID}
	where := ""
	argIdx := 2

	if p.Query != "" {
		where += fmt.Sprintf(" AND (display_name ILIKE $%d OR email ILIKE $%d)", argIdx, argIdx)
		args = append(args, "%"+p.Query+"%")
		argIdx++
	}
	if p.RoleFilter != "" {
		where += fmt.Sprintf(" AND role = $%d::user_role", argIdx)
		args = append(args, p.RoleFilter)
		argIdx++
	}

	// Single query: rows + window count. See perf audit 2026-04-22 #2.
	dataQuery := baseCTE + fmt.Sprintf(
		" SELECT membership_id, user_id, display_name, email, role, COUNT(*) OVER () AS total_count"+
			" FROM ranked WHERE 1=1%s ORDER BY role_rank, display_name LIMIT $%d OFFSET $%d",
		where, argIdx, argIdx+1,
	)
	args = append(args, p.Limit, p.Offset)

	rows, err := r.db.Query(ctx, dataQuery, args...)
	if err != nil {
		return MemberSearchResult{}, fmt.Errorf("search org members: %w", err)
	}
	defer rows.Close()

	var (
		members []LocationMember
		total   int
	)
	for rows.Next() {
		var m LocationMember
		if err := rows.Scan(&m.MembershipID, &m.UserID, &m.DisplayName, &m.Email, &m.Role, &total); err != nil {
			return MemberSearchResult{}, fmt.Errorf("scan org member: %w", err)
		}
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		return MemberSearchResult{}, err
	}
	return MemberSearchResult{Members: members, TotalCount: total}, nil
}

// UpdateMemberRole changes a user's role at a specific membership.
// GetMembershipByID returns a single membership by its ID.
func (r *UserRepo) GetMembershipByID(ctx context.Context, membershipID string) (*model.UserMembership, error) {
	var m model.UserMembership
	err := r.db.QueryRow(ctx, `
		SELECT id, user_id, org_id, location_id, role, specialties, created_at, updated_at
		FROM user_memberships WHERE id = $1 AND deleted_at IS NULL`, membershipID).
		Scan(&m.ID, &m.UserID, &m.OrgID, &m.LocationID, &m.Role, &m.Specialties, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *UserRepo) UpdateMemberRole(ctx context.Context, membershipID, newRole string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE user_memberships SET role = $1::user_role, updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL`,
		newRole, membershipID,
	)
	return err
}

// RemoveMembership soft-deletes a membership row. The user can re-join later
// (a new membership row will be created).
func (r *UserRepo) RemoveMembership(ctx context.Context, membershipID string) error {
	ct, err := r.db.Exec(ctx,
		`UPDATE user_memberships SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`,
		membershipID,
	)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("membership not found")
	}
	return nil
}
