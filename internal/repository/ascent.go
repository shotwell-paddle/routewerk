package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

type AscentRepo struct {
	db *pgxpool.Pool
}

func NewAscentRepo(db *pgxpool.Pool) *AscentRepo {
	return &AscentRepo{db: db}
}

func (r *AscentRepo) Create(ctx context.Context, a *model.Ascent) error {
	query := `
		INSERT INTO ascents (user_id, route_id, ascent_type, attempts, notes, climbed_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at`

	err := r.db.QueryRow(ctx, query,
		a.UserID, a.RouteID, a.AscentType, a.Attempts, a.Notes, a.ClimbedAt,
	).Scan(&a.ID, &a.CreatedAt)
	if err != nil {
		return fmt.Errorf("create ascent: %w", err)
	}

	// Counter updates handled by trg_ascent_insert_counts trigger (see migration 002)
	return nil
}

func (r *AscentRepo) ListByUser(ctx context.Context, userID string, limit, offset int) ([]AscentWithRoute, int, error) {
	if limit <= 0 {
		limit = 50
	}

	countQuery := `SELECT COUNT(*) FROM ascents WHERE user_id = $1`
	var total int
	if err := r.db.QueryRow(ctx, countQuery, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count ascents: %w", err)
	}

	query := `
		SELECT a.id, a.user_id, a.route_id, a.ascent_type, a.attempts, a.notes, a.climbed_at, a.created_at,
			r.grade, r.grading_system, r.route_type, r.color, r.name, r.wall_id
		FROM ascents a
		JOIN routes r ON r.id = a.route_id
		WHERE a.user_id = $1
		ORDER BY a.climbed_at DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.db.Query(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list ascents: %w", err)
	}
	defer rows.Close()

	var ascents []AscentWithRoute
	for rows.Next() {
		var a AscentWithRoute
		if err := rows.Scan(
			&a.ID, &a.UserID, &a.RouteID, &a.AscentType, &a.Attempts, &a.Notes, &a.ClimbedAt, &a.CreatedAt,
			&a.RouteGrade, &a.RouteGradingSystem, &a.RouteType, &a.RouteColor, &a.RouteName, &a.WallID,
		); err != nil {
			return nil, 0, fmt.Errorf("scan ascent: %w", err)
		}
		ascents = append(ascents, a)
	}
	return ascents, total, nil
}

func (r *AscentRepo) ListByRoute(ctx context.Context, routeID string, limit, offset int) ([]AscentWithUser, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT a.id, a.user_id, a.route_id, a.ascent_type, a.attempts, a.notes, a.climbed_at, a.created_at,
			u.display_name, u.avatar_url
		FROM ascents a
		JOIN users u ON u.id = a.user_id
		WHERE a.route_id = $1
		ORDER BY a.climbed_at DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.db.Query(ctx, query, routeID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list route ascents: %w", err)
	}
	defer rows.Close()

	var ascents []AscentWithUser
	for rows.Next() {
		var a AscentWithUser
		if err := rows.Scan(
			&a.ID, &a.UserID, &a.RouteID, &a.AscentType, &a.Attempts, &a.Notes, &a.ClimbedAt, &a.CreatedAt,
			&a.UserDisplayName, &a.UserAvatarURL,
		); err != nil {
			return nil, fmt.Errorf("scan ascent: %w", err)
		}
		ascents = append(ascents, a)
	}
	return ascents, nil
}

// UserStats returns grade pyramid and summary stats for a climber.
func (r *AscentRepo) UserStats(ctx context.Context, userID string) (*UserClimbingStats, error) {
	stats := &UserClimbingStats{}

	// Total sends / attempts
	summaryQuery := `
		SELECT
			COUNT(*) FILTER (WHERE ascent_type IN ('send', 'flash')) as total_sends,
			COUNT(*) FILTER (WHERE ascent_type = 'flash') as total_flashes,
			COUNT(*) as total_logged,
			COUNT(DISTINCT route_id) as unique_routes
		FROM ascents
		WHERE user_id = $1`

	if err := r.db.QueryRow(ctx, summaryQuery, userID).Scan(
		&stats.TotalSends, &stats.TotalFlashes, &stats.TotalLogged, &stats.UniqueRoutes,
	); err != nil {
		return nil, fmt.Errorf("user stats summary: %w", err)
	}

	// Grade pyramid: count of sends per grade
	pyramidQuery := `
		SELECT r.grading_system, r.grade, COUNT(*) as count
		FROM ascents a
		JOIN routes r ON r.id = a.route_id
		WHERE a.user_id = $1 AND a.ascent_type IN ('send', 'flash')
		GROUP BY r.grading_system, r.grade
		ORDER BY r.grading_system, r.grade`

	rows, err := r.db.Query(ctx, pyramidQuery, userID)
	if err != nil {
		return nil, fmt.Errorf("grade pyramid: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var entry GradePyramidEntry
		if err := rows.Scan(&entry.GradingSystem, &entry.Grade, &entry.Count); err != nil {
			return nil, fmt.Errorf("scan pyramid: %w", err)
		}
		stats.GradePyramid = append(stats.GradePyramid, entry)
	}

	return stats, nil
}

// Joined types for richer list responses

type AscentWithRoute struct {
	model.Ascent
	RouteGrade         string  `json:"route_grade"`
	RouteGradingSystem string  `json:"route_grading_system"`
	RouteType          string  `json:"route_type"`
	RouteColor         string  `json:"route_color"`
	RouteName          *string `json:"route_name,omitempty"`
	WallID             string  `json:"wall_id"`
}

type AscentWithUser struct {
	model.Ascent
	UserDisplayName string  `json:"user_display_name"`
	UserAvatarURL   *string `json:"user_avatar_url,omitempty"`
}

type UserClimbingStats struct {
	TotalSends   int                 `json:"total_sends"`
	TotalFlashes int                 `json:"total_flashes"`
	TotalLogged  int                 `json:"total_logged"`
	UniqueRoutes int                 `json:"unique_routes"`
	GradePyramid []GradePyramidEntry `json:"grade_pyramid"`
}

type GradePyramidEntry struct {
	GradingSystem string `json:"grading_system"`
	Grade         string `json:"grade"`
	Count         int    `json:"count"`
}
