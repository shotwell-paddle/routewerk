package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/shotwell-paddle/routewerk/internal/jobs"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

// NotificationService creates in-app notifications and optionally enqueues
// follow-up actions (like email digests) via the job queue.
type NotificationService struct {
	repo *repository.NotificationRepo
	q    *jobs.Queue
}

// NewNotificationService creates a notification service.
func NewNotificationService(repo *repository.NotificationRepo, q *jobs.Queue) *NotificationService {
	return &NotificationService{repo: repo, q: q}
}

// RegisterHandlers registers notification-related job handlers.
func (s *NotificationService) RegisterHandlers(q *jobs.Queue) {
	q.Register("notification.create", s.handleCreate)
	q.Register("notification.cleanup", s.handleCleanup)
}

// ── Notification Types ──────────────────────────────────────────

// Notify creates an in-app notification directly (synchronous).
// Use this for real-time notifications that should appear immediately.
func (s *NotificationService) Notify(ctx context.Context, n repository.Notification) (int64, error) {
	id, err := s.repo.Create(ctx, n)
	if err != nil {
		return 0, err
	}
	slog.Debug("notification created", "id", id, "user_id", n.UserID, "type", n.Type)
	return id, nil
}

// NotifyAsync enqueues a notification creation as a background job.
// Use this when you don't want to block the request on notification creation.
func (s *NotificationService) NotifyAsync(ctx context.Context, n repository.Notification) error {
	payload, err := json.Marshal(n)
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}
	_, err = s.q.Enqueue(ctx, jobs.EnqueueParams{
		JobType: "notification.create",
		Payload: payload,
	})
	return err
}

// ── Convenience Methods ─────────────────────────────────────────

func strPtr(s string) *string { return &s }

// NotifyRouteRated notifies a setter that their route received a rating.
func (s *NotificationService) NotifyRouteRated(ctx context.Context, setterID, climberName, routeName, routeID string, rating int) {
	s.NotifyAsync(ctx, repository.Notification{ //nolint:errcheck
		UserID: setterID,
		Type:   "route.rated",
		Title:  fmt.Sprintf("%s rated your route", climberName),
		Body:   fmt.Sprintf("%s gave %s %d stars", climberName, routeName, rating),
		Link:   strPtr("/routes/" + routeID),
	})
}

// NotifyRouteAscent notifies a setter that their route was climbed.
func (s *NotificationService) NotifyRouteAscent(ctx context.Context, setterID, climberName, routeName, routeID, style string) {
	s.NotifyAsync(ctx, repository.Notification{ //nolint:errcheck
		UserID: setterID,
		Type:   "route.ascent",
		Title:  fmt.Sprintf("%s climbed your route", climberName),
		Body:   fmt.Sprintf("%s logged a %s on %s", climberName, style, routeName),
		Link:   strPtr("/routes/" + routeID),
	})
}

// NotifySessionPublished notifies assigned setters that a session was published.
func (s *NotificationService) NotifySessionPublished(ctx context.Context, setterIDs []string, sessionName, sessionID string) {
	for _, id := range setterIDs {
		s.NotifyAsync(ctx, repository.Notification{ //nolint:errcheck
			UserID: id,
			Type:   "session.published",
			Title:  "Setting session published",
			Body:   fmt.Sprintf("The session \"%s\" has been published", sessionName),
			Link:   strPtr("/sessions/" + sessionID),
		})
	}
}

// NotifyNewFollower notifies a user that someone followed them.
func (s *NotificationService) NotifyNewFollower(ctx context.Context, userID, followerName string) {
	s.NotifyAsync(ctx, repository.Notification{ //nolint:errcheck
		UserID: userID,
		Type:   "social.follow",
		Title:  fmt.Sprintf("%s started following you", followerName),
		Body:   "",
		Link:   strPtr("/profile"),
	})
}

// ── Job Handlers ────────────────────────────────────────────────

func (s *NotificationService) handleCreate(_ context.Context, job jobs.Job) error {
	var n repository.Notification
	if err := json.Unmarshal(job.Payload, &n); err != nil {
		return fmt.Errorf("unmarshal notification: %w", err)
	}
	// Use a fresh context since the job context may have a short timeout
	ctx := context.Background()
	_, err := s.repo.Create(ctx, n)
	return err
}

func (s *NotificationService) handleCleanup(_ context.Context, _ jobs.Job) error {
	ctx := context.Background()
	deleted, err := s.repo.DeleteOld(ctx, 90*24*time.Hour)
	if err != nil {
		return err
	}
	if deleted > 0 {
		slog.Info("cleaned up old notifications", "deleted", deleted)
	}
	return nil
}
