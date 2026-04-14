package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

type LocationRepo struct {
	db *pgxpool.Pool
}

func NewLocationRepo(db *pgxpool.Pool) *LocationRepo {
	return &LocationRepo{db: db}
}

func (r *LocationRepo) Create(ctx context.Context, l *model.Location) error {
	query := `
		INSERT INTO locations (org_id, name, slug, address, timezone, website_url, phone, hours_json, day_pass_info, waiver_url, allow_shared_setters, custom_domain)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, created_at, updated_at`

	return r.db.QueryRow(ctx, query,
		l.OrgID, l.Name, l.Slug, l.Address, l.Timezone,
		l.WebsiteURL, l.Phone, l.HoursJSON, l.DayPassInfo,
		l.WaiverURL, l.AllowSharedSetters, l.CustomDomain,
	).Scan(&l.ID, &l.CreatedAt, &l.UpdatedAt)
}

func (r *LocationRepo) GetByID(ctx context.Context, id string) (*model.Location, error) {
	query := `
		SELECT id, org_id, name, slug, address, timezone, website_url, phone,
			hours_json, day_pass_info, waiver_url, allow_shared_setters,
			custom_domain, progressions_enabled, created_at, updated_at
		FROM locations
		WHERE id = $1 AND deleted_at IS NULL`

	l := &model.Location{}
	err := r.db.QueryRow(ctx, query, id).Scan(
		&l.ID, &l.OrgID, &l.Name, &l.Slug, &l.Address, &l.Timezone,
		&l.WebsiteURL, &l.Phone, &l.HoursJSON, &l.DayPassInfo,
		&l.WaiverURL, &l.AllowSharedSetters, &l.CustomDomain, &l.ProgressionsEnabled,
		&l.CreatedAt, &l.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get location by id: %w", err)
	}
	return l, nil
}

func (r *LocationRepo) ListByOrg(ctx context.Context, orgID string) ([]model.Location, error) {
	query := `
		SELECT id, org_id, name, slug, address, timezone, website_url, phone,
			hours_json, day_pass_info, waiver_url, allow_shared_setters,
			custom_domain, progressions_enabled, created_at, updated_at
		FROM locations
		WHERE org_id = $1 AND deleted_at IS NULL
		ORDER BY name`

	rows, err := r.db.Query(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("list locations by org: %w", err)
	}
	defer rows.Close()

	var locations []model.Location
	for rows.Next() {
		var l model.Location
		if err := rows.Scan(
			&l.ID, &l.OrgID, &l.Name, &l.Slug, &l.Address, &l.Timezone,
			&l.WebsiteURL, &l.Phone, &l.HoursJSON, &l.DayPassInfo,
			&l.WaiverURL, &l.AllowSharedSetters, &l.CustomDomain, &l.ProgressionsEnabled,
			&l.CreatedAt, &l.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan location: %w", err)
		}
		locations = append(locations, l)
	}
	return locations, rows.Err()
}

// SearchPublic finds locations by name (case-insensitive substring match).
// Returns up to `limit` results with org name included. Used for the
// "find your gym" page — no auth context needed.
func (r *LocationRepo) SearchPublic(ctx context.Context, query string, limit int) ([]LocationSearchResult, error) {
	if limit <= 0 {
		limit = 20
	}

	q := `
		SELECT l.id, l.org_id, l.name, l.address, o.name AS org_name
		FROM locations l
		JOIN organizations o ON o.id = l.org_id
		WHERE l.deleted_at IS NULL
		  AND o.deleted_at IS NULL
		  AND l.name ILIKE '%' || $1 || '%'
		ORDER BY l.name
		LIMIT $2`

	rows, err := r.db.Query(ctx, q, query, limit)
	if err != nil {
		return nil, fmt.Errorf("search locations: %w", err)
	}
	defer rows.Close()

	var results []LocationSearchResult
	for rows.Next() {
		var sr LocationSearchResult
		if err := rows.Scan(&sr.ID, &sr.OrgID, &sr.Name, &sr.Address, &sr.OrgName); err != nil {
			return nil, fmt.Errorf("scan location search result: %w", err)
		}
		results = append(results, sr)
	}
	return results, rows.Err()
}

// ListAllPublic returns all locations (no search filter), for the initial
// gym selection page before the user types a query.
func (r *LocationRepo) ListAllPublic(ctx context.Context, limit int) ([]LocationSearchResult, error) {
	if limit <= 0 {
		limit = 50
	}

	q := `
		SELECT l.id, l.org_id, l.name, l.address, o.name AS org_name
		FROM locations l
		JOIN organizations o ON o.id = l.org_id
		WHERE l.deleted_at IS NULL
		  AND o.deleted_at IS NULL
		ORDER BY l.name
		LIMIT $1`

	rows, err := r.db.Query(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("list public locations: %w", err)
	}
	defer rows.Close()

	var results []LocationSearchResult
	for rows.Next() {
		var sr LocationSearchResult
		if err := rows.Scan(&sr.ID, &sr.OrgID, &sr.Name, &sr.Address, &sr.OrgName); err != nil {
			return nil, fmt.Errorf("scan location: %w", err)
		}
		results = append(results, sr)
	}
	return results, rows.Err()
}

// LocationSearchResult is a lightweight location view for search results.
type LocationSearchResult struct {
	ID      string  `json:"id"`
	OrgID   string  `json:"org_id"`
	Name    string  `json:"name"`
	Address *string `json:"address,omitempty"`
	OrgName string  `json:"org_name"`
}

// UserLocationItem is a lightweight location view for the location switcher.
type UserLocationItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"` // user's best role at this location
}

// ListForUser returns all locations the user has access to, with their best role
// at each location. Includes locations from direct memberships and locations
// belonging to orgs where the user has org-wide roles.
func (r *LocationRepo) ListForUser(ctx context.Context, userID string) ([]UserLocationItem, error) {
	query := `
		SELECT DISTINCT ON (l.id) l.id, l.name, um.role
		FROM locations l
		JOIN user_memberships um ON (
			um.location_id = l.id
			OR (um.location_id IS NULL AND um.org_id = l.org_id)
		)
		WHERE um.user_id = $1
		  AND um.deleted_at IS NULL
		  AND l.deleted_at IS NULL
		ORDER BY l.id, CASE um.role
			WHEN 'org_admin' THEN 1
			WHEN 'gym_manager' THEN 2
			WHEN 'head_setter' THEN 3
			WHEN 'setter' THEN 4
			WHEN 'climber' THEN 5
			ELSE 6
		END`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list locations for user: %w", err)
	}
	defer rows.Close()

	var locations []UserLocationItem
	for rows.Next() {
		var li UserLocationItem
		if err := rows.Scan(&li.ID, &li.Name, &li.Role); err != nil {
			return nil, fmt.Errorf("scan user location: %w", err)
		}
		locations = append(locations, li)
	}
	return locations, rows.Err()
}

// SetProgressionsEnabled toggles the progressions feature flag for a location.
// This is the dark-launch gate: when false, /quests returns 404, climber-facing
// progressions UI is hidden, and quest-related event listeners short-circuit.
func (r *LocationRepo) SetProgressionsEnabled(ctx context.Context, id string, enabled bool) error {
	query := `
		UPDATE locations
		SET progressions_enabled = $2
		WHERE id = $1 AND deleted_at IS NULL`

	tag, err := r.db.Exec(ctx, query, id, enabled)
	if err != nil {
		return fmt.Errorf("set progressions_enabled: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *LocationRepo) Update(ctx context.Context, l *model.Location) error {
	query := `
		UPDATE locations
		SET name = $2, slug = $3, address = $4, timezone = $5, website_url = $6,
			phone = $7, hours_json = $8, day_pass_info = $9, waiver_url = $10,
			allow_shared_setters = $11, custom_domain = $12
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING updated_at`

	return r.db.QueryRow(ctx, query,
		l.ID, l.Name, l.Slug, l.Address, l.Timezone,
		l.WebsiteURL, l.Phone, l.HoursJSON, l.DayPassInfo,
		l.WaiverURL, l.AllowSharedSetters, l.CustomDomain,
	).Scan(&l.UpdatedAt)
}

// GetByCustomDomain looks up a location by its vanity hostname.
// Returns (nil, nil) when no location matches.
func (r *LocationRepo) GetByCustomDomain(ctx context.Context, domain string) (*model.Location, error) {
	query := `
		SELECT id, org_id, name, slug, address, timezone, website_url, phone,
			hours_json, day_pass_info, waiver_url, allow_shared_setters,
			custom_domain, progressions_enabled, created_at, updated_at
		FROM locations
		WHERE custom_domain = $1 AND deleted_at IS NULL`

	l := &model.Location{}
	err := r.db.QueryRow(ctx, query, domain).Scan(
		&l.ID, &l.OrgID, &l.Name, &l.Slug, &l.Address, &l.Timezone,
		&l.WebsiteURL, &l.Phone, &l.HoursJSON, &l.DayPassInfo,
		&l.WaiverURL, &l.AllowSharedSetters, &l.CustomDomain, &l.ProgressionsEnabled,
		&l.CreatedAt, &l.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get location by custom domain: %w", err)
	}
	return l, nil
}
