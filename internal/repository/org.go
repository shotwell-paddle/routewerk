package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

type OrgRepo struct {
	db *pgxpool.Pool
}

func NewOrgRepo(db *pgxpool.Pool) *OrgRepo {
	return &OrgRepo{db: db}
}

func (r *OrgRepo) Create(ctx context.Context, o *model.Organization) error {
	query := `
		INSERT INTO organizations (name, slug, logo_url)
		VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at`

	return r.db.QueryRow(ctx, query,
		o.Name, o.Slug, o.LogoURL,
	).Scan(&o.ID, &o.CreatedAt, &o.UpdatedAt)
}

func (r *OrgRepo) GetByID(ctx context.Context, id string) (*model.Organization, error) {
	query := `
		SELECT id, name, slug, logo_url, created_at, updated_at
		FROM organizations
		WHERE id = $1 AND deleted_at IS NULL`

	o := &model.Organization{}
	err := r.db.QueryRow(ctx, query, id).Scan(
		&o.ID, &o.Name, &o.Slug, &o.LogoURL, &o.CreatedAt, &o.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get org by id: %w", err)
	}
	return o, nil
}

func (r *OrgRepo) GetBySlug(ctx context.Context, slug string) (*model.Organization, error) {
	query := `
		SELECT id, name, slug, logo_url, created_at, updated_at
		FROM organizations
		WHERE slug = $1 AND deleted_at IS NULL`

	o := &model.Organization{}
	err := r.db.QueryRow(ctx, query, slug).Scan(
		&o.ID, &o.Name, &o.Slug, &o.LogoURL, &o.CreatedAt, &o.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get org by slug: %w", err)
	}
	return o, nil
}

func (r *OrgRepo) ListByUser(ctx context.Context, userID string) ([]model.Organization, error) {
	query := `
		SELECT DISTINCT o.id, o.name, o.slug, o.logo_url, o.created_at, o.updated_at
		FROM organizations o
		JOIN user_memberships um ON um.org_id = o.id
		WHERE um.user_id = $1 AND um.deleted_at IS NULL AND o.deleted_at IS NULL
		ORDER BY o.name`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list orgs by user: %w", err)
	}
	defer rows.Close()

	var orgs []model.Organization
	for rows.Next() {
		var o model.Organization
		if err := rows.Scan(
			&o.ID, &o.Name, &o.Slug, &o.LogoURL, &o.CreatedAt, &o.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan org: %w", err)
		}
		orgs = append(orgs, o)
	}
	return orgs, nil
}

func (r *OrgRepo) Update(ctx context.Context, o *model.Organization) error {
	query := `
		UPDATE organizations
		SET name = $2, slug = $3, logo_url = $4
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING updated_at`

	return r.db.QueryRow(ctx, query,
		o.ID, o.Name, o.Slug, o.LogoURL,
	).Scan(&o.UpdatedAt)
}

// AddMember creates a membership for a user in the org.
func (r *OrgRepo) AddMember(ctx context.Context, m *model.UserMembership) error {
	query := `
		INSERT INTO user_memberships (user_id, org_id, location_id, role, specialties)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at`

	return r.db.QueryRow(ctx, query,
		m.UserID, m.OrgID, m.LocationID, m.Role, m.Specialties,
	).Scan(&m.ID, &m.CreatedAt, &m.UpdatedAt)
}
