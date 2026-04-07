package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/shotwell-paddle/routewerk/internal/event"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

var (
	ErrQuestNotFound       = errors.New("quest not found")
	ErrQuestNotActive      = errors.New("quest is not currently active")
	ErrQuestNotAvailable   = errors.New("quest is not within its availability window")
	ErrAlreadyEnrolled     = errors.New("already enrolled in this quest")
	ErrClimberQuestNotFound = errors.New("climber quest not found")
	ErrNotOwner            = errors.New("you do not own this quest enrollment")
)

// QuestService orchestrates the quest lifecycle: enrollment, progress
// tracking, completion, and abandonment. It publishes events on the
// bus so that listeners can handle badge awards, activity logging, and
// notifications without the service needing to know about those systems.
type QuestService struct {
	quests *repository.QuestRepo
	badges *repository.BadgeRepo
	bus    event.Bus
}

func NewQuestService(quests *repository.QuestRepo, badges *repository.BadgeRepo, bus event.Bus) *QuestService {
	return &QuestService{quests: quests, badges: badges, bus: bus}
}

// ============================================================
// Quest Browsing
// ============================================================

// ListAvailable returns active, in-window quests for the climber browser.
func (s *QuestService) ListAvailable(ctx context.Context, locationID string) ([]repository.QuestListItem, error) {
	return s.quests.ListAvailable(ctx, locationID)
}

// ListByLocation returns all quests for admin views.
func (s *QuestService) ListByLocation(ctx context.Context, locationID string) ([]repository.QuestListItem, error) {
	return s.quests.ListByLocation(ctx, locationID)
}

// GetQuest returns a single quest with its domain and badge joined.
func (s *QuestService) GetQuest(ctx context.Context, id string) (*model.Quest, error) {
	q, err := s.quests.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, ErrQuestNotFound
	}
	return q, nil
}

// ============================================================
// Quest Enrollment
// ============================================================

// StartQuest enrolls a climber in a quest. Validates that the quest is
// active and available, then publishes a QuestStarted event.
func (s *QuestService) StartQuest(ctx context.Context, userID, questID, locationID string) (*model.ClimberQuest, error) {
	// Load and validate
	quest, err := s.quests.GetByID(ctx, questID)
	if err != nil {
		return nil, err
	}
	if quest == nil {
		return nil, ErrQuestNotFound
	}
	if quest.LocationID != locationID {
		return nil, ErrQuestNotFound // don't leak existence across locations
	}
	if !quest.IsActive {
		return nil, ErrQuestNotActive
	}
	now := time.Now()
	if quest.AvailableFrom != nil && now.Before(*quest.AvailableFrom) {
		return nil, ErrQuestNotAvailable
	}
	if quest.AvailableUntil != nil && now.After(*quest.AvailableUntil) {
		return nil, ErrQuestNotAvailable
	}

	// Check for existing active enrollment
	existing, err := s.quests.ListUserQuests(ctx, userID, "active")
	if err != nil {
		return nil, err
	}
	for _, cq := range existing {
		if cq.QuestID == questID {
			return nil, ErrAlreadyEnrolled
		}
	}

	// Enroll
	cq, err := s.quests.StartQuest(ctx, userID, questID)
	if err != nil {
		return nil, err
	}

	// Publish event
	domainName := ""
	domainColor := ""
	if quest.Domain != nil {
		domainName = quest.Domain.Name
		if quest.Domain.Color != nil {
			domainColor = *quest.Domain.Color
		}
	}
	s.bus.Publish(ctx, event.Event{
		Type:      event.QuestStarted,
		GymID:     locationID,
		UserID:    userID,
		Payload: event.QuestStartedPayload{
			ClimberQuestID: cq.ID,
			QuestID:        questID,
			QuestName:      quest.Name,
			DomainName:     domainName,
			DomainColor:    domainColor,
		},
		Timestamp: time.Now(),
	})

	cq.Quest = quest
	return cq, nil
}

// ============================================================
// Progress Logging
// ============================================================

// LogProgress records a quest log entry, increments the progress counter,
// publishes ProgressLogged, and auto-completes if target is reached.
func (s *QuestService) LogProgress(ctx context.Context, userID string, climberQuestID string, log *model.QuestLog) error {
	// Verify ownership
	cq, err := s.quests.GetClimberQuest(ctx, climberQuestID)
	if err != nil {
		return err
	}
	if cq == nil {
		return ErrClimberQuestNotFound
	}
	if cq.UserID != userID {
		return ErrNotOwner
	}
	if cq.Status != "active" {
		return fmt.Errorf("quest enrollment is %s, not active", cq.Status)
	}

	// Load quest for metadata
	quest, err := s.quests.GetByID(ctx, cq.QuestID)
	if err != nil {
		return err
	}

	// Write log entry
	log.ClimberQuestID = climberQuestID
	if err := s.quests.LogProgress(ctx, log); err != nil {
		return err
	}

	// Increment count
	newCount, err := s.quests.IncrementProgress(ctx, climberQuestID)
	if err != nil {
		return err
	}

	locationID := ""
	questName := ""
	if quest != nil {
		locationID = quest.LocationID
		questName = quest.Name
	}

	s.bus.Publish(ctx, event.Event{
		Type:   event.ProgressLogged,
		GymID:  locationID,
		UserID: userID,
		Payload: event.ProgressLoggedPayload{
			ClimberQuestID: climberQuestID,
			QuestID:        cq.QuestID,
			QuestName:      questName,
			LogType:        log.LogType,
			RouteID:        derefStr(log.RouteID),
			ProgressCount:  newCount,
			TargetCount:    quest.TargetCount,
		},
		Timestamp: time.Now(),
	})

	// Auto-complete if target reached
	if quest != nil && quest.TargetCount != nil && newCount >= *quest.TargetCount {
		slog.Info("quest auto-completing",
			"climber_quest_id", climberQuestID,
			"count", newCount,
			"target", *quest.TargetCount,
		)
		if err := s.completeQuest(ctx, userID, climberQuestID, quest); err != nil {
			slog.Error("auto-complete failed", "error", err)
			// Don't fail the whole log — the progress was already recorded.
		}
	}

	return nil
}

// ============================================================
// Completion & Abandonment
// ============================================================

// CompleteQuest manually completes a quest (for criteria that can't be
// auto-detected, like "try 5 different slab techniques").
func (s *QuestService) CompleteQuest(ctx context.Context, userID, climberQuestID string) error {
	cq, err := s.quests.GetClimberQuest(ctx, climberQuestID)
	if err != nil {
		return err
	}
	if cq == nil {
		return ErrClimberQuestNotFound
	}
	if cq.UserID != userID {
		return ErrNotOwner
	}

	quest, err := s.quests.GetByID(ctx, cq.QuestID)
	if err != nil {
		return err
	}

	return s.completeQuest(ctx, userID, climberQuestID, quest)
}

func (s *QuestService) completeQuest(ctx context.Context, userID, climberQuestID string, quest *model.Quest) error {
	_, err := s.quests.CompleteQuest(ctx, climberQuestID)
	if err != nil {
		return err
	}

	// Build payload with badge info
	payload := event.QuestCompletedPayload{
		ClimberQuestID: climberQuestID,
		QuestID:        quest.ID,
		QuestName:      quest.Name,
	}
	if quest.Domain != nil {
		payload.DomainName = quest.Domain.Name
		if quest.Domain.Color != nil {
			payload.DomainColor = *quest.Domain.Color
		}
	}
	if quest.Badge != nil {
		payload.BadgeID = quest.Badge.ID
		payload.BadgeName = quest.Badge.Name
		payload.BadgeIcon = quest.Badge.Icon
		payload.BadgeColor = quest.Badge.Color
	}

	s.bus.Publish(ctx, event.Event{
		Type:      event.QuestCompleted,
		GymID:     quest.LocationID,
		UserID:    userID,
		Payload:   payload,
		Timestamp: time.Now(),
	})

	return nil
}

// AbandonQuest lets a climber drop out of a quest.
func (s *QuestService) AbandonQuest(ctx context.Context, userID, climberQuestID string) error {
	cq, err := s.quests.GetClimberQuest(ctx, climberQuestID)
	if err != nil {
		return err
	}
	if cq == nil {
		return ErrClimberQuestNotFound
	}
	if cq.UserID != userID {
		return ErrNotOwner
	}

	quest, err := s.quests.GetByID(ctx, cq.QuestID)
	if err != nil {
		return err
	}

	if err := s.quests.AbandonQuest(ctx, climberQuestID); err != nil {
		return err
	}

	questName := ""
	locationID := ""
	if quest != nil {
		questName = quest.Name
		locationID = quest.LocationID
	}

	s.bus.Publish(ctx, event.Event{
		Type:   event.QuestAbandoned,
		GymID:  locationID,
		UserID: userID,
		Payload: event.QuestAbandonedPayload{
			ClimberQuestID: climberQuestID,
			QuestID:        cq.QuestID,
			QuestName:      questName,
		},
		Timestamp: time.Now(),
	})

	return nil
}

// ============================================================
// Climber Quest Queries
// ============================================================

// ListUserQuests returns a climber's quests, optionally filtered by status.
func (s *QuestService) ListUserQuests(ctx context.Context, userID, status string) ([]model.ClimberQuest, error) {
	return s.quests.ListUserQuests(ctx, userID, status)
}

// ListLogs returns progress logs for a climber quest enrollment.
func (s *QuestService) ListLogs(ctx context.Context, userID, climberQuestID string) ([]model.QuestLog, error) {
	cq, err := s.quests.GetClimberQuest(ctx, climberQuestID)
	if err != nil {
		return nil, err
	}
	if cq == nil {
		return nil, ErrClimberQuestNotFound
	}
	if cq.UserID != userID {
		return nil, ErrNotOwner
	}
	return s.quests.ListLogs(ctx, climberQuestID)
}

// UserDomainProgress returns per-domain completion counts for the radar chart.
func (s *QuestService) UserDomainProgress(ctx context.Context, userID, locationID string) ([]repository.DomainProgress, error) {
	return s.quests.UserDomainProgress(ctx, userID, locationID)
}

// ============================================================
// Helpers
// ============================================================

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
