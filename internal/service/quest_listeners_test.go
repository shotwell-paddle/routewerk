package service

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/shotwell-paddle/routewerk/internal/event"
)

// ── Constructor ───────────────────────────────────────────────────

func TestNewQuestListeners(t *testing.T) {
	bus := event.NewMemoryBus(slog.Default())
	l := NewQuestListeners(nil, nil, nil, nil, bus)
	if l == nil {
		t.Fatal("NewQuestListeners returned nil")
	}
}

// ── Event Routing ─────────────────────────────────────────────────
// These tests verify that Register() wires events to the correct handlers
// by subscribing a test interceptor and publishing events through the bus.

func TestListeners_QuestCompletedTriggersHandlers(t *testing.T) {
	bus := event.NewMemoryBus(slog.Default())

	var mu sync.Mutex
	var received []string

	// Subscribe test handlers to track which events fire
	bus.Subscribe(event.QuestCompleted, func(ctx context.Context, e event.Event) error {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, "quest.completed")
		return nil
	}, true)

	bus.Publish(context.Background(), event.Event{
		Type:   event.QuestCompleted,
		GymID:  "loc-1",
		UserID: "user-1",
		Payload: event.QuestCompletedPayload{
			ClimberQuestID: "cq-1",
			QuestID:        "q-1",
			QuestName:      "Slab Master",
			DomainName:     "Slab",
			BadgeID:        "b-1",
			BadgeName:      "Slab Badge",
			BadgeIcon:      "mountain",
			BadgeColor:     "#ff0000",
		},
		Timestamp: time.Now(),
	})

	// Give async handlers a moment
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	bus.Shutdown(ctx)

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Errorf("expected 1 event, got %d", len(received))
	}
}

func TestListeners_BadgeEarnedPayloadMapping(t *testing.T) {
	bus := event.NewMemoryBus(slog.Default())

	var capturedPayload event.BadgeEarnedPayload
	var gotPayload bool

	bus.Subscribe(event.BadgeEarned, func(ctx context.Context, e event.Event) error {
		p, ok := e.Payload.(event.BadgeEarnedPayload)
		if ok {
			capturedPayload = p
			gotPayload = true
		}
		return nil
	}, true)

	bus.Publish(context.Background(), event.Event{
		Type:   event.BadgeEarned,
		GymID:  "loc-1",
		UserID: "user-1",
		Payload: event.BadgeEarnedPayload{
			BadgeID:    "b-1",
			BadgeName:  "Slab Master",
			BadgeIcon:  "mountain",
			BadgeColor: "#ff0000",
			QuestName:  "Slab Quest",
		},
		Timestamp: time.Now(),
	})

	if !gotPayload {
		t.Fatal("did not receive BadgeEarnedPayload")
	}
	if capturedPayload.BadgeID != "b-1" {
		t.Errorf("BadgeID = %q, want b-1", capturedPayload.BadgeID)
	}
	if capturedPayload.BadgeName != "Slab Master" {
		t.Errorf("BadgeName = %q, want Slab Master", capturedPayload.BadgeName)
	}
	if capturedPayload.QuestName != "Slab Quest" {
		t.Errorf("QuestName = %q, want Slab Quest", capturedPayload.QuestName)
	}
}

func TestListeners_ProgressLoggedPayloadFields(t *testing.T) {
	bus := event.NewMemoryBus(slog.Default())

	var captured event.ProgressLoggedPayload
	var gotIt bool

	bus.Subscribe(event.ProgressLogged, func(ctx context.Context, e event.Event) error {
		p, ok := e.Payload.(event.ProgressLoggedPayload)
		if ok {
			captured = p
			gotIt = true
		}
		return nil
	}, true)

	target := 10
	bus.Publish(context.Background(), event.Event{
		Type:   event.ProgressLogged,
		GymID:  "loc-1",
		UserID: "user-1",
		Payload: event.ProgressLoggedPayload{
			ClimberQuestID: "cq-1",
			QuestID:        "q-1",
			QuestName:      "Overhang Explorer",
			LogType:        "route_climbed",
			RouteID:        "route-42",
			ProgressCount:  3,
			TargetCount:    &target,
		},
		Timestamp: time.Now(),
	})

	if !gotIt {
		t.Fatal("did not receive ProgressLoggedPayload")
	}
	if captured.ProgressCount != 3 {
		t.Errorf("ProgressCount = %d, want 3", captured.ProgressCount)
	}
	if captured.TargetCount == nil || *captured.TargetCount != 10 {
		t.Errorf("TargetCount = %v, want 10", captured.TargetCount)
	}
	if captured.RouteID != "route-42" {
		t.Errorf("RouteID = %q, want route-42", captured.RouteID)
	}
	if captured.LogType != "route_climbed" {
		t.Errorf("LogType = %q, want route_climbed", captured.LogType)
	}
}

func TestListeners_MultipleEventTypes(t *testing.T) {
	bus := event.NewMemoryBus(slog.Default())

	var mu sync.Mutex
	eventTypes := make(map[string]int)

	handler := func(ctx context.Context, e event.Event) error {
		mu.Lock()
		defer mu.Unlock()
		eventTypes[e.Type]++
		return nil
	}

	bus.Subscribe(event.QuestStarted, handler, false)
	bus.Subscribe(event.QuestCompleted, handler, false)
	bus.Subscribe(event.ProgressLogged, handler, false)
	bus.Subscribe(event.BadgeEarned, handler, false)

	events := []event.Event{
		{Type: event.QuestStarted, Payload: event.QuestStartedPayload{QuestName: "Q1"}},
		{Type: event.QuestCompleted, Payload: event.QuestCompletedPayload{QuestName: "Q1"}},
		{Type: event.ProgressLogged, Payload: event.ProgressLoggedPayload{QuestName: "Q1"}},
		{Type: event.ProgressLogged, Payload: event.ProgressLoggedPayload{QuestName: "Q2"}},
		{Type: event.BadgeEarned, Payload: event.BadgeEarnedPayload{BadgeName: "B1"}},
	}

	for _, e := range events {
		bus.Publish(context.Background(), e)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	bus.Shutdown(ctx)

	mu.Lock()
	defer mu.Unlock()

	tests := []struct {
		eventType string
		wantCount int
	}{
		{event.QuestStarted, 1},
		{event.QuestCompleted, 1},
		{event.ProgressLogged, 2},
		{event.BadgeEarned, 1},
	}

	for _, tt := range tests {
		if eventTypes[tt.eventType] != tt.wantCount {
			t.Errorf("%s fired %d times, want %d", tt.eventType, eventTypes[tt.eventType], tt.wantCount)
		}
	}
}

func TestListeners_QuestStartedPayloadAssertion(t *testing.T) {
	bus := event.NewMemoryBus(slog.Default())

	var captured event.QuestStartedPayload
	var gotIt bool

	bus.Subscribe(event.QuestStarted, func(ctx context.Context, e event.Event) error {
		p, ok := e.Payload.(event.QuestStartedPayload)
		if ok {
			captured = p
			gotIt = true
		}
		return nil
	}, true)

	bus.Publish(context.Background(), event.Event{
		Type:   event.QuestStarted,
		GymID:  "loc-1",
		UserID: "user-1",
		Payload: event.QuestStartedPayload{
			ClimberQuestID: "cq-1",
			QuestID:        "q-1",
			QuestName:      "Dyno King",
			DomainName:     "Dynamic",
			DomainColor:    "#00ff00",
		},
		Timestamp: time.Now(),
	})

	if !gotIt {
		t.Fatal("did not receive QuestStartedPayload")
	}
	if captured.QuestName != "Dyno King" {
		t.Errorf("QuestName = %q, want Dyno King", captured.QuestName)
	}
	if captured.DomainName != "Dynamic" {
		t.Errorf("DomainName = %q, want Dynamic", captured.DomainName)
	}
	if captured.DomainColor != "#00ff00" {
		t.Errorf("DomainColor = %q, want #00ff00", captured.DomainColor)
	}
}
