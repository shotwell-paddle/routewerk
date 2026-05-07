package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shotwell-paddle/routewerk/internal/database"
)

// RouteDistributionTarget is the per-(location, route_type, grade)
// goal head-setters set so the dashboard can overlay actual vs target
// on the distribution charts. See migration 000036.
type RouteDistributionTarget struct {
	ID          string    `json:"id"`
	LocationID  string    `json:"location_id"`
	RouteType   string    `json:"route_type"` // 'boulder', 'route', or 'circuit'
	Grade       string    `json:"grade"`      // V4, 5.10a, "red", …
	TargetCount int       `json:"target_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// RouteDistributionTargetRepo wraps the route_distribution_targets
// table. The full set per location is small (a few dozen rows at most
// — one per grade/circuit bucket the gym tracks), so we list-all and
// replace-all rather than maintaining per-row PATCH endpoints.
type RouteDistributionTargetRepo struct {
	db *pgxpool.Pool
}

func NewRouteDistributionTargetRepo(db *pgxpool.Pool) *RouteDistributionTargetRepo {
	return &RouteDistributionTargetRepo{db: db}
}

// ListByLocation returns every target for a location, ordered by
// (route_type, grade) so the SPA can render directly without sorting.
func (r *RouteDistributionTargetRepo) ListByLocation(ctx context.Context, locationID string) ([]RouteDistributionTarget, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, location_id, route_type, grade, target_count, created_at, updated_at
		FROM route_distribution_targets
		WHERE location_id = $1
		ORDER BY route_type, grade`, locationID)
	if err != nil {
		return nil, fmt.Errorf("list distribution targets: %w", err)
	}
	defer rows.Close()

	var out []RouteDistributionTarget
	for rows.Next() {
		var t RouteDistributionTarget
		if err := rows.Scan(&t.ID, &t.LocationID, &t.RouteType, &t.Grade, &t.TargetCount, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan distribution target: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ReplaceAll wipes the location's existing targets and inserts the
// caller's set in a single transaction. The PUT semantics suit the UI:
// the editor loads every row, lets the head-setter add/edit/remove
// freely, then submits the final set as one payload. Per-row CRUD
// would multiply round-trips for no real benefit at this scale.
//
// Filter: target_count == 0 rows are dropped on insert (a target of
// zero is the same as not having a target — the chart treats both as
// "no goal set" and the row would just take up space in the table).
func (r *RouteDistributionTargetRepo) ReplaceAll(ctx context.Context, locationID string, targets []RouteDistributionTarget) error {
	return database.RunInTx(ctx, r.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM route_distribution_targets WHERE location_id = $1`, locationID); err != nil {
			return fmt.Errorf("delete existing targets: %w", err)
		}
		for _, t := range targets {
			if t.TargetCount <= 0 {
				continue
			}
			if t.Grade == "" || t.RouteType == "" {
				continue
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO route_distribution_targets
					(location_id, route_type, grade, target_count)
				VALUES ($1, $2, $3, $4)
				ON CONFLICT (location_id, route_type, grade) DO UPDATE
					SET target_count = EXCLUDED.target_count,
					    updated_at = now()`,
				locationID, t.RouteType, t.Grade, t.TargetCount,
			); err != nil {
				return fmt.Errorf("insert target (%s/%s): %w", t.RouteType, t.Grade, err)
			}
		}
		return nil
	})
}
