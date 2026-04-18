package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shotwell-paddle/routewerk/internal/database"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

type CardBatchRepo struct {
	db *pgxpool.Pool
}

func NewCardBatchRepo(db *pgxpool.Pool) *CardBatchRepo {
	return &CardBatchRepo{db: db}
}

// Create inserts a new batch and fills in the server-generated fields
// (ID, CreatedAt, UpdatedAt) on the passed-in struct.
func (r *CardBatchRepo) Create(ctx context.Context, b *model.CardBatch) error {
	query := `
		INSERT INTO route_card_batches (location_id, created_by, route_ids, theme, cutter_profile, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at`

	return r.db.QueryRow(ctx, query,
		b.LocationID, b.CreatedBy, b.RouteIDs,
		b.Theme, b.CutterProfile, b.Status,
	).Scan(&b.ID, &b.CreatedAt, &b.UpdatedAt)
}

// GetByID returns a single batch or (nil, nil) when the batch does not exist.
func (r *CardBatchRepo) GetByID(ctx context.Context, id string) (*model.CardBatch, error) {
	ctx, cancel := database.QueryTimeout(ctx, database.TimeoutFast)
	defer cancel()

	query := `
		SELECT id, location_id, created_by, route_ids, theme, cutter_profile,
			storage_key, status, error_message, created_at, updated_at
		FROM route_card_batches
		WHERE id = $1`

	b := &model.CardBatch{}
	err := r.db.QueryRow(ctx, query, id).Scan(
		&b.ID, &b.LocationID, &b.CreatedBy, &b.RouteIDs,
		&b.Theme, &b.CutterProfile,
		&b.StorageKey, &b.Status, &b.ErrorMessage,
		&b.CreatedAt, &b.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get card batch by id: %w", err)
	}
	return b, nil
}

// ListByLocation returns the most recent `limit` batches for a location,
// ordered by created_at DESC. Used on the batch-history listing page.
// Pass limit <= 0 to get a sensible default (50).
func (r *CardBatchRepo) ListByLocation(ctx context.Context, locationID string, limit int) ([]model.CardBatch, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `
		SELECT id, location_id, created_by, route_ids, theme, cutter_profile,
			storage_key, status, error_message, created_at, updated_at
		FROM route_card_batches
		WHERE location_id = $1
		ORDER BY created_at DESC
		LIMIT $2`

	rows, err := r.db.Query(ctx, query, locationID, limit)
	if err != nil {
		return nil, fmt.Errorf("list card batches by location: %w", err)
	}
	defer rows.Close()

	var batches []model.CardBatch
	for rows.Next() {
		var b model.CardBatch
		if err := rows.Scan(
			&b.ID, &b.LocationID, &b.CreatedBy, &b.RouteIDs,
			&b.Theme, &b.CutterProfile,
			&b.StorageKey, &b.Status, &b.ErrorMessage,
			&b.CreatedAt, &b.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan card batch: %w", err)
		}
		batches = append(batches, b)
	}
	return batches, rows.Err()
}

// MarkReady transitions a batch to status=ready with the given storage key,
// clearing any prior error message. No-op if the batch is already in a
// terminal state with the same key.
func (r *CardBatchRepo) MarkReady(ctx context.Context, id, storageKey string) error {
	query := `
		UPDATE route_card_batches
		SET status = 'ready', storage_key = $2, error_message = NULL, updated_at = NOW()
		WHERE id = $1`
	ct, err := r.db.Exec(ctx, query, id, storageKey)
	if err != nil {
		return fmt.Errorf("mark card batch ready: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("mark card batch ready: not found")
	}
	return nil
}

// MarkFailed transitions a batch to status=failed and records the error
// message. Storage key is preserved so the UI can explain what went wrong
// on the most recent render attempt.
func (r *CardBatchRepo) MarkFailed(ctx context.Context, id, errMsg string) error {
	query := `
		UPDATE route_card_batches
		SET status = 'failed', error_message = $2, updated_at = NOW()
		WHERE id = $1`
	ct, err := r.db.Exec(ctx, query, id, errMsg)
	if err != nil {
		return fmt.Errorf("mark card batch failed: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("mark card batch failed: not found")
	}
	return nil
}

// UpdateRouteIDs replaces the batch's route_ids and resets it to pending so
// the next download re-renders with the new selection. Theme / cutter are
// not touched — if those need to change, use the dedicated methods (or
// extend this signature).
//
// Returns a "not found" error if the row does not exist so callers can turn
// that into a 404 without a second lookup.
func (r *CardBatchRepo) UpdateRouteIDs(ctx context.Context, id string, routeIDs []string) error {
	query := `
		UPDATE route_card_batches
		SET route_ids = $2, status = 'pending', storage_key = NULL, error_message = NULL, updated_at = NOW()
		WHERE id = $1`
	ct, err := r.db.Exec(ctx, query, id, routeIDs)
	if err != nil {
		return fmt.Errorf("update card batch route ids: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("update card batch route ids: not found")
	}
	return nil
}

// InvalidateStorageKey clears the storage_key and resets status to pending
// so a stale cached PDF will be re-rendered on next download. Called when
// any of the batch's routes have changed since the last render.
func (r *CardBatchRepo) InvalidateStorageKey(ctx context.Context, id string) error {
	query := `
		UPDATE route_card_batches
		SET storage_key = NULL, status = 'pending', error_message = NULL, updated_at = NOW()
		WHERE id = $1`
	_, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("invalidate card batch: %w", err)
	}
	return nil
}

// Delete removes a batch permanently. The storage_key (if any) is NOT
// deleted from object storage here — callers that care about cleanup should
// invoke StorageService.Delete first.
func (r *CardBatchRepo) Delete(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM route_card_batches WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete card batch: %w", err)
	}
	return nil
}

// PurgeOlderThan deletes batches created before cutoff. Used by the nightly
// retention sweep so old print runs don't accumulate indefinitely — setters
// rarely need to reprint a batch more than a month after it was made, and
// routes tend to be reset before then anyway.
//
// Returns the number of rows removed.
func (r *CardBatchRepo) PurgeOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	tag, err := r.db.Exec(ctx,
		`DELETE FROM route_card_batches WHERE created_at < $1`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("purge old card batches: %w", err)
	}
	return tag.RowsAffected(), nil
}

// RoutesWithoutCard returns route IDs for active routes at a location that
// have never been included in a ready batch. Drives the "print cards for
// newly-set routes" picker in the setter UI.
func (r *CardBatchRepo) RoutesWithoutCard(ctx context.Context, locationID string) ([]string, error) {
	query := `
		SELECT r.id
		FROM routes r
		WHERE r.location_id = $1
			AND r.status = 'active'
			AND r.deleted_at IS NULL
			AND NOT EXISTS (
				SELECT 1 FROM route_card_batches b
				WHERE b.location_id = $1
					AND b.status = 'ready'
					AND r.id = ANY(b.route_ids)
			)
		ORDER BY r.date_set DESC`

	rows, err := r.db.Query(ctx, query, locationID)
	if err != nil {
		return nil, fmt.Errorf("routes without card: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan route id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
