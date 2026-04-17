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

// ListByRoute lists ratings for a route scoped to the given location. If
// the route doesn't belong to locationID the result is empty — callers that
// want the route's true owning location must look it up first. This guard
// prevents cross-tenant probing via a stolen routeID.
func (r *RatingRepo) ListByRoute(ctx context.Context, routeID, locationID string, limit, offset int) ([]RatingWithUser, error) {
	return r.ListByRouteForViewer(ctx, routeID, locationID, "", limit, offset)
}

// ListByRouteForViewer lists ratings respecting user privacy settings and
// scoped to a specific location for tenant isolation.
func (r *RatingRepo) ListByRouteForViewer(ctx context.Context, routeID, locationID, viewerID string, limit, offset int) ([]RatingWithUser, error) {
	if limit <= 0 {
		limit = 50
	}

	// JOIN routes and filter by location_id so cross-tenant routeID probes
	// return zero rows regardless of what the caller passes for routeID.
	query := `
		SELECT rr.id, rr.user_id, rr.route_id, rr.rating, rr.comment, rr.created_at, rr.updated_at,
			u.display_name, u.avatar_url
		FROM route_ratings rr
		JOIN users u ON u.id = rr.user_id
		JOIN routes r ON r.id = rr.route_id
		WHERE rr.route_id = $1
		  AND r.location_id = $2
		  AND r.deleted_at IS NULL
		  AND (
		    rr.user_id = $5
		    OR COALESCE(u.settings_json->'privacy'->>'show_profile', 'true') = 'true'
		  )
		ORDER BY rr.created_at DESC
		LIMIT $3 OFFSET $4`

	rows, err := r.db.Query(ctx, query, routeID, locationID, limit, offset, viewerID)
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
	return ratings, rows.Err()
}

type RatingWithUser struct {
	model.RouteRating
	UserDisplayName string  `json:"user_display_name"`
	UserAvatarURL   *string `json:"user_avatar_url,omitempty"`
}
