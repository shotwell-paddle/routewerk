package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

type WallRepo struct {
	db *pgxpool.Pool
}

func NewWallRepo(db *pgxpool.Pool) *WallRepo {
	return &WallRepo{db: db}
}

func (r *WallRepo) Create(ctx context.Context, w *model.Wall) error {
	query := `
		INSERT INTO walls (location_id, name, wall_type, angle, height_meters, num_anchors, surface_type, sort_order, map_x, map_y, map_width, map_height)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, created_at, updated_at`

	return r.db.QueryRow(ctx, query,
		w.LocationID, w.Name, w.WallType, w.Angle, w.HeightMeters,
		w.NumAnchors, w.SurfaceType, w.SortOrder,
		w.MapX, w.MapY, w.MapWidth, w.MapHeight,
	).Scan(&w.ID, &w.CreatedAt, &w.UpdatedAt)
}

func (r *WallRepo) GetByID(ctx context.Context, id string) (*model.Wall, error) {
	query := `
		SELECT id, location_id, name, wall_type, angle, height_meters, num_anchors,
			surface_type, sort_order, map_x, map_y, map_width, map_height,
			created_at, updated_at
		FROM walls
		WHERE id = $1 AND deleted_at IS NULL`

	w := &model.Wall{}
	err := r.db.QueryRow(ctx, query, id).Scan(
		&w.ID, &w.LocationID, &w.Name, &w.WallType, &w.Angle, &w.HeightMeters,
		&w.NumAnchors, &w.SurfaceType, &w.SortOrder,
		&w.MapX, &w.MapY, &w.MapWidth, &w.MapHeight,
		&w.CreatedAt, &w.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get wall by id: %w", err)
	}
	return w, nil
}

func (r *WallRepo) ListByLocation(ctx context.Context, locationID string) ([]model.Wall, error) {
	query := `
		SELECT id, location_id, name, wall_type, angle, height_meters, num_anchors,
			surface_type, sort_order, map_x, map_y, map_width, map_height,
			created_at, updated_at
		FROM walls
		WHERE location_id = $1 AND deleted_at IS NULL
		ORDER BY sort_order, name`

	rows, err := r.db.Query(ctx, query, locationID)
	if err != nil {
		return nil, fmt.Errorf("list walls by location: %w", err)
	}
	defer rows.Close()

	var walls []model.Wall
	for rows.Next() {
		var w model.Wall
		if err := rows.Scan(
			&w.ID, &w.LocationID, &w.Name, &w.WallType, &w.Angle, &w.HeightMeters,
			&w.NumAnchors, &w.SurfaceType, &w.SortOrder,
			&w.MapX, &w.MapY, &w.MapWidth, &w.MapHeight,
			&w.CreatedAt, &w.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan wall: %w", err)
		}
		walls = append(walls, w)
	}
	return walls, nil
}

// WallWithCounts extends Wall with aggregate route counts.
type WallWithCounts struct {
	model.Wall
	ActiveRoutes   int `json:"active_routes"`
	FlaggedRoutes  int `json:"flagged_routes"`
	ArchivedRoutes int `json:"archived_routes"`
	TotalRoutes    int `json:"total_routes"`
}

// ListWithCounts returns walls with per-status route counts.
func (r *WallRepo) ListWithCounts(ctx context.Context, locationID string) ([]WallWithCounts, error) {
	query := `
		SELECT w.id, w.location_id, w.name, w.wall_type, w.angle, w.height_meters,
			w.num_anchors, w.surface_type, w.sort_order,
			w.map_x, w.map_y, w.map_width, w.map_height,
			w.created_at, w.updated_at,
			COUNT(*) FILTER (WHERE r.status = 'active' AND r.deleted_at IS NULL) AS active_routes,
			COUNT(*) FILTER (WHERE r.status = 'flagged' AND r.deleted_at IS NULL) AS flagged_routes,
			COUNT(*) FILTER (WHERE r.status = 'archived' AND r.deleted_at IS NULL) AS archived_routes,
			COUNT(*) FILTER (WHERE r.deleted_at IS NULL) AS total_routes
		FROM walls w
		LEFT JOIN routes r ON r.wall_id = w.id
		WHERE w.location_id = $1 AND w.deleted_at IS NULL
		GROUP BY w.id
		ORDER BY w.sort_order, w.name`

	rows, err := r.db.Query(ctx, query, locationID)
	if err != nil {
		return nil, fmt.Errorf("list walls with counts: %w", err)
	}
	defer rows.Close()

	var walls []WallWithCounts
	for rows.Next() {
		var wc WallWithCounts
		if err := rows.Scan(
			&wc.ID, &wc.LocationID, &wc.Name, &wc.WallType, &wc.Angle, &wc.HeightMeters,
			&wc.NumAnchors, &wc.SurfaceType, &wc.SortOrder,
			&wc.MapX, &wc.MapY, &wc.MapWidth, &wc.MapHeight,
			&wc.CreatedAt, &wc.UpdatedAt,
			&wc.ActiveRoutes, &wc.FlaggedRoutes, &wc.ArchivedRoutes, &wc.TotalRoutes,
		); err != nil {
			return nil, fmt.Errorf("scan wall with counts: %w", err)
		}
		walls = append(walls, wc)
	}
	return walls, nil
}

func (r *WallRepo) Update(ctx context.Context, w *model.Wall) error {
	query := `
		UPDATE walls
		SET name = $2, wall_type = $3, angle = $4, height_meters = $5, num_anchors = $6,
			surface_type = $7, sort_order = $8, map_x = $9, map_y = $10,
			map_width = $11, map_height = $12
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING updated_at`

	return r.db.QueryRow(ctx, query,
		w.ID, w.Name, w.WallType, w.Angle, w.HeightMeters,
		w.NumAnchors, w.SurfaceType, w.SortOrder,
		w.MapX, w.MapY, w.MapWidth, w.MapHeight,
	).Scan(&w.UpdatedAt)
}

func (r *WallRepo) Delete(ctx context.Context, id string) error {
	query := `UPDATE walls SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`
	_, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("soft delete wall: %w", err)
	}
	return nil
}
