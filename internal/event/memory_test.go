package event

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

func newTestBus() *memoryBus {
	return NewMemoryBus(slog.Default()).(*memoryBus)
}

func TestSyncHandler_Called(t *testing.T) {
	bus := newTestBus()
	var called bool
	bus.Subscribe("test.event", func(ctx context.Context, e Event) error {
		called = true
		return nil
	}, true)

	bus.Publish(context.Background(), Event{Type: "test.event"})

	if !called {
		t.Fatal("sync handler was not called")
	}
}

func TestSyncHandler_ReceivesRequestContext(t *testing.T) {
	bus := newTestBus()

	type ctxKey string
	key := ctxKey("test")

	var got string
	bus.Subscribe("test.event", func(ctx context.Context, e Event) error {
		got, _ = ctx.Value(key).(string)
		return nil
	}, true)

	ctx := context.WithValue(context.Background(), key, "hello")
	bus.Publish(ctx, Event{Type: "test.event"})

	if got != "hello" {
		t.Fatalf("sync handler did not receive request context: got %q, want %q", got, "hello")
	}
}

func TestSyncHandler_ErrorDoesNotPanic(t *testing.T) {
	bus := newTestBus()
	bus.Subscribe("test.event", func(ctx context.Context, e Event) error {
		return errors.New("handler failed")
	}, true)

	// Should not panic — error is logged, not propagated
	bus.Publish(context.Background(), Event{Type: "test.event"})
}

func TestAsyncHandler_Called(t *testing.T) {
	bus := newTestBus()
	ch := make(chan bool, 1)
	bus.Subscribe("test.event", func(ctx context.Context, e Event) error {
		ch <- true
		return nil
	}, false)

	bus.Publish(context.Background(), Event{Type: "test.event"})

	select {
	case <-ch:
		// success
	case <-time.After(time.Second):
		t.Fatal("async handler was not called within 1 second")
	}
}

func TestAsyncHandler_GetsBackgroundContext(t *testing.T) {
	bus := newTestBus()

	type ctxKey string
	key := ctxKey("test")

	ch := make(chan string, 1)
	bus.Subscribe("test.event", func(ctx context.Context, e Event) error {
		val, _ := ctx.Value(key).(string)
		ch <- val
		return nil
	}, false)

	ctx := context.WithValue(context.Background(), key, "should-not-see")
	bus.Publish(ctx, Event{Type: "test.event"})

	select {
	case got := <-ch:
		if got != "" {
			t.Fatalf("async handler should get background context, but got value %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("async handler was not called within 1 second")
	}
}

func TestAsyncHandler_ErrorDoesNotPanic(t *testing.T) {
	bus := newTestBus()
	ch := make(chan bool, 1)
	bus.Subscribe("test.event", func(ctx context.Context, e Event) error {
		ch <- true
		return errors.New("async handler failed")
	}, false)

	bus.Publish(context.Background(), Event{Type: "test.event"})

	select {
	case <-ch:
		// success — error was logged, not propagated
	case <-time.After(time.Second):
		t.Fatal("async handler was not called within 1 second")
	}
}

func TestMultipleHandlers_SameEvent(t *testing.T) {
	bus := newTestBus()
	var count atomic.Int32

	bus.Subscribe("test.event", func(ctx context.Context, e Event) error {
		count.Add(1)
		return nil
	}, true)
	bus.Subscribe("test.event", func(ctx context.Context, e Event) error {
		count.Add(1)
		return nil
	}, true)

	bus.Publish(context.Background(), Event{Type: "test.event"})

	if got := count.Load(); got != 2 {
		t.Fatalf("expected 2 handlers called, got %d", got)
	}
}

func TestMixedSyncAsync_SameEvent(t *testing.T) {
	bus := newTestBus()
	var syncCalled bool
	asyncCh := make(chan bool, 1)

	bus.Subscribe("test.event", func(ctx context.Context, e Event) error {
		syncCalled = true
		return nil
	}, true)
	bus.Subscribe("test.event", func(ctx context.Context, e Event) error {
		asyncCh <- true
		return nil
	}, false)

	bus.Publish(context.Background(), Event{Type: "test.event"})

	if !syncCalled {
		t.Fatal("sync handler was not called")
	}

	select {
	case <-asyncCh:
		// success
	case <-time.After(time.Second):
		t.Fatal("async handler was not called within 1 second")
	}
}

func TestNoSubscribers_DoesNotPanic(t *testing.T) {
	bus := newTestBus()
	// Should not panic when no handlers are registered
	bus.Publish(context.Background(), Event{Type: "unsubscribed.event"})
}

func TestEventPayload_TypeAssertion(t *testing.T) {
	bus := newTestBus()
	var got QuestCompletedPayload

	bus.Subscribe(QuestCompleted, func(ctx context.Context, e Event) error {
		p, ok := e.Payload.(QuestCompletedPayload)
		if !ok {
			t.Fatal("payload type assertion failed")
		}
		got = p
		return nil
	}, true)

	bus.Publish(context.Background(), Event{
		Type:   QuestCompleted,
		GymID:  "gym-123",
		UserID: "user-456",
		Payload: QuestCompletedPayload{
			ClimberQuestID: "cq-789",
			QuestID:        "q-001",
			QuestName:      "Breathe Through the Crux",
			DomainName:     "Breath & regulation",
			DomainColor:    "#7F77DD",
			BadgeID:        "badge-001",
			BadgeName:      "Breath Master",
			BadgeIcon:      "breath-master",
			BadgeColor:     "#7F77DD",
		},
		Timestamp: time.Now(),
	})

	if got.QuestName != "Breathe Through the Crux" {
		t.Fatalf("unexpected quest name: %q", got.QuestName)
	}
	if got.BadgeName != "Breath Master" {
		t.Fatalf("unexpected badge name: %q", got.BadgeName)
	}
}

func TestEventFields_PassedToHandler(t *testing.T) {
	bus := newTestBus()
	var got Event

	bus.Subscribe("test.event", func(ctx context.Context, e Event) error {
		got = e
		return nil
	}, true)

	now := time.Now()
	bus.Publish(context.Background(), Event{
		Type:      "test.event",
		GymID:     "gym-abc",
		UserID:    "user-xyz",
		Timestamp: now,
	})

	if got.GymID != "gym-abc" {
		t.Fatalf("GymID: got %q, want %q", got.GymID, "gym-abc")
	}
	if got.UserID != "user-xyz" {
		t.Fatalf("UserID: got %q, want %q", got.UserID, "user-xyz")
	}
	if !got.Timestamp.Equal(now) {
		t.Fatalf("Timestamp: got %v, want %v", got.Timestamp, now)
	}
}

func TestShutdown_WaitsForAsyncHandlers(t *testing.T) {
	bus := newTestBus()
	var completed atomic.Bool

	bus.Subscribe("test.event", func(ctx context.Context, e Event) error {
		time.Sleep(50 * time.Millisecond)
		completed.Store(true)
		return nil
	}, false)

	bus.Publish(context.Background(), Event{Type: "test.event"})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := bus.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}

	if !completed.Load() {
		t.Fatal("async handler did not complete before Shutdown returned")
	}
}

func TestShutdown_RespectsContextDeadline(t *testing.T) {
	bus := newTestBus()

	bus.Subscribe("test.event", func(ctx context.Context, e Event) error {
		time.Sleep(5 * time.Second) // intentionally slow
		return nil
	}, false)

	bus.Publish(context.Background(), Event{Type: "test.event"})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := bus.Shutdown(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got: %v", err)
	}
}

func TestShutdown_NoHandlers_ReturnsImmediately(t *testing.T) {
	bus := newTestBus()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := bus.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown with no handlers returned error: %v", err)
	}
}

func TestSubscribe_DifferentEvents_Independent(t *testing.T) {
	bus := newTestBus()
	var aCalled, bCalled bool

	bus.Subscribe("event.a", func(ctx context.Context, e Event) error {
		aCalled = true
		return nil
	}, true)
	bus.Subscribe("event.b", func(ctx context.Context, e Event) error {
		bCalled = true
		return nil
	}, true)

	bus.Publish(context.Background(), Event{Type: "event.a"})

	if !aCalled {
		t.Fatal("handler for event.a was not called")
	}
	if bCalled {
		t.Fatal("handler for event.b was called when only event.a was published")
	}
}

func TestConcurrentPublish(t *testing.T) {
	bus := newTestBus()
	var count atomic.Int32

	bus.Subscribe("test.event", func(ctx context.Context, e Event) error {
		count.Add(1)
		return nil
	}, true)

	// Publish from multiple goroutines concurrently
	done := make(chan struct{})
	for i := 0; i < 100; i++ {
		go func() {
			bus.Publish(context.Background(), Event{Type: "test.event"})
			done <- struct{}{}
		}()
	}

	for i := 0; i < 100; i++ {
		<-done
	}

	if got := count.Load(); got != 100 {
		t.Fatalf("expected 100 handler calls, got %d", got)
	}
}
