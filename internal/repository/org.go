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

// Count returns the total number of active organizations.
func (r *OrgRepo) Count(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM organizations WHERE deleted_at IS NULL`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count orgs: %w", err)
	}
	return count, nil
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

// EnsureOrgScopedMembership makes sure the user has an org-wide membership
// (location_id IS NULL) at the given role or higher. If they only have
// location-scoped memberships, this upgrades the highest one to org-scoped.
// This is needed so org_admins can see all locations in the gym switcher.
func (r *OrgRepo) EnsureOrgScopedMembership(ctx context.Context, userID, orgID, role string) error {
	// Check if an org-scoped membership already exists
	var exists bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM user_memberships
			WHERE user_id = $1 AND org_id = $2 AND location_id IS NULL AND deleted_at IS NULL
		)`, userID, orgID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check org membership: %w", err)
	}
	if exists {
		return nil
	}

	// No org-scoped membership — upgrade the user's location-scoped one to org-scoped
	_, err = r.db.Exec(ctx, `
		UPDATE user_memberships
		SET location_id = NULL, role = $3, updated_at = NOW()
		WHERE id = (
			SELECT id FROM user_memberships
			WHERE user_id = $1 AND org_id = $2 AND deleted_at IS NULL
			ORDER BY CASE role
				WHEN 'org_admin' THEN 1
				WHEN 'gym_manager' THEN 2
				WHEN 'head_setter' THEN 3
				WHEN 'setter' THEN 4
				ELSE 5
			END
			LIMIT 1
		)`, userID, orgID, role)
	if err != nil {
		return fmt.Errorf("upgrade to org membership: %w", err)
	}
	return nil
}
