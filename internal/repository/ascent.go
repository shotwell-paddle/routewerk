package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
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

// GetByID returns a single ascent by its ID.
func (r *AscentRepo) GetByID(ctx context.Context, id string) (*model.Ascent, error) {
	query := `
		SELECT id, user_id, route_id, ascent_type, attempts, notes, climbed_at, created_at
		FROM ascents
		WHERE id = $1`

	var a model.Ascent
	err := r.db.QueryRow(ctx, query, id).Scan(
		&a.ID, &a.UserID, &a.RouteID, &a.AscentType, &a.Attempts, &a.Notes, &a.ClimbedAt, &a.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get ascent: %w", err)
	}
	return &a, nil
}

// Update modifies an existing ascent's type, attempts, and notes.
func (r *AscentRepo) Update(ctx context.Context, a *model.Ascent) error {
	query := `
		UPDATE ascents
		SET ascent_type = $2, attempts = $3, notes = $4
		WHERE id = $1 AND user_id = $5`

	tag, err := r.db.Exec(ctx, query, a.ID, a.AscentType, a.Attempts, a.Notes, a.UserID)
	if err != nil {
		return fmt.Errorf("update ascent: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("ascent not found or not owned by user")
	}
	return nil
}

// Delete removes an ascent. Only the owning user can delete.
func (r *AscentRepo) Delete(ctx context.Context, ascentID, userID string) error {
	query := `DELETE FROM ascents WHERE id = $1 AND user_id = $2`

	tag, err := r.db.Exec(ctx, query, ascentID, userID)
	if err != nil {
		return fmt.Errorf("delete ascent: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("ascent not found or not owned by user")
	}
	return nil
}

// TickFilter controls filtering and sorting for a user's tick list.
type TickFilter struct {
	RouteType  string // "boulder" or "route" — empty means all
	AscentType string // "send", "flash", "attempt", "project" — empty means all
	Sort       string // "date", "grade" — default "date"
}

func (r *AscentRepo) ListByUser(ctx context.Context, userID string, limit, offset int) ([]AscentWithRoute, int, error) {
	return r.ListByUserFiltered(ctx, userID, TickFilter{}, limit, offset)
}

func (r *AscentRepo) ListByUserFiltered(ctx context.Context, userID string, f TickFilter, limit, offset int) ([]AscentWithRoute, int, error) {
	if limit <= 0 {
		limit = 50
	}

	where := []string{"a.user_id = $1"}
	args := []interface{}{userID}
	argN := 2

	if f.RouteType != "" {
		where = append(where, fmt.Sprintf("r.route_type = $%d", argN))
		args = append(args, f.RouteType)
		argN++
	}
	if f.AscentType != "" {
		where = append(where, fmt.Sprintf("a.ascent_type = $%d", argN))
		args = append(args, f.AscentType)
		argN++
	}

	whereClause := strings.Join(where, " AND ")

	// Determine sort order
	orderBy := "a.climbed_at DESC"
	if f.Sort == "grade" {
		// Sort by grade descending, then by date. V-scale sorts lexically after V prefix.
		orderBy = "r.grade DESC, a.climbed_at DESC"
	}

	// Rows-only query. The previous COUNT(*) OVER () window appended a
	// "total" cell to every row, which forced PG to materialize every
	// matching row before LIMIT could trim — O(rows) per request even
	// when the user only wanted the first 25. The handler now pulls
	// the total directly from users.total_logged (denormalized; see
	// migration 000037), so this query just fetches the page.
	//
	// Caveat: filters on route_type / ascent_type narrow the row set
	// without narrowing the counter. Callers that filter need to pull
	// their own COUNT if they want an accurate "X of Y filtered" — at
	// SPA scale today no caller does (the profile lists all ticks
	// chronologically and the count panel uses the unfiltered total).
	query := fmt.Sprintf(`
		SELECT a.id, a.user_id, a.route_id, a.ascent_type, a.attempts, a.notes, a.climbed_at, a.created_at,
			r.grade, r.grading_system, r.route_type, r.color, r.name, r.wall_id
		FROM ascents a
		JOIN routes r ON r.id = a.route_id
		WHERE %s
		ORDER BY %s
		LIMIT $%d OFFSET $%d`,
		whereClause, orderBy, argN, argN+1,
	)
	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, query, args...)
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

	// Cheap counter read — sub-ms, no scan. Returned alongside the
	// page so the handler can hand `{ascents, total}` back to the SPA
	// for pagination meta without any extra round-trip from the caller.
	var total int
	if err := r.db.QueryRow(ctx, `SELECT total_logged FROM users WHERE id = $1`, userID).Scan(&total); err != nil {
		// If the counter read fails (extremely unlikely — same conn,
		// same tx semantics), fall back to len(ascents) so the page
		// still renders. The "Showing X of Y" line will be wrong but
		// the user data is correct.
		total = len(ascents)
	}

	return ascents, total, rows.Err()
}

// HasPriorAscents returns true if the user has any existing ascent records on this route.
func (r *AscentRepo) HasPriorAscents(ctx context.Context, userID, routeID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM ascents WHERE user_id = $1 AND route_id = $2)`,
		userID, routeID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("has prior ascents: %w", err)
	}
	return exists, nil
}

// HasCompletedRoute returns true if the user has a "send" or "flash" on this route.
func (r *AscentRepo) HasCompletedRoute(ctx context.Context, userID, routeID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM ascents WHERE user_id = $1 AND route_id = $2 AND ascent_type IN ('send', 'flash'))`,
		userID, routeID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("has completed route: %w", err)
	}
	return exists, nil
}

// RouteAscentStatus returns both "has any ascent" and "has completed (send/flash)" in a single query.
func (r *AscentRepo) RouteAscentStatus(ctx context.Context, userID, routeID string) (hasAny bool, hasCompleted bool, err error) {
	err = r.db.QueryRow(ctx,
		`SELECT
			EXISTS(SELECT 1 FROM ascents WHERE user_id = $1 AND route_id = $2),
			EXISTS(SELECT 1 FROM ascents WHERE user_id = $1 AND route_id = $2 AND ascent_type IN ('send', 'flash'))`,
		userID, routeID,
	).Scan(&hasAny, &hasCompleted)
	if err != nil {
		return false, false, fmt.Errorf("route ascent status: %w", err)
	}
	return hasAny, hasCompleted, nil
}

func (r *AscentRepo) ListByRoute(ctx context.Context, routeID string, limit, offset int) ([]AscentWithUser, error) {
	return r.ListByRouteForViewer(ctx, routeID, "", limit, offset)
}

// ListByRouteForViewer lists ascents for a route, respecting user privacy settings.
// The viewer always sees their own ascents regardless of privacy. Other users'
// ascents are hidden if they have set show_profile = false.
func (r *AscentRepo) ListByRouteForViewer(ctx context.Context, routeID, viewerID string, limit, offset int) ([]AscentWithUser, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT a.id, a.user_id, a.route_id, a.ascent_type, a.attempts, a.notes, a.climbed_at, a.created_at,
			u.display_name, u.avatar_url
		FROM ascents a
		JOIN users u ON u.id = a.user_id
		WHERE a.route_id = $1
		  AND (
		    a.user_id = $4
		    OR COALESCE(u.settings_json->'privacy'->>'show_profile', 'true') = 'true'
		  )
		ORDER BY a.climbed_at DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.db.Query(ctx, query, routeID, limit, offset, viewerID)
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
	return ascents, rows.Err()
}

// UserStats returns the summary counters (total_sends / total_flashes /
// total_logged / unique_routes) for a climber.
//
// Reads from the denormalized counters on the users row (maintained by
// trigger; see migration 000037). Sub-millisecond regardless of how
// many ascents the user has logged. The grade pyramid moved to its
// own UserGradePyramid method since it's the structurally-expensive
// part — the SPA lazy-loads it on the profile page so the summary
// stat panel renders immediately.
func (r *AscentRepo) UserStats(ctx context.Context, userID string) (*UserClimbingStats, error) {
	stats := &UserClimbingStats{}
	if err := r.db.QueryRow(ctx, `
		SELECT total_sends, total_flashes, total_logged, unique_routes
		FROM users
		WHERE id = $1 AND deleted_at IS NULL`, userID,
	).Scan(&stats.TotalSends, &stats.TotalFlashes, &stats.TotalLogged, &stats.UniqueRoutes); err != nil {
		return nil, fmt.Errorf("user stats summary: %w", err)
	}
	return stats, nil
}

// UserGradePyramid returns the count of (send + flash) ascents grouped
// by grade. This is the formerly-bundled-into-UserStats query that
// scans every send/flash row for the user — kept as its own method so
// the cheap summary read (UserStats above) doesn't pay for it. The SPA
// fetches this lazily via /api/v1/me/grade-pyramid.
func (r *AscentRepo) UserGradePyramid(ctx context.Context, userID string) ([]GradePyramidEntry, error) {
	rows, err := r.db.Query(ctx, `
		SELECT r.grading_system, r.grade, COUNT(*) as count
		FROM ascents a
		JOIN routes r ON r.id = a.route_id
		WHERE a.user_id = $1 AND a.ascent_type IN ('send', 'flash')
		GROUP BY r.grading_system, r.grade
		ORDER BY r.grading_system, r.grade`, userID)
	if err != nil {
		return nil, fmt.Errorf("grade pyramid: %w", err)
	}
	defer rows.Close()

	var out []GradePyramidEntry
	for rows.Next() {
		var entry GradePyramidEntry
		if err := rows.Scan(&entry.GradingSystem, &entry.Grade, &entry.Count); err != nil {
			return nil, fmt.Errorf("scan pyramid: %w", err)
		}
		out = append(out, entry)
	}
	return out, rows.Err()
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
	TotalSends   int `json:"total_sends"`
	TotalFlashes int `json:"total_flashes"`
	TotalLogged  int `json:"total_logged"`
	UniqueRoutes int `json:"unique_routes"`
}

type GradePyramidEntry struct {
	GradingSystem string `json:"grading_system"`
	Grade         string `json:"grade"`
	Count         int    `json:"count"`
}
