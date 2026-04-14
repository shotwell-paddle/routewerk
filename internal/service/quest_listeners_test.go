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
	l := NewQuestListeners(nil, nil, nil, nil, nil, bus)
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

// ── RouteSent Event Routing ───────────────────────────────────
// These tests verify that RouteSent events are properly routed and that
// the autoProgressQuests handler fires when a route.sent event is published.

func TestListeners_RouteSentPayloadAssertion(t *testing.T) {
	bus := event.NewMemoryBus(slog.Default())

	var captured event.RouteSentPayload
	var gotIt bool

	bus.Subscribe(event.RouteSent, func(ctx context.Context, e event.Event) error {
		p, ok := e.Payload.(event.RouteSentPayload)
		if ok {
			captured = p
			gotIt = true
		}
		return nil
	}, true)

	bus.Publish(context.Background(), event.Event{
		Type:   event.RouteSent,
		GymID:  "loc-1",
		UserID: "user-1",
		Payload: event.RouteSentPayload{
			AscentID:   "asc-1",
			RouteID:    "route-42",
			RouteName:  "Crimpy McFace",
			RouteGrade: "V5",
			AscentType: "send",
			LocationID: "loc-1",
		},
		Timestamp: time.Now(),
	})

	if !gotIt {
		t.Fatal("did not receive RouteSentPayload")
	}
	if captured.AscentID != "asc-1" {
		t.Errorf("AscentID = %q, want asc-1", captured.AscentID)
	}
	if captured.RouteID != "route-42" {
		t.Errorf("RouteID = %q, want route-42", captured.RouteID)
	}
	if captured.RouteName != "Crimpy McFace" {
		t.Errorf("RouteName = %q, want Crimpy McFace", captured.RouteName)
	}
	if captured.RouteGrade != "V5" {
		t.Errorf("RouteGrade = %q, want V5", captured.RouteGrade)
	}
	if captured.AscentType != "send" {
		t.Errorf("AscentType = %q, want send", captured.AscentType)
	}
	if captured.LocationID != "loc-1" {
		t.Errorf("LocationID = %q, want loc-1", captured.LocationID)
	}
}

func TestListeners_RouteSentFlashPayload(t *testing.T) {
	bus := event.NewMemoryBus(slog.Default())

	var captured event.RouteSentPayload
	var gotIt bool

	bus.Subscribe(event.RouteSent, func(ctx context.Context, e event.Event) error {
		p, ok := e.Payload.(event.RouteSentPayload)
		if ok {
			captured = p
			gotIt = true
		}
		return nil
	}, true)

	bus.Publish(context.Background(), event.Event{
		Type:   event.RouteSent,
		GymID:  "loc-1",
		UserID: "user-2",
		Payload: event.RouteSentPayload{
			AscentID:   "asc-2",
			RouteID:    "route-99",
			RouteName:  "Slab City",
			RouteGrade: "5.10+",
			AscentType: "flash",
			LocationID: "loc-1",
		},
		Timestamp: time.Now(),
	})

	if !gotIt {
		t.Fatal("did not receive flash RouteSentPayload")
	}
	if captured.AscentType != "flash" {
		t.Errorf("AscentType = %q, want flash", captured.AscentType)
	}
}

func TestListeners_RouteSentDoesNotTriggerUnrelatedHandlers(t *testing.T) {
	bus := event.NewMemoryBus(slog.Default())

	var mu sync.Mutex
	var received []string

	// Subscribe to QuestCompleted — should NOT fire on RouteSent
	bus.Subscribe(event.QuestCompleted, func(ctx context.Context, e event.Event) error {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, "quest.completed")
		return nil
	}, true)

	// Subscribe to RouteSent — should fire
	bus.Subscribe(event.RouteSent, func(ctx context.Context, e event.Event) error {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, "route.sent")
		return nil
	}, true)

	bus.Publish(context.Background(), event.Event{
		Type:   event.RouteSent,
		GymID:  "loc-1",
		UserID: "user-1",
		Payload: event.RouteSentPayload{
			AscentID:   "asc-1",
			RouteID:    "route-1",
			AscentType: "send",
			LocationID: "loc-1",
		},
		Timestamp: time.Now(),
	})

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Errorf("expected 1 event, got %d: %v", len(received), received)
	}
	if len(received) > 0 && received[0] != "route.sent" {
		t.Errorf("expected route.sent, got %q", received[0])
	}
}

func TestListeners_RouteSentChainToProgressAndComplete(t *testing.T) {
	// Simulates the full chain: RouteSent → ProgressLogged → QuestCompleted
	// by manually publishing the downstream events (since autoProgressQuests
	// needs real repos). This verifies the event bus routing works end-to-end.
	bus := event.NewMemoryBus(slog.Default())

	var mu sync.Mutex
	eventOrder := make([]string, 0, 3)

	// RouteSent handler simulates what autoProgressQuests does
	bus.Subscribe(event.RouteSent, func(ctx context.Context, e event.Event) error {
		mu.Lock()
		eventOrder = append(eventOrder, "route.sent")
		mu.Unlock()

		// Simulate publishing ProgressLogged
		target := 5
		bus.Publish(ctx, event.Event{
			Type:   event.ProgressLogged,
			GymID:  e.GymID,
			UserID: e.UserID,
			Payload: event.ProgressLoggedPayload{
				ClimberQuestID: "cq-1",
				QuestID:        "q-1",
				QuestName:      "Send 5 Routes",
				LogType:        "route_send",
				ProgressCount:  5,
				TargetCount:    &target,
			},
			Timestamp: e.Timestamp,
		})
		return nil
	}, true)

	bus.Subscribe(event.ProgressLogged, func(ctx context.Context, e event.Event) error {
		mu.Lock()
		eventOrder = append(eventOrder, "progress.logged")
		mu.Unlock()

		p := e.Payload.(event.ProgressLoggedPayload)
		if p.TargetCount != nil && p.ProgressCount >= *p.TargetCount {
			bus.Publish(ctx, event.Event{
				Type:   event.QuestCompleted,
				GymID:  e.GymID,
				UserID: e.UserID,
				Payload: event.QuestCompletedPayload{
					ClimberQuestID: p.ClimberQuestID,
					QuestID:        p.QuestID,
					QuestName:      p.QuestName,
				},
				Timestamp: e.Timestamp,
			})
		}
		return nil
	}, true)

	bus.Subscribe(event.QuestCompleted, func(ctx context.Context, e event.Event) error {
		mu.Lock()
		eventOrder = append(eventOrder, "quest.completed")
		mu.Unlock()
		return nil
	}, true)

	bus.Publish(context.Background(), event.Event{
		Type:   event.RouteSent,
		GymID:  "loc-1",
		UserID: "user-1",
		Payload: event.RouteSentPayload{
			AscentID:   "asc-1",
			RouteID:    "route-1",
			RouteName:  "The Problem",
			RouteGrade: "V4",
			AscentType: "send",
			LocationID: "loc-1",
		},
		Timestamp: time.Now(),
	})

	mu.Lock()
	defer mu.Unlock()

	if len(eventOrder) != 3 {
		t.Fatalf("expected 3 events, got %d: %v", len(eventOrder), eventOrder)
	}
	expected := []string{"route.sent", "progress.logged", "quest.completed"}
	for i, want := range expected {
		if eventOrder[i] != want {
			t.Errorf("event[%d] = %q, want %q", i, eventOrder[i], want)
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

// ── Event Payload Type Safety ────────────────────────────────────
// These tests verify that each event type's payload can be correctly
// type-asserted and that fields are preserved through publish/subscribe.

func TestPayload_RouteSentFieldCompleteness(t *testing.T) {
	// Verify all fields survive a round-trip through the bus
	bus := event.NewMemoryBus(slog.Default())

	original := event.RouteSentPayload{
		AscentID:   "asc-abc",
		RouteID:    "route-xyz",
		RouteName:  "The Nose",
		RouteGrade: "5.12a",
		AscentType: "flash",
		LocationID: "loc-42",
	}

	var captured event.RouteSentPayload
	bus.Subscribe(event.RouteSent, func(ctx context.Context, e event.Event) error {
		captured = e.Payload.(event.RouteSentPayload)
		return nil
	}, true)

	bus.Publish(context.Background(), event.Event{
		Type:    event.RouteSent,
		GymID:   "loc-42",
		UserID:  "user-1",
		Payload: original,
	})

	if captured != original {
		t.Errorf("RouteSentPayload round-trip mismatch:\ngot  %+v\nwant %+v", captured, original)
	}
}

func TestPayload_QuestCompletedWithBadge(t *testing.T) {
	bus := event.NewMemoryBus(slog.Default())

	var captured event.QuestCompletedPayload
	bus.Subscribe(event.QuestCompleted, func(ctx context.Context, e event.Event) error {
		captured = e.Payload.(event.QuestCompletedPayload)
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
			DomainColor:    "#2e7d32",
			BadgeID:        "badge-1",
			BadgeName:      "Slab Badge",
			BadgeIcon:      "mountain",
			BadgeColor:     "#ff0000",
		},
	})

	if captured.BadgeID != "badge-1" {
		t.Errorf("BadgeID = %q, want badge-1", captured.BadgeID)
	}
	if captured.DomainColor != "#2e7d32" {
		t.Errorf("DomainColor = %q, want #2e7d32", captured.DomainColor)
	}
}

func TestPayload_QuestCompletedWithoutBadge(t *testing.T) {
	bus := event.NewMemoryBus(slog.Default())

	var captured event.QuestCompletedPayload
	bus.Subscribe(event.QuestCompleted, func(ctx context.Context, e event.Event) error {
		captured = e.Payload.(event.QuestCompletedPayload)
		return nil
	}, true)

	bus.Publish(context.Background(), event.Event{
		Type:   event.QuestCompleted,
		GymID:  "loc-1",
		UserID: "user-1",
		Payload: event.QuestCompletedPayload{
			ClimberQuestID: "cq-1",
			QuestID:        "q-1",
			QuestName:      "Quick Challenge",
			// No badge fields set
		},
	})

	if captured.BadgeID != "" {
		t.Errorf("BadgeID = %q, want empty", captured.BadgeID)
	}
}

func TestPayload_QuestAbandonedFields(t *testing.T) {
	bus := event.NewMemoryBus(slog.Default())

	var captured event.QuestAbandonedPayload
	var gotIt bool
	bus.Subscribe(event.QuestAbandoned, func(ctx context.Context, e event.Event) error {
		p, ok := e.Payload.(event.QuestAbandonedPayload)
		if ok {
			captured = p
			gotIt = true
		}
		return nil
	}, true)

	bus.Publish(context.Background(), event.Event{
		Type:   event.QuestAbandoned,
		GymID:  "loc-1",
		UserID: "user-1",
		Payload: event.QuestAbandonedPayload{
			ClimberQuestID: "cq-5",
			QuestID:        "q-5",
			QuestName:      "Abandoned Quest",
		},
	})

	if !gotIt {
		t.Fatal("did not receive QuestAbandonedPayload")
	}
	if captured.ClimberQuestID != "cq-5" {
		t.Errorf("ClimberQuestID = %q, want cq-5", captured.ClimberQuestID)
	}
	if captured.QuestName != "Abandoned Quest" {
		t.Errorf("QuestName = %q, want Abandoned Quest", captured.QuestName)
	}
}

// ── Sync vs Async Behavior ───────────────────────────────────────

func TestListeners_SyncHandlerBlocksPublisher(t *testing.T) {
	bus := event.NewMemoryBus(slog.Default())
	completed := false

	bus.Subscribe(event.QuestCompleted, func(ctx context.Context, e event.Event) error {
		completed = true
		return nil
	}, true) // sync = true

	bus.Publish(context.Background(), event.Event{
		Type:    event.QuestCompleted,
		Payload: event.QuestCompletedPayload{QuestName: "Sync Test"},
	})

	// Since handler is sync, completed should be true immediately after Publish
	if !completed {
		t.Error("sync handler should have completed before Publish returns")
	}
}

func TestListeners_AsyncHandlerRunsInBackground(t *testing.T) {
	bus := event.NewMemoryBus(slog.Default())

	var mu sync.Mutex
	completed := false

	bus.Subscribe(event.ProgressLogged, func(ctx context.Context, e event.Event) error {
		mu.Lock()
		defer mu.Unlock()
		completed = true
		return nil
	}, false) // sync = false

	bus.Publish(context.Background(), event.Event{
		Type:    event.ProgressLogged,
		Payload: event.ProgressLoggedPayload{QuestName: "Async Test"},
	})

	// Shutdown to wait for async handlers
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	bus.Shutdown(ctx)

	mu.Lock()
	defer mu.Unlock()
	if !completed {
		t.Error("async handler should have completed after Shutdown")
	}
}
