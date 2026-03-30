package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type AnalyticsRepo struct {
	db *pgxpool.Pool
}

func NewAnalyticsRepo(db *pgxpool.Pool) *AnalyticsRepo {
	return &AnalyticsRepo{db: db}
}

// LocationDashboardStats returns the summary numbers for the setter dashboard.
func (r *AnalyticsRepo) LocationDashboardStats(ctx context.Context, locationID string) (*LocationDashboard, error) {
	d := &LocationDashboard{}

	// Route summary
	routeQuery := `
		SELECT
			COUNT(*) FILTER (WHERE status = 'active') as active_routes,
			COUNT(*) FILTER (WHERE status = 'active' AND projected_strip_date IS NOT NULL AND projected_strip_date <= CURRENT_DATE) as due_for_strip,
			COALESCE(AVG(avg_rating) FILTER (WHERE status = 'active' AND rating_count > 0), 0) as avg_rating,
			COUNT(*) FILTER (WHERE status = 'active' AND date_set >= CURRENT_DATE - 7) as set_this_week,
			COUNT(*) FILTER (WHERE status = 'active' AND date_set >= CURRENT_DATE - 14 AND date_set < CURRENT_DATE - 7) as set_last_week
		FROM routes
		WHERE location_id = $1 AND deleted_at IS NULL`

	if err := r.db.QueryRow(ctx, routeQuery, locationID).Scan(
		&d.ActiveRoutes, &d.DueForStrip, &d.AvgRating, &d.SetThisWeek, &d.SetLastWeek,
	); err != nil {
		return nil, fmt.Errorf("dashboard route stats: %w", err)
	}

	d.ActiveDelta = d.SetThisWeek - d.SetLastWeek

	// Sends in last 30 days
	sendsQuery := `
		SELECT COUNT(*)
		FROM ascents a
		JOIN routes r ON r.id = a.route_id
		WHERE r.location_id = $1
			AND a.ascent_type IN ('send', 'flash')
			AND a.climbed_at >= NOW() - interval '30 days'`

	if err := r.db.QueryRow(ctx, sendsQuery, locationID).Scan(&d.TotalSends30d); err != nil {
		return nil, fmt.Errorf("dashboard sends: %w", err)
	}

	return d, nil
}

// GradeDistribution returns route counts by grade for a location, optionally filtered by wall.
func (r *AnalyticsRepo) GradeDistribution(ctx context.Context, locationID, wallID string) ([]GradeCount, error) {
	query := `
		SELECT grading_system,
			CASE WHEN grading_system = 'circuit' THEN COALESCE(circuit_color, grade) ELSE grade END AS grade,
			route_type, COUNT(*) as count
		FROM routes
		WHERE location_id = $1 AND status = 'active' AND deleted_at IS NULL`
	args := []interface{}{locationID}

	if wallID != "" {
		query += ` AND wall_id = $2`
		args = append(args, wallID)
	}

	query += ` GROUP BY grading_system, CASE WHEN grading_system = 'circuit' THEN COALESCE(circuit_color, grade) ELSE grade END, route_type
		ORDER BY grading_system, grade`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("grade distribution: %w", err)
	}
	defer rows.Close()

	var grades []GradeCount
	for rows.Next() {
		var g GradeCount
		if err := rows.Scan(&g.GradingSystem, &g.Grade, &g.RouteType, &g.Count); err != nil {
			return nil, fmt.Errorf("scan grade: %w", err)
		}
		grades = append(grades, g)
	}
	return grades, rows.Err()
}

// RouteLifecycle returns age and status info for active routes.
func (r *AnalyticsRepo) RouteLifecycle(ctx context.Context, locationID string) ([]RouteAgeInfo, error) {
	query := `
		SELECT id, grade, grading_system, wall_id, color, date_set, projected_strip_date,
			CURRENT_DATE - date_set as age_days,
			avg_rating, ascent_count
		FROM routes
		WHERE location_id = $1 AND status = 'active' AND deleted_at IS NULL
		ORDER BY date_set ASC`

	rows, err := r.db.Query(ctx, query, locationID)
	if err != nil {
		return nil, fmt.Errorf("route lifecycle: %w", err)
	}
	defer rows.Close()

	var routes []RouteAgeInfo
	for rows.Next() {
		var ri RouteAgeInfo
		if err := rows.Scan(
			&ri.RouteID, &ri.Grade, &ri.GradingSystem, &ri.WallID, &ri.Color,
			&ri.DateSet, &ri.ProjectedStripDate, &ri.AgeDays,
			&ri.AvgRating, &ri.AscentCount,
		); err != nil {
			return nil, fmt.Errorf("scan lifecycle: %w", err)
		}
		routes = append(routes, ri)
	}
	return routes, rows.Err()
}

// Engagement returns climber activity metrics for a location.
func (r *AnalyticsRepo) Engagement(ctx context.Context, locationID string, days int) (*EngagementStats, error) {
	if days <= 0 {
		days = 30
	}

	stats := &EngagementStats{}

	// Use integer interval to avoid string concatenation SQL injection surface
	query := `
		SELECT
			COUNT(DISTINCT a.user_id) as active_climbers,
			COUNT(*) as total_ascents,
			COUNT(*) FILTER (WHERE a.ascent_type IN ('send', 'flash')) as total_sends
		FROM ascents a
		JOIN routes r ON r.id = a.route_id
		WHERE r.location_id = $1 AND a.climbed_at >= NOW() - make_interval(days => $2)`

	if err := r.db.QueryRow(ctx, query, locationID, days).Scan(
		&stats.ActiveClimbers, &stats.TotalAscents, &stats.TotalSends,
	); err != nil {
		return nil, fmt.Errorf("engagement: %w", err)
	}

	// Trending routes (most ascents in period)
	trendingQuery := `
		SELECT r.id, r.grade, r.grading_system, r.color, r.name, COUNT(*) as ascent_count
		FROM ascents a
		JOIN routes r ON r.id = a.route_id
		WHERE r.location_id = $1 AND a.climbed_at >= NOW() - make_interval(days => $2)
		GROUP BY r.id, r.grade, r.grading_system, r.color, r.name
		ORDER BY ascent_count DESC
		LIMIT 10`

	rows, err := r.db.Query(ctx, trendingQuery, locationID, days)
	if err != nil {
		return nil, fmt.Errorf("trending: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var t TrendingRoute
		if err := rows.Scan(&t.RouteID, &t.Grade, &t.GradingSystem, &t.Color, &t.Name, &t.AscentCount); err != nil {
			return nil, fmt.Errorf("scan trending: %w", err)
		}
		stats.TrendingRoutes = append(stats.TrendingRoutes, t)
	}

	return stats, rows.Err()
}

// SetterProductivity returns per-setter metrics for a location.
func (r *AnalyticsRepo) SetterProductivity(ctx context.Context, locationID string, days int) ([]SetterStats, error) {
	if days <= 0 {
		days = 30
	}

	query := `
		SELECT u.id, u.display_name,
			COUNT(r.id) as routes_set,
			COALESCE(AVG(r.avg_rating), 0) as avg_route_rating,
			COALESCE(SUM(l.hours_worked), 0) as total_hours
		FROM users u
		JOIN user_memberships um ON um.user_id = u.id
		LEFT JOIN routes r ON r.setter_id = u.id AND r.location_id = $1
			AND r.date_set >= CURRENT_DATE - $2
			AND r.deleted_at IS NULL
		LEFT JOIN setter_labor_logs l ON l.user_id = u.id AND l.location_id = $1
			AND l.date >= CURRENT_DATE - $2
		WHERE um.location_id = $1 AND um.role IN ('setter', 'head_setter') AND um.deleted_at IS NULL
		GROUP BY u.id, u.display_name
		ORDER BY routes_set DESC`

	rows, err := r.db.Query(ctx, query, locationID, days)
	if err != nil {
		return nil, fmt.Errorf("setter productivity: %w", err)
	}
	defer rows.Close()

	var stats []SetterStats
	for rows.Next() {
		var s SetterStats
		if err := rows.Scan(
			&s.SetterID, &s.SetterName, &s.RoutesSet,
			&s.AvgRouteRating, &s.TotalHours,
		); err != nil {
			return nil, fmt.Errorf("scan setter stats: %w", err)
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

// RecentActivity returns the latest climber actions at a location for the activity feed.
func (r *AnalyticsRepo) RecentActivity(ctx context.Context, locationID string, limit int) ([]ActivityEntry, error) {
	if limit <= 0 {
		limit = 10
	}

	query := `
		SELECT u.display_name, a.ascent_type, a.climbed_at,
			r.color, r.grade, r.grading_system, r.circuit_color, r.name
		FROM ascents a
		JOIN routes r ON r.id = a.route_id
		JOIN users u ON u.id = a.user_id
		WHERE r.location_id = $1
		ORDER BY a.climbed_at DESC
		LIMIT $2`

	rows, err := r.db.Query(ctx, query, locationID, limit)
	if err != nil {
		return nil, fmt.Errorf("recent activity: %w", err)
	}
	defer rows.Close()

	var entries []ActivityEntry
	for rows.Next() {
		var e ActivityEntry
		if err := rows.Scan(
			&e.UserName, &e.AscentType, &e.Time,
			&e.RouteColor, &e.RouteGrade, &e.RouteGradingSystem, &e.RouteCircuitColor, &e.RouteName,
		); err != nil {
			return nil, fmt.Errorf("scan activity: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// OrgOverview returns high-level metrics across all locations in an org.
func (r *AnalyticsRepo) OrgOverview(ctx context.Context, orgID string) ([]LocationOverview, error) {
	query := `
		SELECT l.id, l.name,
			COUNT(DISTINCT r.id) FILTER (WHERE r.status = 'active' AND r.deleted_at IS NULL) as active_routes,
			COUNT(DISTINCT a.user_id) FILTER (WHERE a.climbed_at >= NOW() - interval '30 days') as active_climbers_30d,
			COUNT(DISTINCT r2.id) as overdue_strips
		FROM locations l
		LEFT JOIN routes r ON r.location_id = l.id
		LEFT JOIN ascents a ON a.route_id = r.id
		LEFT JOIN routes r2 ON r2.location_id = l.id AND r2.status = 'active'
			AND r2.projected_strip_date <= CURRENT_DATE AND r2.deleted_at IS NULL
		WHERE l.org_id = $1 AND l.deleted_at IS NULL
		GROUP BY l.id, l.name
		ORDER BY l.name`

	rows, err := r.db.Query(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("org overview: %w", err)
	}
	defer rows.Close()

	var overview []LocationOverview
	for rows.Next() {
		var lo LocationOverview
		if err := rows.Scan(
			&lo.LocationID, &lo.LocationName,
			&lo.ActiveRoutes, &lo.ActiveClimbers30d, &lo.OverdueStrips,
		); err != nil {
			return nil, fmt.Errorf("scan overview: %w", err)
		}
		overview = append(overview, lo)
	}
	return overview, rows.Err()
}

// Types

type LocationDashboard struct {
	ActiveRoutes  int     `json:"active_routes"`
	ActiveDelta   int     `json:"active_delta"`
	TotalSends30d int     `json:"total_sends_30d"`
	AvgRating     float64 `json:"avg_rating"`
	DueForStrip   int     `json:"due_for_strip"`
	SetThisWeek   int     `json:"set_this_week"`
	SetLastWeek   int     `json:"set_last_week"`
}

type GradeCount struct {
	GradingSystem string `json:"grading_system"`
	Grade         string `json:"grade"`
	RouteType     string `json:"route_type"`
	Count         int    `json:"count"`
}

type RouteAgeInfo struct {
	RouteID            string      `json:"route_id"`
	Grade              string      `json:"grade"`
	GradingSystem      string      `json:"grading_system"`
	WallID             string      `json:"wall_id"`
	Color              string      `json:"color"`
	DateSet            interface{} `json:"date_set"`
	ProjectedStripDate interface{} `json:"projected_strip_date"`
	AgeDays            int         `json:"age_days"`
	AvgRating          float64     `json:"avg_rating"`
	AscentCount        int         `json:"ascent_count"`
}

type EngagementStats struct {
	ActiveClimbers int             `json:"active_climbers"`
	TotalAscents   int             `json:"total_ascents"`
	TotalSends     int             `json:"total_sends"`
	TrendingRoutes []TrendingRoute `json:"trending_routes"`
}

type TrendingRoute struct {
	RouteID       string  `json:"route_id"`
	Grade         string  `json:"grade"`
	GradingSystem string  `json:"grading_system"`
	Color         string  `json:"color"`
	Name          *string `json:"name,omitempty"`
	AscentCount   int     `json:"ascent_count"`
}

type SetterStats struct {
	SetterID       string  `json:"setter_id"`
	SetterName     string  `json:"setter_name"`
	RoutesSet      int     `json:"routes_set"`
	AvgRouteRating float64 `json:"avg_route_rating"`
	TotalHours     float64 `json:"total_hours"`
}

type ActivityEntry struct {
	UserName          string    `json:"user_name"`
	AscentType        string    `json:"ascent_type"`
	Time              time.Time `json:"time"`
	RouteColor        string    `json:"route_color"`
	RouteGrade        string    `json:"route_grade"`
	RouteGradingSystem string   `json:"route_grading_system"`
	RouteCircuitColor *string   `json:"route_circuit_color,omitempty"`
	RouteName         *string   `json:"route_name,omitempty"`
}

type LocationOverview struct {
	LocationID       string `json:"location_id"`
	LocationName     string `json:"location_name"`
	ActiveRoutes     int    `json:"active_routes"`
	ActiveClimbers30d int   `json:"active_climbers_30d"`
	OverdueStrips    int    `json:"overdue_strips"`
}
