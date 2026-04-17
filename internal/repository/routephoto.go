package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

// RoutePhotoRepo handles CRUD for route photos.
type RoutePhotoRepo struct {
	db *pgxpool.Pool
}

func NewRoutePhotoRepo(db *pgxpool.Pool) *RoutePhotoRepo {
	return &RoutePhotoRepo{db: db}
}

// PhotoWithUploader extends RoutePhoto with uploader info for display.
type PhotoWithUploader struct {
	model.RoutePhoto
	UploaderName    string
	UploaderInitial string
}

// Create inserts a new route photo. StorageKey may be nil for legacy callers;
// new upload paths should always supply it so future deletes can hit S3 by key.
func (r *RoutePhotoRepo) Create(ctx context.Context, p *model.RoutePhoto) error {
	return r.db.QueryRow(ctx, `
		INSERT INTO route_photos (route_id, photo_url, storage_key, caption, uploaded_by, sort_order)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at`,
		p.RouteID, p.PhotoURL, p.StorageKey, p.Caption, p.UploadedBy, p.SortOrder,
	).Scan(&p.ID, &p.CreatedAt)
}

// ListByRoute returns all photos for a route, ordered by sort_order then newest.
func (r *RoutePhotoRepo) ListByRoute(ctx context.Context, routeID string) ([]PhotoWithUploader, error) {
	rows, err := r.db.Query(ctx, `
		SELECT rp.id, rp.route_id, rp.photo_url, rp.storage_key, rp.caption, rp.uploaded_by,
		       rp.sort_order, rp.created_at,
		       COALESCE(u.display_name, 'Unknown') AS uploader_name
		FROM route_photos rp
		LEFT JOIN users u ON u.id = rp.uploaded_by
		WHERE rp.route_id = $1
		ORDER BY rp.sort_order, rp.created_at DESC`,
		routeID,
	)
	if err != nil {
		return nil, fmt.Errorf("list route photos: %w", err)
	}
	defer rows.Close()

	var photos []PhotoWithUploader
	for rows.Next() {
		var p PhotoWithUploader
		if err := rows.Scan(
			&p.ID, &p.RouteID, &p.PhotoURL, &p.StorageKey, &p.Caption, &p.UploadedBy,
			&p.SortOrder, &p.CreatedAt,
			&p.UploaderName,
		); err != nil {
			return nil, fmt.Errorf("scan route photo: %w", err)
		}
		if len(p.UploaderName) > 0 {
			p.UploaderInitial = string([]rune(p.UploaderName)[0:1])
		}
		photos = append(photos, p)
	}
	return photos, nil
}

// GetByID returns a single photo by ID.
func (r *RoutePhotoRepo) GetByID(ctx context.Context, id string) (*model.RoutePhoto, error) {
	p := &model.RoutePhoto{}
	err := r.db.QueryRow(ctx, `
		SELECT id, route_id, photo_url, storage_key, caption, uploaded_by, sort_order, created_at
		FROM route_photos
		WHERE id = $1`,
		id,
	).Scan(&p.ID, &p.RouteID, &p.PhotoURL, &p.StorageKey, &p.Caption, &p.UploadedBy, &p.SortOrder, &p.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get route photo: %w", err)
	}
	return p, nil
}

// CountByRoute returns the number of photos for a route.
func (r *RoutePhotoRepo) CountByRoute(ctx context.Context, routeID string) (int, error) {
	var count int
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM route_photos WHERE route_id = $1`,
		routeID,
	).Scan(&count)
	return count, err
}

// Delete removes a photo by ID.
func (r *RoutePhotoRepo) Delete(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM route_photos WHERE id = $1`, id)
	return err
}

// NextSortOrder returns the next available sort order for a route's photos.
func (r *RoutePhotoRepo) NextSortOrder(ctx context.Context, routeID string) (int, error) {
	var maxOrder int
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(MAX(sort_order), -1) + 1 FROM route_photos WHERE route_id = $1`,
		routeID,
	).Scan(&maxOrder)
	return maxOrder, err
}
