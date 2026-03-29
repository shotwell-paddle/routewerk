package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

type PartnerRepo struct {
	db *pgxpool.Pool
}

func NewPartnerRepo(db *pgxpool.Pool) *PartnerRepo {
	return &PartnerRepo{db: db}
}

func (r *PartnerRepo) Upsert(ctx context.Context, p *model.PartnerProfile) error {
	query := `
		INSERT INTO partner_profiles (user_id, location_id, looking_for, climbing_types, grade_range, availability, bio, active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (user_id)
		DO UPDATE SET location_id = $2, looking_for = $3, climbing_types = $4,
			grade_range = $5, availability = $6, bio = $7, active = $8
		RETURNING id, updated_at`

	return r.db.QueryRow(ctx, query,
		p.UserID, p.LocationID, p.LookingFor, p.ClimbingTypes,
		p.GradeRange, p.Availability, p.Bio, p.Active,
	).Scan(&p.ID, &p.UpdatedAt)
}

func (r *PartnerRepo) GetByUser(ctx context.Context, userID string) (*model.PartnerProfile, error) {
	query := `
		SELECT id, user_id, location_id, looking_for, climbing_types, grade_range, availability, bio, active, updated_at
		FROM partner_profiles
		WHERE user_id = $1`

	p := &model.PartnerProfile{}
	err := r.db.QueryRow(ctx, query, userID).Scan(
		&p.ID, &p.UserID, &p.LocationID, &p.LookingFor, &p.ClimbingTypes,
		&p.GradeRange, &p.Availability, &p.Bio, &p.Active, &p.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get partner profile: %w", err)
	}
	return p, nil
}

func (r *PartnerRepo) Search(ctx context.Context, locationID string) ([]PartnerWithUser, error) {
	query := `
		SELECT pp.id, pp.user_id, pp.location_id, pp.looking_for, pp.climbing_types,
			pp.grade_range, pp.bio, pp.updated_at,
			u.display_name, u.avatar_url
		FROM partner_profiles pp
		JOIN users u ON u.id = pp.user_id
		WHERE pp.location_id = $1 AND pp.active = true
		ORDER BY pp.updated_at DESC`

	rows, err := r.db.Query(ctx, query, locationID)
	if err != nil {
		return nil, fmt.Errorf("search partners: %w", err)
	}
	defer rows.Close()

	var partners []PartnerWithUser
	for rows.Next() {
		var p PartnerWithUser
		if err := rows.Scan(
			&p.ID, &p.UserID, &p.LocationID, &p.LookingFor, &p.ClimbingTypes,
			&p.GradeRange, &p.Bio, &p.UpdatedAt,
			&p.UserDisplayName, &p.UserAvatarURL,
		); err != nil {
			return nil, fmt.Errorf("scan partner: %w", err)
		}
		partners = append(partners, p)
	}
	return partners, nil
}

type PartnerWithUser struct {
	ID              string      `json:"id"`
	UserID          string      `json:"user_id"`
	LocationID      string      `json:"location_id"`
	LookingFor      []string    `json:"looking_for"`
	ClimbingTypes   []string    `json:"climbing_types"`
	GradeRange      *string     `json:"grade_range,omitempty"`
	Bio             *string     `json:"bio,omitempty"`
	UpdatedAt       interface{} `json:"updated_at"`
	UserDisplayName string      `json:"user_display_name"`
	UserAvatarURL   *string     `json:"user_avatar_url,omitempty"`
}
