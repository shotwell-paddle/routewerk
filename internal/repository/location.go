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
		INSERT INTO locations (org_id, name, slug, address, timezone, website_url, phone, hours_json, day_pass_info, waiver_url, allow_shared_setters)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, created_at, updated_at`

	return r.db.QueryRow(ctx, query,
		l.OrgID, l.Name, l.Slug, l.Address, l.Timezone,
		l.WebsiteURL, l.Phone, l.HoursJSON, l.DayPassInfo,
		l.WaiverURL, l.AllowSharedSetters,
	).Scan(&l.ID, &l.CreatedAt, &l.UpdatedAt)
}

func (r *LocationRepo) GetByID(ctx context.Context, id string) (*model.Location, error) {
	query := `
		SELECT id, org_id, name, slug, address, timezone, website_url, phone,
			hours_json, day_pass_info, waiver_url, allow_shared_setters,
			created_at, updated_at
		FROM locations
		WHERE id = $1 AND deleted_at IS NULL`

	l := &model.Location{}
	err := r.db.QueryRow(ctx, query, id).Scan(
		&l.ID, &l.OrgID, &l.Name, &l.Slug, &l.Address, &l.Timezone,
		&l.WebsiteURL, &l.Phone, &l.HoursJSON, &l.DayPassInfo,
		&l.WaiverURL, &l.AllowSharedSetters, &l.CreatedAt, &l.UpdatedAt,
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
			created_at, updated_at
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
			&l.WaiverURL, &l.AllowSharedSetters, &l.CreatedAt, &l.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan location: %w", err)
		}
		locations = append(locations, l)
	}
	return locations, nil
}

func (r *LocationRepo) Update(ctx context.Context, l *model.Location) error {
	query := `
		UPDATE locations
		SET name = $2, slug = $3, address = $4, timezone = $5, website_url = $6,
			phone = $7, hours_json = $8, day_pass_info = $9, waiver_url = $10,
			allow_shared_setters = $11
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING updated_at`

	return r.db.QueryRow(ctx, query,
		l.ID, l.Name, l.Slug, l.Address, l.Timezone,
		l.WebsiteURL, l.Phone, l.HoursJSON, l.DayPassInfo,
		l.WaiverURL, l.AllowSharedSetters,
	).Scan(&l.UpdatedAt)
}
