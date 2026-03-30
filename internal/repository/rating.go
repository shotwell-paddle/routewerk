package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

type RatingRepo struct {
	db *pgxpool.Pool
}

func NewRatingRepo(db *pgxpool.Pool) *RatingRepo {
	return &RatingRepo{db: db}
}

func (r *RatingRepo) Upsert(ctx context.Context, rating *model.RouteRating) error {
	query := `
		INSERT INTO route_ratings (user_id, route_id, rating, comment)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, route_id)
		DO UPDATE SET rating = $3, comment = $4
		RETURNING id, created_at, updated_at`

	err := r.db.QueryRow(ctx, query,
		rating.UserID, rating.RouteID, rating.Rating, rating.Comment,
	).Scan(&rating.ID, &rating.CreatedAt, &rating.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert rating: %w", err)
	}

	// Avg rating update handled by trg_rating_avg trigger (see migration 002)
	return nil
}

func (r *RatingRepo) GetByUserAndRoute(ctx context.Context, userID, routeID string) (*model.RouteRating, error) {
	query := `
		SELECT id, user_id, route_id, rating, comment, created_at, updated_at
		FROM route_ratings
		WHERE user_id = $1 AND route_id = $2`

	rating := &model.RouteRating{}
	err := r.db.QueryRow(ctx, query, userID, routeID).Scan(
		&rating.ID, &rating.UserID, &rating.RouteID,
		&rating.Rating, &rating.Comment, &rating.CreatedAt, &rating.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get rating: %w", err)
	}
	return rating, nil
}

func (r *RatingRepo) ListByRoute(ctx context.Context, routeID string, limit, offset int) ([]RatingWithUser, error) {
	return r.ListByRouteForViewer(ctx, routeID, "", limit, offset)
}

// ListByRouteForViewer lists ratings respecting user privacy settings.
func (r *RatingRepo) ListByRouteForViewer(ctx context.Context, routeID, viewerID string, limit, offset int) ([]RatingWithUser, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT rr.id, rr.user_id, rr.route_id, rr.rating, rr.comment, rr.created_at, rr.updated_at,
			u.display_name, u.avatar_url
		FROM route_ratings rr
		JOIN users u ON u.id = rr.user_id
		WHERE rr.route_id = $1
		  AND (
		    rr.user_id = $4
		    OR COALESCE(u.settings_json->'privacy'->>'show_profile', 'true') = 'true'
		  )
		ORDER BY rr.created_at DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.db.Query(ctx, query, routeID, limit, offset, viewerID)
	if err != nil {
		return nil, fmt.Errorf("list ratings: %w", err)
	}
	defer rows.Close()

	var ratings []RatingWithUser
	for rows.Next() {
		var rr RatingWithUser
		if err := rows.Scan(
			&rr.ID, &rr.UserID, &rr.RouteID, &rr.Rating, &rr.Comment,
			&rr.CreatedAt, &rr.UpdatedAt, &rr.UserDisplayName, &rr.UserAvatarURL,
		); err != nil {
			return nil, fmt.Errorf("scan rating: %w", err)
		}
		ratings = append(ratings, rr)
	}
	return ratings, nil
}

type RatingWithUser struct {
	model.RouteRating
	UserDisplayName string  `json:"user_display_name"`
	UserAvatarURL   *string `json:"user_avatar_url,omitempty"`
}
