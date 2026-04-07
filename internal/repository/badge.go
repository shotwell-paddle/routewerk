package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

type BadgeRepo struct {
	db *pgxpool.Pool
}

func NewBadgeRepo(db *pgxpool.Pool) *BadgeRepo {
	return &BadgeRepo{db: db}
}

// ============================================================
// Badge CRUD
// ============================================================

func (r *BadgeRepo) Create(ctx context.Context, b *model.Badge) error {
	return r.db.QueryRow(ctx,
		`INSERT INTO badges (location_id, name, description, icon, color)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, created_at`,
		b.LocationID, b.Name, b.Description, b.Icon, b.Color,
	).Scan(&b.ID, &b.CreatedAt)
}

func (r *BadgeRepo) GetByID(ctx context.Context, id string) (*model.Badge, error) {
	b := &model.Badge{}
	err := r.db.QueryRow(ctx,
		`SELECT id, location_id, name, description, icon, color, created_at
		 FROM badges WHERE id = $1`,
		id,
	).Scan(&b.ID, &b.LocationID, &b.Name, &b.Description, &b.Icon, &b.Color, &b.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get badge: %w", err)
	}
	return b, nil
}

func (r *BadgeRepo) Update(ctx context.Context, b *model.Badge) error {
	ct, err := r.db.Exec(ctx,
		`UPDATE badges SET name = $2, description = $3, icon = $4, color = $5
		 WHERE id = $1`,
		b.ID, b.Name, b.Description, b.Icon, b.Color,
	)
	if err != nil {
		return fmt.Errorf("update badge: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("badge not found")
	}
	return nil
}

func (r *BadgeRepo) Delete(ctx context.Context, id string) error {
	ct, err := r.db.Exec(ctx, `DELETE FROM badges WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete badge: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("badge not found")
	}
	return nil
}

func (r *BadgeRepo) ListByLocation(ctx context.Context, locationID string) ([]model.Badge, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, location_id, name, description, icon, color, created_at
		 FROM badges WHERE location_id = $1
		 ORDER BY name`,
		locationID,
	)
	if err != nil {
		return nil, fmt.Errorf("list badges: %w", err)
	}
	defer rows.Close()

	var badges []model.Badge
	for rows.Next() {
		var b model.Badge
		if err := rows.Scan(&b.ID, &b.LocationID, &b.Name, &b.Description, &b.Icon, &b.Color, &b.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan badge: %w", err)
		}
		badges = append(badges, b)
	}
	return badges, rows.Err()
}

// ============================================================
// Climber Badges (awards)
// ============================================================

// AwardBadge grants a badge to a climber. Returns the new ClimberBadge.
func (r *BadgeRepo) AwardBadge(ctx context.Context, userID, badgeID string) (*model.ClimberBadge, error) {
	cb := &model.ClimberBadge{
		UserID:  userID,
		BadgeID: badgeID,
	}
	err := r.db.QueryRow(ctx,
		`INSERT INTO climber_badges (user_id, badge_id)
		 VALUES ($1, $2)
		 ON CONFLICT (user_id, badge_id) DO NOTHING
		 RETURNING id, earned_at`,
		userID, badgeID,
	).Scan(&cb.ID, &cb.EarnedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			// Already awarded — not an error, just fetch the existing one.
			return r.getClimberBadge(ctx, userID, badgeID)
		}
		return nil, fmt.Errorf("award badge: %w", err)
	}
	return cb, nil
}

func (r *BadgeRepo) getClimberBadge(ctx context.Context, userID, badgeID string) (*model.ClimberBadge, error) {
	cb := &model.ClimberBadge{}
	err := r.db.QueryRow(ctx,
		`SELECT id, user_id, badge_id, earned_at
		 FROM climber_badges WHERE user_id = $1 AND badge_id = $2`,
		userID, badgeID,
	).Scan(&cb.ID, &cb.UserID, &cb.BadgeID, &cb.EarnedAt)
	if err != nil {
		return nil, fmt.Errorf("get climber badge: %w", err)
	}
	return cb, nil
}

// HasBadge checks if a climber already has a specific badge.
func (r *BadgeRepo) HasBadge(ctx context.Context, userID, badgeID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM climber_badges WHERE user_id = $1 AND badge_id = $2)`,
		userID, badgeID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check badge: %w", err)
	}
	return exists, nil
}

// ListUserBadges returns all badges earned by a climber, with badge details.
func (r *BadgeRepo) ListUserBadges(ctx context.Context, userID string) ([]model.ClimberBadge, error) {
	rows, err := r.db.Query(ctx,
		`SELECT cb.id, cb.user_id, cb.badge_id, cb.earned_at,
		        b.id, b.location_id, b.name, b.description, b.icon, b.color, b.created_at
		 FROM climber_badges cb
		 JOIN badges b ON b.id = cb.badge_id
		 WHERE cb.user_id = $1
		 ORDER BY cb.earned_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list user badges: %w", err)
	}
	defer rows.Close()

	var results []model.ClimberBadge
	for rows.Next() {
		var cb model.ClimberBadge
		badge := &model.Badge{}
		if err := rows.Scan(
			&cb.ID, &cb.UserID, &cb.BadgeID, &cb.EarnedAt,
			&badge.ID, &badge.LocationID, &badge.Name, &badge.Description,
			&badge.Icon, &badge.Color, &badge.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan user badge: %w", err)
		}
		cb.Badge = badge
		results = append(results, cb)
	}
	return results, rows.Err()
}

// ListUserBadgesForLocation returns badges earned by a climber at a specific location.
func (r *BadgeRepo) ListUserBadgesForLocation(ctx context.Context, userID, locationID string) ([]model.ClimberBadge, error) {
	rows, err := r.db.Query(ctx,
		`SELECT cb.id, cb.user_id, cb.badge_id, cb.earned_at,
		        b.id, b.location_id, b.name, b.description, b.icon, b.color, b.created_at
		 FROM climber_badges cb
		 JOIN badges b ON b.id = cb.badge_id
		 WHERE cb.user_id = $1 AND b.location_id = $2
		 ORDER BY cb.earned_at DESC`,
		userID, locationID,
	)
	if err != nil {
		return nil, fmt.Errorf("list user badges for location: %w", err)
	}
	defer rows.Close()

	var results []model.ClimberBadge
	for rows.Next() {
		var cb model.ClimberBadge
		badge := &model.Badge{}
		if err := rows.Scan(
			&cb.ID, &cb.UserID, &cb.BadgeID, &cb.EarnedAt,
			&badge.ID, &badge.LocationID, &badge.Name, &badge.Description,
			&badge.Icon, &badge.Color, &badge.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan user badge: %w", err)
		}
		cb.Badge = badge
		results = append(results, cb)
	}
	return results, rows.Err()
}

// CountByLocation returns the number of distinct badges earned at a location.
func (r *BadgeRepo) CountByLocation(ctx context.Context, locationID string) (int, error) {
	var count int
	err := r.db.QueryRow(ctx,
		`SELECT COUNT(DISTINCT cb.badge_id)
		 FROM climber_badges cb
		 JOIN badges b ON b.id = cb.badge_id
		 WHERE b.location_id = $1`,
		locationID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count badges: %w", err)
	}
	return count, nil
}
