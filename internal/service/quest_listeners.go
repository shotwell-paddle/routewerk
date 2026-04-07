package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/shotwell-paddle/routewerk/internal/event"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

// QuestListeners registers event bus subscribers for the progressions
// feature. Three concerns are separated into distinct handlers:
//
//   - Badge award (sync): runs in the caller's goroutine so that the
//     badge is guaranteed to exist before the response returns. This
//     lets the handler show "You earned a badge!" immediately.
//
//   - Activity log (async): writes to the activity_log table for the
//     gym feed. Fire-and-forget — no need to block the request.
//
//   - Notifications (async): creates in-app notifications for the
//     climber. Also fire-and-forget.
type QuestListeners struct {
	badges   *repository.BadgeRepo
	activity *repository.ActivityRepo
	notifs   *NotificationService
	bus      event.Bus
}

func NewQuestListeners(badges *repository.BadgeRepo, activity *repository.ActivityRepo, notifs *NotificationService, bus event.Bus) *QuestListeners {
	return &QuestListeners{
		badges:   badges,
		activity: activity,
		notifs:   notifs,
		bus:      bus,
	}
}

// Register subscribes all progressions listeners on the bus.
func (l *QuestListeners) Register() {
	// Badge award — SYNC: must complete before response
	l.bus.Subscribe(event.QuestCompleted, l.awardBadge, true)

	// Activity log — ASYNC: background write
	l.bus.Subscribe(event.QuestStarted, l.logActivity, false)
	l.bus.Subscribe(event.QuestCompleted, l.logActivity, false)
	l.bus.Subscribe(event.ProgressLogged, l.logActivity, false)
	l.bus.Subscribe(event.BadgeEarned, l.logActivity, false)

	// Notifications — ASYNC: background notification
	l.bus.Subscribe(event.QuestCompleted, l.notifyQuestCompleted, false)
	l.bus.Subscribe(event.BadgeEarned, l.notifyBadgeEarned, false)
}

// ============================================================
// Badge Award (sync)
// ============================================================

func (l *QuestListeners) awardBadge(ctx context.Context, e event.Event) error {
	payload, ok := e.Payload.(event.QuestCompletedPayload)
	if !ok {
		return fmt.Errorf("unexpected payload type for quest.completed: %T", e.Payload)
	}
	if payload.BadgeID == "" {
		return nil // quest has no badge — nothing to award
	}

	// Idempotent award (ON CONFLICT DO NOTHING in repo)
	_, err := l.badges.AwardBadge(ctx, e.UserID, payload.BadgeID)
	if err != nil {
		return fmt.Errorf("award badge: %w", err)
	}

	slog.Info("badge awarded",
		"user_id", e.UserID,
		"badge_id", payload.BadgeID,
		"badge_name", payload.BadgeName,
		"quest", payload.QuestName,
	)

	// Publish a secondary event so activity + notification listeners pick it up
	l.bus.Publish(ctx, event.Event{
		Type:   event.BadgeEarned,
		GymID:  e.GymID,
		UserID: e.UserID,
		Payload: event.BadgeEarnedPayload{
			BadgeID:    payload.BadgeID,
			BadgeName:  payload.BadgeName,
			BadgeIcon:  payload.BadgeIcon,
			BadgeColor: payload.BadgeColor,
			QuestName:  payload.QuestName,
		},
		Timestamp: e.Timestamp,
	})

	return nil
}

// ============================================================
// Activity Log (async)
// ============================================================

func (l *QuestListeners) logActivity(ctx context.Context, e event.Event) error {
	entry := &model.ActivityLogEntry{
		LocationID:   e.GymID,
		UserID:       e.UserID,
		ActivityType: e.Type,
	}

	switch p := e.Payload.(type) {
	case event.QuestStartedPayload:
		entry.EntityType = "quest"
		entry.EntityID = p.QuestID
		entry.Metadata = map[string]any{
			"quest_name":   p.QuestName,
			"domain_name":  p.DomainName,
			"domain_color": p.DomainColor,
		}

	case event.QuestCompletedPayload:
		entry.EntityType = "quest"
		entry.EntityID = p.QuestID
		entry.Metadata = map[string]any{
			"quest_name":   p.QuestName,
			"domain_name":  p.DomainName,
			"domain_color": p.DomainColor,
		}
		if p.BadgeID != "" {
			entry.Metadata["badge_id"] = p.BadgeID
			entry.Metadata["badge_name"] = p.BadgeName
			entry.Metadata["badge_icon"] = p.BadgeIcon
		}

	case event.ProgressLoggedPayload:
		entry.EntityType = "quest"
		entry.EntityID = p.QuestID
		entry.Metadata = map[string]any{
			"quest_name":     p.QuestName,
			"log_type":       p.LogType,
			"progress_count": p.ProgressCount,
		}
		if p.TargetCount != nil {
			entry.Metadata["target_count"] = *p.TargetCount
		}
		if p.RouteID != "" {
			entry.Metadata["route_id"] = p.RouteID
		}

	case event.BadgeEarnedPayload:
		entry.EntityType = "badge"
		entry.EntityID = p.BadgeID
		entry.Metadata = map[string]any{
			"badge_name":  p.BadgeName,
			"badge_icon":  p.BadgeIcon,
			"badge_color": p.BadgeColor,
			"quest_name":  p.QuestName,
		}

	default:
		slog.Warn("unhandled event type in activity logger", "type", e.Type)
		return nil
	}

	if err := l.activity.Insert(ctx, entry); err != nil {
		return fmt.Errorf("log activity for %s: %w", e.Type, err)
	}
	return nil
}

// ============================================================
// Notifications (async)
// ============================================================

func (l *QuestListeners) notifyQuestCompleted(ctx context.Context, e event.Event) error {
	payload, ok := e.Payload.(event.QuestCompletedPayload)
	if !ok {
		return nil
	}

	link := "/progressions"
	l.notifs.NotifyAsync(ctx, repository.Notification{ //nolint:errcheck
		UserID: e.UserID,
		Type:   "quest.completed",
		Title:  "Quest completed!",
		Body:   fmt.Sprintf("You completed \"%s\"", payload.QuestName),
		Link:   &link,
	})
	return nil
}

func (l *QuestListeners) notifyBadgeEarned(ctx context.Context, e event.Event) error {
	payload, ok := e.Payload.(event.BadgeEarnedPayload)
	if !ok {
		return nil
	}

	link := "/progressions/badges"
	l.notifs.NotifyAsync(ctx, repository.Notification{ //nolint:errcheck
		UserID: e.UserID,
		Type:   "badge.earned",
		Title:  fmt.Sprintf("Badge earned: %s", payload.BadgeName),
		Body:   fmt.Sprintf("You earned the %s badge by completing \"%s\"", payload.BadgeName, payload.QuestName),
		Link:   &link,
	})
	return nil
}
