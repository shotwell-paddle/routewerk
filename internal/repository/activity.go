package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

type ActivityRepo struct {
	db *pgxpool.Pool
}

func NewActivityRepo(db *pgxpool.Pool) *ActivityRepo {
	return &ActivityRepo{db: db}
}

// Insert records a new activity log entry. Metadata is stored as JSONB.
func (r *ActivityRepo) Insert(ctx context.Context, entry *model.ActivityLogEntry) error {
	metaJSON, err := json.Marshal(entry.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	return r.db.QueryRow(ctx,
		`INSERT INTO activity_log (location_id, user_id, activity_type, entity_type, entity_id, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, created_at`,
		entry.LocationID, entry.UserID, entry.ActivityType,
		entry.EntityType, entry.EntityID, metaJSON,
	).Scan(&entry.ID, &entry.CreatedAt)
}

// ListByLocation returns the activity feed for a location, newest first.
// Joins to users for display name and avatar.
func (r *ActivityRepo) ListByLocation(ctx context.Context, locationID string, limit, offset int) ([]model.ActivityLogEntry, error) {
	if limit <= 0 {
		limit = 30
	}
	rows, err := r.db.Query(ctx,
		`SELECT a.id, a.location_id, a.user_id, a.activity_type,
		        a.entity_type, a.entity_id, a.metadata, a.created_at,
		        u.display_name, u.avatar_url
		 FROM activity_log a
		 JOIN users u ON u.id = a.user_id
		 WHERE a.location_id = $1
		 ORDER BY a.created_at DESC
		 LIMIT $2 OFFSET $3`,
		locationID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list activity by location: %w", err)
	}
	defer rows.Close()

	return scanActivityRows(rows)
}

// ListByUser returns a specific user's activity, newest first.
func (r *ActivityRepo) ListByUser(ctx context.Context, userID string, limit, offset int) ([]model.ActivityLogEntry, error) {
	if limit <= 0 {
		limit = 30
	}
	rows, err := r.db.Query(ctx,
		`SELECT a.id, a.location_id, a.user_id, a.activity_type,
		        a.entity_type, a.entity_id, a.metadata, a.created_at,
		        u.display_name, u.avatar_url
		 FROM activity_log a
		 JOIN users u ON u.id = a.user_id
		 WHERE a.user_id = $1
		 ORDER BY a.created_at DESC
		 LIMIT $2 OFFSET $3`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list activity by user: %w", err)
	}
	defer rows.Close()

	return scanActivityRows(rows)
}

// ListByLocationAndType returns activity filtered by type (e.g. "quest.completed").
func (r *ActivityRepo) ListByLocationAndType(ctx context.Context, locationID, activityType string, limit, offset int) ([]model.ActivityLogEntry, error) {
	if limit <= 0 {
		limit = 30
	}
	rows, err := r.db.Query(ctx,
		`SELECT a.id, a.location_id, a.user_id, a.activity_type,
		        a.entity_type, a.entity_id, a.metadata, a.created_at,
		        u.display_name, u.avatar_url
		 FROM activity_log a
		 JOIN users u ON u.id = a.user_id
		 WHERE a.location_id = $1 AND a.activity_type = $2
		 ORDER BY a.created_at DESC
		 LIMIT $3 OFFSET $4`,
		locationID, activityType, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list activity by type: %w", err)
	}
	defer rows.Close()

	return scanActivityRows(rows)
}

// scanActivityRows is a shared scanner for all list queries.
func scanActivityRows(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]model.ActivityLogEntry, error) {
	var entries []model.ActivityLogEntry
	for rows.Next() {
		var e model.ActivityLogEntry
		var metaJSON []byte
		if err := rows.Scan(
			&e.ID, &e.LocationID, &e.UserID, &e.ActivityType,
			&e.EntityType, &e.EntityID, &metaJSON, &e.CreatedAt,
			&e.UserDisplayName, &e.UserAvatarURL,
		); err != nil {
			return nil, fmt.Errorf("scan activity: %w", err)
		}
		if metaJSON != nil {
			if err := json.Unmarshal(metaJSON, &e.Metadata); err != nil {
				return nil, fmt.Errorf("unmarshal metadata: %w", err)
			}
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
