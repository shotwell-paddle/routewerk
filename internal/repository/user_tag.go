package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// UserRouteTag represents a single community tag added by a user.
type UserRouteTag struct {
	ID        string    `json:"id"`
	RouteID   string    `json:"route_id"`
	UserID    string    `json:"user_id"`
	TagName   string    `json:"tag_name"`
	CreatedAt time.Time `json:"created_at"`
}

// AggregatedTag is a tag name with the number of users who applied it.
type AggregatedTag struct {
	TagName   string `json:"tag_name"`
	Count     int    `json:"count"`
	UserAdded bool   `json:"user_added"` // true if the current viewer added this tag
}

type UserTagRepo struct {
	db *pgxpool.Pool
}

func NewUserTagRepo(db *pgxpool.Pool) *UserTagRepo {
	return &UserTagRepo{db: db}
}

// Add inserts a user-submitted tag on a route. Duplicate (route, user, tag) is a no-op.
func (r *UserTagRepo) Add(ctx context.Context, routeID, userID, tagName string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO user_route_tags (route_id, user_id, tag_name)
		VALUES ($1, $2, $3)
		ON CONFLICT (route_id, user_id, tag_name) DO NOTHING
	`, routeID, userID, tagName)
	return err
}

// Remove deletes a specific user's tag from a route.
func (r *UserTagRepo) Remove(ctx context.Context, routeID, userID, tagName string) error {
	_, err := r.db.Exec(ctx, `
		DELETE FROM user_route_tags
		WHERE route_id = $1 AND user_id = $2 AND tag_name = $3
	`, routeID, userID, tagName)
	return err
}

// DeleteTagFromRoute removes ALL instances of a tag name from a route (moderator action).
func (r *UserTagRepo) DeleteTagFromRoute(ctx context.Context, routeID, tagName string) error {
	_, err := r.db.Exec(ctx, `
		DELETE FROM user_route_tags
		WHERE route_id = $1 AND tag_name = $2
	`, routeID, tagName)
	return err
}

// ListByRoute returns aggregated tags for a route, ordered by popularity.
// If viewerID is provided, the UserAdded field is populated.
func (r *UserTagRepo) ListByRoute(ctx context.Context, routeID string, viewerID string) ([]AggregatedTag, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			urt.tag_name,
			COUNT(*) AS cnt,
			BOOL_OR(urt.user_id = $2) AS user_added
		FROM user_route_tags urt
		WHERE urt.route_id = $1
		GROUP BY urt.tag_name
		ORDER BY cnt DESC, urt.tag_name
	`, routeID, viewerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []AggregatedTag
	for rows.Next() {
		var t AggregatedTag
		if err := rows.Scan(&t.TagName, &t.Count, &t.UserAdded); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}
