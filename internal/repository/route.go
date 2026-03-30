package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

type RouteRepo struct {
	db *pgxpool.Pool
}

func NewRouteRepo(db *pgxpool.Pool) *RouteRepo {
	return &RouteRepo{db: db}
}

func (r *RouteRepo) Create(ctx context.Context, rt *model.Route) error {
	query := `
		INSERT INTO routes (location_id, wall_id, setter_id, route_type, status,
			grading_system, grade, grade_low, grade_high, circuit_color,
			name, color, description, photo_url, date_set, projected_strip_date, session_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		RETURNING id, avg_rating, rating_count, ascent_count, attempt_count, created_at, updated_at`

	return r.db.QueryRow(ctx, query,
		rt.LocationID, rt.WallID, rt.SetterID, rt.RouteType, rt.Status,
		rt.GradingSystem, rt.Grade, rt.GradeLow, rt.GradeHigh, rt.CircuitColor,
		rt.Name, rt.Color, rt.Description, rt.PhotoURL, rt.DateSet, rt.ProjectedStripDate,
		rt.SessionID,
	).Scan(&rt.ID, &rt.AvgRating, &rt.RatingCount, &rt.AscentCount, &rt.AttemptCount, &rt.CreatedAt, &rt.UpdatedAt)
}

func (r *RouteRepo) GetByID(ctx context.Context, id string) (*model.Route, error) {
	// Single query with LEFT JOIN to load route + tags in one round trip
	query := `
		SELECT r.id, r.location_id, r.wall_id, r.setter_id, r.route_type, r.status,
			r.grading_system, r.grade, r.grade_low, r.grade_high, r.circuit_color,
			r.name, r.color, r.description, r.photo_url, r.date_set, r.projected_strip_date, r.date_stripped,
			r.avg_rating, r.rating_count, r.ascent_count, r.attempt_count, r.created_at, r.updated_at,
			t.id, t.org_id, t.category, t.name, t.color
		FROM routes r
		LEFT JOIN route_tags rt2 ON rt2.route_id = r.id
		LEFT JOIN tags t ON t.id = rt2.tag_id
		WHERE r.id = $1 AND r.deleted_at IS NULL`

	rows, err := r.db.Query(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("get route by id: %w", err)
	}
	defer rows.Close()

	var rt *model.Route
	for rows.Next() {
		var tagID, tagOrgID, tagCategory, tagName *string
		var tagColor *string

		if rt == nil {
			rt = &model.Route{}
		}

		if err := rows.Scan(
			&rt.ID, &rt.LocationID, &rt.WallID, &rt.SetterID, &rt.RouteType, &rt.Status,
			&rt.GradingSystem, &rt.Grade, &rt.GradeLow, &rt.GradeHigh, &rt.CircuitColor,
			&rt.Name, &rt.Color, &rt.Description, &rt.PhotoURL,
			&rt.DateSet, &rt.ProjectedStripDate, &rt.DateStripped,
			&rt.AvgRating, &rt.RatingCount, &rt.AscentCount, &rt.AttemptCount, &rt.CreatedAt, &rt.UpdatedAt,
			&tagID, &tagOrgID, &tagCategory, &tagName, &tagColor,
		); err != nil {
			return nil, fmt.Errorf("scan route: %w", err)
		}

		if tagID != nil {
			rt.Tags = append(rt.Tags, model.Tag{
				ID:       *tagID,
				OrgID:    *tagOrgID,
				Category: *tagCategory,
				Name:     *tagName,
				Color:    tagColor,
			})
		}
	}

	return rt, nil
}

type RouteFilter struct {
	LocationID   string
	WallID       string
	Status       string
	RouteType    string
	Grade        string   // exact grade match
	GradeIn      []string // grade IN (...) filter for grade ranges
	CircuitColor string   // filter by circuit_color (for circuit grade chips)
	SetterID     string
	Limit        int
	Offset       int
}

func (r *RouteRepo) List(ctx context.Context, f RouteFilter) ([]model.Route, int, error) {
	where := []string{"r.location_id = $1", "r.deleted_at IS NULL"}
	args := []interface{}{f.LocationID}
	argN := 2

	if f.WallID != "" {
		where = append(where, fmt.Sprintf("r.wall_id = $%d", argN))
		args = append(args, f.WallID)
		argN++
	}
	if f.Status != "" {
		where = append(where, fmt.Sprintf("r.status = $%d", argN))
		args = append(args, f.Status)
		argN++
	}
	if f.RouteType != "" {
		where = append(where, fmt.Sprintf("r.route_type = $%d", argN))
		args = append(args, f.RouteType)
		argN++
	}
	if f.Grade != "" {
		where = append(where, fmt.Sprintf("r.grade = $%d", argN))
		args = append(args, f.Grade)
		argN++
	}
	if len(f.GradeIn) > 0 {
		placeholders := make([]string, len(f.GradeIn))
		for i, g := range f.GradeIn {
			placeholders[i] = fmt.Sprintf("$%d", argN)
			args = append(args, g)
			argN++
		}
		where = append(where, fmt.Sprintf("r.grade IN (%s)", strings.Join(placeholders, ",")))
	}
	if f.CircuitColor != "" {
		where = append(where, fmt.Sprintf("r.circuit_color = $%d", argN))
		args = append(args, f.CircuitColor)
		argN++
	}
	if f.SetterID != "" {
		where = append(where, fmt.Sprintf("r.setter_id = $%d", argN))
		args = append(args, f.SetterID)
		argN++
	}

	whereClause := strings.Join(where, " AND ")

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM routes r WHERE %s", whereClause)
	var total int
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count routes: %w", err)
	}

	// Fetch routes
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	query := fmt.Sprintf(`
		SELECT r.id, r.location_id, r.wall_id, r.setter_id, r.route_type, r.status,
			r.grading_system, r.grade, r.grade_low, r.grade_high, r.circuit_color,
			r.name, r.color, r.description, r.photo_url,
			r.date_set, r.projected_strip_date, r.date_stripped,
			r.avg_rating, r.rating_count, r.ascent_count, r.attempt_count, r.created_at, r.updated_at
		FROM routes r
		WHERE %s
		ORDER BY r.date_set DESC, r.created_at DESC
		LIMIT $%d OFFSET $%d`,
		whereClause, argN, argN+1)

	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list routes: %w", err)
	}
	defer rows.Close()

	var routes []model.Route
	for rows.Next() {
		var rt model.Route
		if err := rows.Scan(
			&rt.ID, &rt.LocationID, &rt.WallID, &rt.SetterID, &rt.RouteType, &rt.Status,
			&rt.GradingSystem, &rt.Grade, &rt.GradeLow, &rt.GradeHigh, &rt.CircuitColor,
			&rt.Name, &rt.Color, &rt.Description, &rt.PhotoURL,
			&rt.DateSet, &rt.ProjectedStripDate, &rt.DateStripped,
			&rt.AvgRating, &rt.RatingCount, &rt.AscentCount, &rt.AttemptCount, &rt.CreatedAt, &rt.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan route: %w", err)
		}
		routes = append(routes, rt)
	}
	return routes, total, nil
}

// RouteWithDetails is a route joined with wall name and setter display name.
type RouteWithDetails struct {
	model.Route
	WallName   string `json:"wall_name"`
	SetterName string `json:"setter_name"`
}

// ListWithDetails returns routes joined with wall and setter info.
// Used by the web frontend which needs display names alongside route data.
func (r *RouteRepo) ListWithDetails(ctx context.Context, f RouteFilter) ([]RouteWithDetails, int, error) {
	where := []string{"r.location_id = $1", "r.deleted_at IS NULL"}
	args := []interface{}{f.LocationID}
	argN := 2

	if f.WallID != "" {
		where = append(where, fmt.Sprintf("r.wall_id = $%d", argN))
		args = append(args, f.WallID)
		argN++
	}
	if f.Status != "" {
		where = append(where, fmt.Sprintf("r.status = $%d", argN))
		args = append(args, f.Status)
		argN++
	}
	if f.RouteType != "" {
		where = append(where, fmt.Sprintf("r.route_type = $%d", argN))
		args = append(args, f.RouteType)
		argN++
	}
	if f.Grade != "" {
		where = append(where, fmt.Sprintf("r.grade = $%d", argN))
		args = append(args, f.Grade)
		argN++
	}
	if len(f.GradeIn) > 0 {
		placeholders := make([]string, len(f.GradeIn))
		for i, g := range f.GradeIn {
			placeholders[i] = fmt.Sprintf("$%d", argN)
			args = append(args, g)
			argN++
		}
		where = append(where, fmt.Sprintf("r.grade IN (%s)", strings.Join(placeholders, ",")))
	}
	if f.CircuitColor != "" {
		where = append(where, fmt.Sprintf("r.circuit_color = $%d", argN))
		args = append(args, f.CircuitColor)
		argN++
	}
	if f.SetterID != "" {
		where = append(where, fmt.Sprintf("r.setter_id = $%d", argN))
		args = append(args, f.SetterID)
		argN++
	}

	whereClause := strings.Join(where, " AND ")

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM routes r WHERE %s", whereClause)
	var total int
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count routes: %w", err)
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	query := fmt.Sprintf(`
		SELECT r.id, r.location_id, r.wall_id, r.setter_id, r.route_type, r.status,
			r.grading_system, r.grade, r.grade_low, r.grade_high, r.circuit_color,
			r.name, r.color, r.description, r.photo_url,
			r.date_set, r.projected_strip_date, r.date_stripped,
			r.avg_rating, r.rating_count, r.ascent_count, r.attempt_count, r.created_at, r.updated_at,
			COALESCE(w.name, '') as wall_name,
			COALESCE(u.display_name, 'Unknown') as setter_name
		FROM routes r
		LEFT JOIN walls w ON w.id = r.wall_id
		LEFT JOIN users u ON u.id = r.setter_id
		WHERE %s
		ORDER BY r.date_set DESC, r.created_at DESC
		LIMIT $%d OFFSET $%d`,
		whereClause, argN, argN+1)

	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list routes with details: %w", err)
	}
	defer rows.Close()

	var routes []RouteWithDetails
	for rows.Next() {
		var rd RouteWithDetails
		if err := rows.Scan(
			&rd.ID, &rd.LocationID, &rd.WallID, &rd.SetterID, &rd.RouteType, &rd.Status,
			&rd.GradingSystem, &rd.Grade, &rd.GradeLow, &rd.GradeHigh, &rd.CircuitColor,
			&rd.Name, &rd.Color, &rd.Description, &rd.PhotoURL,
			&rd.DateSet, &rd.ProjectedStripDate, &rd.DateStripped,
			&rd.AvgRating, &rd.RatingCount, &rd.AscentCount, &rd.AttemptCount, &rd.CreatedAt, &rd.UpdatedAt,
			&rd.WallName, &rd.SetterName,
		); err != nil {
			return nil, 0, fmt.Errorf("scan route detail: %w", err)
		}
		routes = append(routes, rd)
	}
	return routes, total, nil
}

// ListActiveByLocation returns all active routes for a location in one query,
// joined with wall and setter info. Used by the dashboard to avoid N+1 queries
// (one query per wall). Results are ordered by wall sort order, then date set.
func (r *RouteRepo) ListActiveByLocation(ctx context.Context, locationID string) ([]RouteWithDetails, error) {
	query := `
		SELECT r.id, r.location_id, r.wall_id, r.setter_id, r.route_type, r.status,
			r.grading_system, r.grade, r.grade_low, r.grade_high, r.circuit_color,
			r.name, r.color, r.description, r.photo_url,
			r.date_set, r.projected_strip_date, r.date_stripped,
			r.avg_rating, r.rating_count, r.ascent_count, r.attempt_count, r.created_at, r.updated_at,
			COALESCE(w.name, '') as wall_name,
			COALESCE(u.display_name, 'Unknown') as setter_name
		FROM routes r
		LEFT JOIN walls w ON w.id = r.wall_id
		LEFT JOIN users u ON u.id = r.setter_id
		WHERE r.location_id = $1 AND r.status = 'active' AND r.deleted_at IS NULL
		ORDER BY COALESCE(w.sort_order, 999), w.name, r.date_set DESC, r.created_at DESC`

	rows, err := r.db.Query(ctx, query, locationID)
	if err != nil {
		return nil, fmt.Errorf("list active routes by location: %w", err)
	}
	defer rows.Close()

	var routes []RouteWithDetails
	for rows.Next() {
		var rd RouteWithDetails
		if err := rows.Scan(
			&rd.ID, &rd.LocationID, &rd.WallID, &rd.SetterID, &rd.RouteType, &rd.Status,
			&rd.GradingSystem, &rd.Grade, &rd.GradeLow, &rd.GradeHigh, &rd.CircuitColor,
			&rd.Name, &rd.Color, &rd.Description, &rd.PhotoURL,
			&rd.DateSet, &rd.ProjectedStripDate, &rd.DateStripped,
			&rd.AvgRating, &rd.RatingCount, &rd.AscentCount, &rd.AttemptCount, &rd.CreatedAt, &rd.UpdatedAt,
			&rd.WallName, &rd.SetterName,
		); err != nil {
			return nil, fmt.Errorf("scan active route: %w", err)
		}
		routes = append(routes, rd)
	}
	return routes, nil
}

func (r *RouteRepo) Update(ctx context.Context, rt *model.Route) error {
	query := `
		UPDATE routes
		SET wall_id = $2, setter_id = $3, route_type = $4, status = $5,
			grading_system = $6, grade = $7, grade_low = $8, grade_high = $9, circuit_color = $10,
			name = $11, color = $12, description = $13, photo_url = $14,
			date_set = $15, projected_strip_date = $16, date_stripped = $17
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING updated_at`

	return r.db.QueryRow(ctx, query,
		rt.ID, rt.WallID, rt.SetterID, rt.RouteType, rt.Status,
		rt.GradingSystem, rt.Grade, rt.GradeLow, rt.GradeHigh, rt.CircuitColor,
		rt.Name, rt.Color, rt.Description, rt.PhotoURL,
		rt.DateSet, rt.ProjectedStripDate, rt.DateStripped,
	).Scan(&rt.UpdatedAt)
}

func (r *RouteRepo) UpdateStatus(ctx context.Context, id, status string) error {
	query := `UPDATE routes SET status = $2 WHERE id = $1 AND deleted_at IS NULL`
	_, err := r.db.Exec(ctx, query, id, status)
	if err != nil {
		return fmt.Errorf("update route status: %w", err)
	}
	return nil
}

func (r *RouteRepo) BulkArchive(ctx context.Context, ids []string) (int, error) {
	query := `
		UPDATE routes
		SET status = 'archived', date_stripped = CURRENT_DATE
		WHERE id = ANY($1) AND status != 'archived' AND deleted_at IS NULL`

	tag, err := r.db.Exec(ctx, query, ids)
	if err != nil {
		return 0, fmt.Errorf("bulk archive routes: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func (r *RouteRepo) BulkArchiveByWall(ctx context.Context, wallID string) (int, error) {
	query := `
		UPDATE routes
		SET status = 'archived', date_stripped = CURRENT_DATE
		WHERE wall_id = $1 AND status = 'active' AND deleted_at IS NULL`

	tag, err := r.db.Exec(ctx, query, wallID)
	if err != nil {
		return 0, fmt.Errorf("bulk archive by wall: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// Tag operations

func (r *RouteRepo) GetTags(ctx context.Context, routeID string) ([]model.Tag, error) {
	query := `
		SELECT t.id, t.org_id, t.category, t.name, t.color
		FROM tags t
		JOIN route_tags rt ON rt.tag_id = t.id
		WHERE rt.route_id = $1`

	rows, err := r.db.Query(ctx, query, routeID)
	if err != nil {
		return nil, fmt.Errorf("get route tags: %w", err)
	}
	defer rows.Close()

	var tags []model.Tag
	for rows.Next() {
		var t model.Tag
		if err := rows.Scan(&t.ID, &t.OrgID, &t.Category, &t.Name, &t.Color); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		tags = append(tags, t)
	}
	return tags, nil
}

func (r *RouteRepo) SetTags(ctx context.Context, routeID string, tagIDs []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Remove existing tags
	if _, err := tx.Exec(ctx, "DELETE FROM route_tags WHERE route_id = $1", routeID); err != nil {
		return fmt.Errorf("clear route tags: %w", err)
	}

	// Insert new tags
	for _, tagID := range tagIDs {
		if _, err := tx.Exec(ctx, "INSERT INTO route_tags (route_id, tag_id) VALUES ($1, $2)", routeID, tagID); err != nil {
			return fmt.Errorf("insert route tag: %w", err)
		}
	}

	return tx.Commit(ctx)
}
