package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shotwell-paddle/routewerk/internal/database"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

type RouteRepo struct {
	db *pgxpool.Pool
}

// whereBuilder accumulates WHERE conditions and positional args for pgx.
type whereBuilder struct {
	conds []string
	args  []interface{}
	argN  int
}

func newWhereBuilder(firstArg interface{}) *whereBuilder {
	return &whereBuilder{
		conds: []string{"r.location_id = $1", "r.deleted_at IS NULL"},
		args:  []interface{}{firstArg},
		argN:  2,
	}
}

func (wb *whereBuilder) addEq(col, val string) {
	if val == "" {
		return
	}
	wb.conds = append(wb.conds, fmt.Sprintf("%s = $%d", col, wb.argN))
	wb.args = append(wb.args, val)
	wb.argN++
}

func (wb *whereBuilder) addGte(col, val string) {
	if val == "" {
		return
	}
	wb.conds = append(wb.conds, fmt.Sprintf("%s >= $%d", col, wb.argN))
	wb.args = append(wb.args, val)
	wb.argN++
}

func (wb *whereBuilder) addLte(col, val string) {
	if val == "" {
		return
	}
	wb.conds = append(wb.conds, fmt.Sprintf("%s <= $%d", col, wb.argN))
	wb.args = append(wb.args, val)
	wb.argN++
}

func (wb *whereBuilder) addIn(col string, vals []string) {
	if len(vals) == 0 {
		return
	}
	placeholders := make([]string, len(vals))
	for i, v := range vals {
		placeholders[i] = fmt.Sprintf("$%d", wb.argN)
		wb.args = append(wb.args, v)
		wb.argN++
	}
	wb.conds = append(wb.conds, fmt.Sprintf("%s IN (%s)", col, strings.Join(placeholders, ",")))
}

func (wb *whereBuilder) clause() string {
	return strings.Join(wb.conds, " AND ")
}

// nextArg returns the next positional parameter index (for LIMIT/OFFSET).
func (wb *whereBuilder) nextArg() int {
	return wb.argN
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

// CreateWithTags inserts a route and its tags in a single transaction.
// If tag insertion fails, the route insert is rolled back.
func (r *RouteRepo) CreateWithTags(ctx context.Context, rt *model.Route, tagIDs []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	query := `
		INSERT INTO routes (location_id, wall_id, setter_id, route_type, status,
			grading_system, grade, grade_low, grade_high, circuit_color,
			name, color, description, photo_url, date_set, projected_strip_date, session_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		RETURNING id, avg_rating, rating_count, ascent_count, attempt_count, created_at, updated_at`

	err = tx.QueryRow(ctx, query,
		rt.LocationID, rt.WallID, rt.SetterID, rt.RouteType, rt.Status,
		rt.GradingSystem, rt.Grade, rt.GradeLow, rt.GradeHigh, rt.CircuitColor,
		rt.Name, rt.Color, rt.Description, rt.PhotoURL, rt.DateSet, rt.ProjectedStripDate,
		rt.SessionID,
	).Scan(&rt.ID, &rt.AvgRating, &rt.RatingCount, &rt.AscentCount, &rt.AttemptCount, &rt.CreatedAt, &rt.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create route: %w", err)
	}

	// Bulk-insert tags in a single round-trip via UNNEST instead of a
	// per-tag tx.Exec loop. See perf audit 2026-04-22 #3. Safe under
	// ON CONFLICT DO NOTHING in case the caller passed duplicates.
	if len(tagIDs) > 0 {
		if _, err := tx.Exec(ctx,
			`INSERT INTO route_tags (route_id, tag_id)
			 SELECT $1, UNNEST($2::uuid[])
			 ON CONFLICT DO NOTHING`,
			rt.ID, tagIDs,
		); err != nil {
			return fmt.Errorf("insert route tags: %w", err)
		}
	}

	return tx.Commit(ctx)
}

func (r *RouteRepo) GetByID(ctx context.Context, id string) (*model.Route, error) {
	// Per-query timeout: a single route lookup with the tags LEFT JOIN is a
	// cheap indexed query. Cap at TimeoutFast so one pathologically slow
	// lookup can't drain a whole request budget (callers like the card-batch
	// preview do many of these in series under a shared ctx).
	ctx, cancel := database.QueryTimeout(ctx, database.TimeoutFast)
	defer cancel()

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
	DateFrom     string   // YYYY-MM-DD inclusive lower bound on date_set
	DateTo       string   // YYYY-MM-DD inclusive upper bound on date_set
	Limit        int
	Offset       int
}

// buildWhere constructs the shared WHERE clause for route queries.
func (f RouteFilter) buildWhere() *whereBuilder {
	wb := newWhereBuilder(f.LocationID)
	wb.addEq("r.wall_id", f.WallID)
	wb.addEq("r.status", f.Status)
	wb.addEq("r.route_type", f.RouteType)
	wb.addEq("r.grade", f.Grade)
	wb.addIn("r.grade", f.GradeIn)
	wb.addEq("r.circuit_color", f.CircuitColor)
	wb.addEq("r.setter_id", f.SetterID)
	wb.addGte("r.date_set", f.DateFrom)
	wb.addLte("r.date_set", f.DateTo)
	return wb
}

func (r *RouteRepo) List(ctx context.Context, f RouteFilter) ([]model.Route, int, error) {
	wb := f.buildWhere()

	// Single query: rows + total (via window COUNT). Replaces the prior
	// COUNT + SELECT split, halving round-trips on the listing hot path.
	// See perf audit 2026-04-22 #2.
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	argN := wb.nextArg()
	query := fmt.Sprintf(`
		SELECT r.id, r.location_id, r.wall_id, r.setter_id, r.route_type, r.status,
			r.grading_system, r.grade, r.grade_low, r.grade_high, r.circuit_color,
			r.name, r.color, r.description, r.photo_url,
			r.date_set, r.projected_strip_date, r.date_stripped,
			r.avg_rating, r.rating_count, r.ascent_count, r.attempt_count, r.created_at, r.updated_at,
			COUNT(*) OVER () AS total_count
		FROM routes r
		WHERE %s
		ORDER BY r.date_set DESC, r.created_at DESC
		LIMIT $%d OFFSET $%d`,
		wb.clause(), argN, argN+1)

	args := append(wb.args, limit, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list routes: %w", err)
	}
	defer rows.Close()

	var (
		routes []model.Route
		total  int
	)
	for rows.Next() {
		var rt model.Route
		if err := rows.Scan(
			&rt.ID, &rt.LocationID, &rt.WallID, &rt.SetterID, &rt.RouteType, &rt.Status,
			&rt.GradingSystem, &rt.Grade, &rt.GradeLow, &rt.GradeHigh, &rt.CircuitColor,
			&rt.Name, &rt.Color, &rt.Description, &rt.PhotoURL,
			&rt.DateSet, &rt.ProjectedStripDate, &rt.DateStripped,
			&rt.AvgRating, &rt.RatingCount, &rt.AscentCount, &rt.AttemptCount, &rt.CreatedAt, &rt.UpdatedAt,
			&total,
		); err != nil {
			return nil, 0, fmt.Errorf("scan route: %w", err)
		}
		routes = append(routes, rt)
	}
	// Empty page with filter matching nothing — we still need the total,
	// which is 0 when the window-count row-set is empty.
	return routes, total, rows.Err()
}

// RouteWithDetails is a route joined with wall name and setter display name.
type RouteWithDetails struct {
	model.Route
	WallName   string `json:"wall_name"`
	SetterName string `json:"setter_name"`
}

// ListWithDetails returns routes joined with wall and setter info.
// Used by the web frontend which needs display names alongside route data.
//
// Uses COUNT(*) OVER () to fold the total count into the same scan —
// one round-trip for the listing hot path instead of two. Note: when a
// page lands past the end of the filtered set (OFFSET > total) the query
// returns zero rows and total=0; callers that need the true count to draw
// pagination past the end should detect offset>0 && len(rows)==0 and
// re-query. In practice our UIs don't expose that case.
func (r *RouteRepo) ListWithDetails(ctx context.Context, f RouteFilter) ([]RouteWithDetails, int, error) {
	wb := f.buildWhere()

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	argN := wb.nextArg()
	query := fmt.Sprintf(`
		SELECT r.id, r.location_id, r.wall_id, r.setter_id, r.route_type, r.status,
			r.grading_system, r.grade, r.grade_low, r.grade_high, r.circuit_color,
			r.name, r.color, r.description, r.photo_url,
			r.date_set, r.projected_strip_date, r.date_stripped,
			r.avg_rating, r.rating_count, r.ascent_count, r.attempt_count, r.created_at, r.updated_at,
			COALESCE(w.name, '') as wall_name,
			COALESCE(u.display_name, 'Unknown') as setter_name,
			COUNT(*) OVER () AS total_count
		FROM routes r
		LEFT JOIN walls w ON w.id = r.wall_id
		LEFT JOIN users u ON u.id = r.setter_id
		WHERE %s
		ORDER BY r.date_set DESC, r.created_at DESC
		LIMIT $%d OFFSET $%d`,
		wb.clause(), argN, argN+1)

	args := append(wb.args, limit, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list routes with details: %w", err)
	}
	defer rows.Close()

	var (
		routes []RouteWithDetails
		total  int
	)
	for rows.Next() {
		var rd RouteWithDetails
		if err := rows.Scan(
			&rd.ID, &rd.LocationID, &rd.WallID, &rd.SetterID, &rd.RouteType, &rd.Status,
			&rd.GradingSystem, &rd.Grade, &rd.GradeLow, &rd.GradeHigh, &rd.CircuitColor,
			&rd.Name, &rd.Color, &rd.Description, &rd.PhotoURL,
			&rd.DateSet, &rd.ProjectedStripDate, &rd.DateStripped,
			&rd.AvgRating, &rd.RatingCount, &rd.AscentCount, &rd.AttemptCount, &rd.CreatedAt, &rd.UpdatedAt,
			&rd.WallName, &rd.SetterName,
			&total,
		); err != nil {
			return nil, 0, fmt.Errorf("scan route detail: %w", err)
		}
		routes = append(routes, rd)
	}
	return routes, total, rows.Err()
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
	return routes, rows.Err()
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

// GetTags returns the tags attached to a route, scoped to a specific
// location for tenant isolation. Returns an empty slice if routeID isn't
// owned by locationID. Callers that want the tags for "whatever location
// owns this route" must look the route up first and pass its location_id.
func (r *RouteRepo) GetTags(ctx context.Context, locationID, routeID string) ([]model.Tag, error) {
	query := `
		SELECT t.id, t.org_id, t.category, t.name, t.color
		FROM tags t
		JOIN route_tags rt ON rt.tag_id = t.id
		JOIN routes r ON r.id = rt.route_id
		WHERE rt.route_id = $1
		  AND r.location_id = $2
		  AND r.deleted_at IS NULL`

	rows, err := r.db.Query(ctx, query, routeID, locationID)
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
	return tags, rows.Err()
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

	// Bulk-insert the new tag set in one round-trip via UNNEST — replaces
	// the per-tag tx.Exec loop that could easily turn a 5-tag save into 5
	// wait-on-network queries while holding a tx. See perf audit #3.
	if len(tagIDs) > 0 {
		if _, err := tx.Exec(ctx,
			`INSERT INTO route_tags (route_id, tag_id)
			 SELECT $1, UNNEST($2::uuid[])
			 ON CONFLICT DO NOTHING`,
			routeID, tagIDs,
		); err != nil {
			return fmt.Errorf("insert route tags: %w", err)
		}
	}

	return tx.Commit(ctx)
}

// RouteDistributionBucket is one row of the dashboard distribution
// chart — the count of active routes at a particular grade or circuit
// color. The dashboard renders three groups (boulder grades, route
// grades, circuits) — RouteType + GradingSystem disambiguate them.
//
// CircuitColor is non-nil only when GradingSystem == "circuit"; it
// holds the hex of one route in the bucket so the chart can paint the
// bar with the gym's actual circuit color (palette presets vary).
type RouteDistributionBucket struct {
	RouteType     string  `json:"route_type"`
	GradingSystem string  `json:"grading_system"`
	Grade         string  `json:"grade"`
	CircuitColor  *string `json:"circuit_color,omitempty"`
	Count         int     `json:"count"`
}

// RouteDistribution returns the active-route count per (route_type,
// grading_system, grade) bucket for a location, plus a representative
// hex color for circuit buckets so the chart can render colored bars.
//
// Replaces the old "fetch all 500 active routes, group client-side"
// pattern in the SPA dashboard. At 50 gyms × 200 routes apiece that
// was 200 routes downloaded per dashboard load just to count them;
// this endpoint returns ~30 rows regardless of route count.
func (r *RouteRepo) RouteDistribution(ctx context.Context, locationID string) ([]RouteDistributionBucket, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			route_type,
			grading_system,
			CASE WHEN grading_system = 'circuit' THEN COALESCE(circuit_color, grade) ELSE grade END AS grade,
			-- Representative hex for circuit buckets — MIN() is arbitrary
			-- but stable; all routes in a circuit bucket should share the
			-- same color anyway, so MIN just plucks any one. NULL for
			-- non-circuit rows.
			CASE WHEN grading_system = 'circuit' THEN MIN(color) ELSE NULL END AS circuit_color,
			COUNT(*) AS count
		FROM routes
		WHERE location_id = $1 AND status = 'active' AND deleted_at IS NULL
		GROUP BY route_type, grading_system,
			CASE WHEN grading_system = 'circuit' THEN COALESCE(circuit_color, grade) ELSE grade END
		ORDER BY route_type, grading_system, grade`, locationID)
	if err != nil {
		return nil, fmt.Errorf("route distribution: %w", err)
	}
	defer rows.Close()

	var out []RouteDistributionBucket
	for rows.Next() {
		var b RouteDistributionBucket
		if err := rows.Scan(&b.RouteType, &b.GradingSystem, &b.Grade, &b.CircuitColor, &b.Count); err != nil {
			return nil, fmt.Errorf("scan route distribution: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// ListByIDs returns the routes matching any of the given IDs in a
// single query. Order of the result is NOT guaranteed to match the
// input — callers that need order (e.g. card-batch sheet layout)
// should index the result by ID and walk the input slice.
//
// Replaces the per-id GetByID loop in cardbatch.RenderBatch /
// ValidateRouteIDs which paid one round-trip per route. At 16-card
// batches × 50 gyms × hourly print runs that was ~800 round-trips/hr
// for what's now one. Filters by location_id so cross-tenant lookups
// silently return nothing instead of returning the wrong gym's data.
func (r *RouteRepo) ListByIDs(ctx context.Context, locationID string, ids []string) ([]model.Route, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, location_id, wall_id, setter_id, route_type, status,
			grading_system, grade, grade_low, grade_high, circuit_color,
			name, color, description, photo_url, date_set,
			projected_strip_date, date_stripped, avg_rating, rating_count,
			ascent_count, attempt_count, session_id, created_at, updated_at,
			deleted_at
		FROM routes
		WHERE location_id = $1 AND id = ANY($2::uuid[]) AND deleted_at IS NULL`,
		locationID, ids,
	)
	if err != nil {
		return nil, fmt.Errorf("list routes by ids: %w", err)
	}
	defer rows.Close()

	var out []model.Route
	for rows.Next() {
		var rt model.Route
		if err := rows.Scan(
			&rt.ID, &rt.LocationID, &rt.WallID, &rt.SetterID, &rt.RouteType, &rt.Status,
			&rt.GradingSystem, &rt.Grade, &rt.GradeLow, &rt.GradeHigh, &rt.CircuitColor,
			&rt.Name, &rt.Color, &rt.Description, &rt.PhotoURL, &rt.DateSet,
			&rt.ProjectedStripDate, &rt.DateStripped, &rt.AvgRating, &rt.RatingCount,
			&rt.AscentCount, &rt.AttemptCount, &rt.SessionID, &rt.CreatedAt, &rt.UpdatedAt,
			&rt.DeletedAt,
		); err != nil {
			return nil, fmt.Errorf("scan route: %w", err)
		}
		out = append(out, rt)
	}
	return out, rows.Err()
}
