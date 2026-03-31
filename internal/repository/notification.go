package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Notification represents an in-app notification.
type Notification struct {
	ID        int64      `json:"id"`
	UserID    string     `json:"user_id"`
	Type      string     `json:"type"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	Link      *string    `json:"link,omitempty"`
	ReadAt    *time.Time `json:"read_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type NotificationRepo struct {
	db *pgxpool.Pool
}

func NewNotificationRepo(db *pgxpool.Pool) *NotificationRepo {
	return &NotificationRepo{db: db}
}

// Create inserts a new notification and returns its ID.
func (r *NotificationRepo) Create(ctx context.Context, n Notification) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx,
		`INSERT INTO notifications (user_id, type, title, body, link)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id`,
		n.UserID, n.Type, n.Title, n.Body, n.Link,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create notification: %w", err)
	}
	return id, nil
}

// ListUnread returns unread notifications for a user, newest first.
func (r *NotificationRepo) ListUnread(ctx context.Context, userID string, limit int) ([]Notification, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.Query(ctx,
		`SELECT id, user_id, type, title, body, link, read_at, created_at
		 FROM notifications
		 WHERE user_id = $1 AND read_at IS NULL
		 ORDER BY created_at DESC
		 LIMIT $2`,
		userID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list unread notifications: %w", err)
	}
	defer rows.Close()

	var out []Notification
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.UserID, &n.Type, &n.Title, &n.Body, &n.Link, &n.ReadAt, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// UnreadCount returns the number of unread notifications for a user.
func (r *NotificationRepo) UnreadCount(ctx context.Context, userID string) (int64, error) {
	var count int64
	err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL`,
		userID,
	).Scan(&count)
	return count, err
}

// MarkRead marks a single notification as read.
func (r *NotificationRepo) MarkRead(ctx context.Context, id int64, userID string) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE notifications SET read_at = NOW() WHERE id = $1 AND user_id = $2 AND read_at IS NULL`,
		id, userID,
	)
	if err != nil {
		return fmt.Errorf("mark notification read: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("notification %d not found or already read", id)
	}
	return nil
}

// MarkAllRead marks all unread notifications as read for a user.
func (r *NotificationRepo) MarkAllRead(ctx context.Context, userID string) (int64, error) {
	tag, err := r.db.Exec(ctx,
		`UPDATE notifications SET read_at = NOW() WHERE user_id = $1 AND read_at IS NULL`,
		userID,
	)
	if err != nil {
		return 0, fmt.Errorf("mark all read: %w", err)
	}
	return tag.RowsAffected(), nil
}

// DeleteOld removes notifications older than the given duration.
func (r *NotificationRepo) DeleteOld(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	tag, err := r.db.Exec(ctx,
		`DELETE FROM notifications WHERE created_at < $1`,
		cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("delete old notifications: %w", err)
	}
	return tag.RowsAffected(), nil
}
