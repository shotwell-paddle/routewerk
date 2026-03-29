package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
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
			name, color, description, photo_url, date_set, projected_strip_date)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		RETURNING id, avg_rating, ascent_count, attempt_count, created_at, updated_at`

	return r.db.QueryRow(ctx, query,
		rt.LocationID, rt.WallID, rt.SetterID, rt.RouteType, rt.Status,
		rt.GradingSystem, rt.Grade, rt.GradeLow, rt.GradeHigh, rt.CircuitColor,
		rt.Name, rt.Color, rt.Description, rt.PhotoURL, rt.DateSet, rt.ProjectedStripDate,
	).Scan(&rt.ID, &rt.AvgRating, &rt.AscentCount, &rt.AttemptCount, &rt.CreatedAt, &rt.UpdatedAt)
}

func (r *RouteRepo) GetByID(ctx context.Context, id string) (*model.Route, error) {
	query := `
		SELECT id, location_id, wall_id, setter_id, route_type, status,
			grading_system, grade, grade_low, grade_high, circuit_color,
			name, color, description, photo_url, date_set, projected_strip_date, date_stripped,
			avg_rating, ascent_count, attempt_count, created_at, updated_at
		FROM routes
		WHERE id = $1 AND deleted_at IS NULL`

	rt := &model.Route{}
	err := r.db.QueryRow(ctx, query, id).Scan(
		&rt.ID, &rt.LocationID, &rt.WallID, &rt.SetterID, &rt.RouteType, &rt.Status,
		&rt.GradingSystem, &rt.Grade, &rt.GradeLow, &rt.GradeHigh, &rt.CircuitColor,
		&rt.Name, &rt.Color, &rt.Description, &rt.PhotoURL,
		&rt.DateSet, &rt.ProjectedStripDate, &rt.DateStripped,
		&rt.AvgRating, &rt.AscentCount, &rt.AttemptCount, &rt.CreatedAt, &rt.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get route by id: %w", err)
	}

	// Load tags
	tags, err := r.GetTags(ctx, rt.ID)
	if err != nil {
		return nil, err
	}
	rt.Tags = tags

	return rt, nil
}

type RouteFilter struct {
	LocationID string
	WallID     string
	Status     string
	RouteType  string
	Grade      string
	SetterID   string
	Limit      int
	Offset     int
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
			r.avg_rating, r.ascent_count, r.attempt_count, r.created_at, r.updated_at
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
			&rt.AvgRating, &rt.AscentCount, &rt.AttemptCount, &rt.CreatedAt, &rt.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan route: %w", err)
		}
		routes = append(routes, rt)
	}
	return routes, total, nil
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
		WHERE wall_id = $1 AND status != 'archived' AND deleted_at IS NULL`

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
