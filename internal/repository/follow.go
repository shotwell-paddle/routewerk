package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type FollowRepo struct {
	db *pgxpool.Pool
}

func NewFollowRepo(db *pgxpool.Pool) *FollowRepo {
	return &FollowRepo{db: db}
}

func (r *FollowRepo) Follow(ctx context.Context, followerID, followingID string) error {
	query := `
		INSERT INTO follows (follower_id, following_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING`

	_, err := r.db.Exec(ctx, query, followerID, followingID)
	if err != nil {
		return fmt.Errorf("follow: %w", err)
	}
	return nil
}

func (r *FollowRepo) Unfollow(ctx context.Context, followerID, followingID string) error {
	query := `DELETE FROM follows WHERE follower_id = $1 AND following_id = $2`
	_, err := r.db.Exec(ctx, query, followerID, followingID)
	if err != nil {
		return fmt.Errorf("unfollow: %w", err)
	}
	return nil
}

func (r *FollowRepo) Followers(ctx context.Context, userID string, limit, offset int) ([]FollowUser, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `
		SELECT u.id, u.display_name, u.avatar_url, f.created_at
		FROM follows f
		JOIN users u ON u.id = f.follower_id
		WHERE f.following_id = $1
		ORDER BY f.created_at DESC
		LIMIT $2 OFFSET $3`

	return r.queryFollows(ctx, query, userID, limit, offset)
}

func (r *FollowRepo) Following(ctx context.Context, userID string, limit, offset int) ([]FollowUser, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `
		SELECT u.id, u.display_name, u.avatar_url, f.created_at
		FROM follows f
		JOIN users u ON u.id = f.following_id
		WHERE f.follower_id = $1
		ORDER BY f.created_at DESC
		LIMIT $2 OFFSET $3`

	return r.queryFollows(ctx, query, userID, limit, offset)
}

func (r *FollowRepo) IsFollowing(ctx context.Context, followerID, followingID string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM follows WHERE follower_id = $1 AND following_id = $2)`
	var exists bool
	err := r.db.QueryRow(ctx, query, followerID, followingID).Scan(&exists)
	return exists, err
}

// ActivityFeed returns recent ascents from followed users.
func (r *FollowRepo) ActivityFeed(ctx context.Context, userID string, limit, offset int) ([]FeedItem, error) {
	if limit <= 0 {
		limit = 30
	}

	query := `
		SELECT a.id, a.user_id, a.route_id, a.ascent_type, a.attempts, a.climbed_at,
			u.display_name, u.avatar_url,
			r.grade, r.grading_system, r.route_type, r.color, r.name, r.location_id
		FROM ascents a
		JOIN follows f ON f.following_id = a.user_id
		JOIN users u ON u.id = a.user_id
		JOIN routes r ON r.id = a.route_id
		WHERE f.follower_id = $1
		ORDER BY a.climbed_at DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.db.Query(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("activity feed: %w", err)
	}
	defer rows.Close()

	var items []FeedItem
	for rows.Next() {
		var item FeedItem
		if err := rows.Scan(
			&item.AscentID, &item.UserID, &item.RouteID, &item.AscentType,
			&item.Attempts, &item.ClimbedAt,
			&item.UserDisplayName, &item.UserAvatarURL,
			&item.RouteGrade, &item.RouteGradingSystem, &item.RouteType,
			&item.RouteColor, &item.RouteName, &item.LocationID,
		); err != nil {
			return nil, fmt.Errorf("scan feed item: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *FollowRepo) queryFollows(ctx context.Context, query, userID string, limit, offset int) ([]FollowUser, error) {
	rows, err := r.db.Query(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("query follows: %w", err)
	}
	defer rows.Close()

	var users []FollowUser
	for rows.Next() {
		var u FollowUser
		if err := rows.Scan(&u.ID, &u.DisplayName, &u.AvatarURL, &u.FollowedAt); err != nil {
			return nil, fmt.Errorf("scan follow: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

type FollowUser struct {
	ID          string      `json:"id"`
	DisplayName string      `json:"display_name"`
	AvatarURL   *string     `json:"avatar_url,omitempty"`
	FollowedAt  interface{} `json:"followed_at"`
}

type FeedItem struct {
	AscentID           string      `json:"ascent_id"`
	UserID             string      `json:"user_id"`
	RouteID            string      `json:"route_id"`
	AscentType         string      `json:"ascent_type"`
	Attempts           int         `json:"attempts"`
	ClimbedAt          interface{} `json:"climbed_at"`
	UserDisplayName    string      `json:"user_display_name"`
	UserAvatarURL      *string     `json:"user_avatar_url,omitempty"`
	RouteGrade         string      `json:"route_grade"`
	RouteGradingSystem string      `json:"route_grading_system"`
	RouteType          string      `json:"route_type"`
	RouteColor         string      `json:"route_color"`
	RouteName          *string     `json:"route_name,omitempty"`
	LocationID         string      `json:"location_id"`
}
