package event

import (
	"context"
	"time"
)

// Event is the base type for all domain events.
type Event struct {
	Type      string
	GymID     string // location/gym ID
	UserID    string // the actor who triggered this
	Payload   any    // typed payload, cast by the listener
	Timestamp time.Time
}

// Handler processes an event. Return an error to log the failure;
// the bus does NOT retry (keep it simple, add retry later if needed).
type Handler func(ctx context.Context, e Event) error

// Bus publishes events and dispatches them to registered handlers.
type Bus interface {
	// Publish sends an event to all registered handlers.
	// Sync handlers run in the caller's goroutine (same request).
	// Async handlers run in a background goroutine.
	Publish(ctx context.Context, e Event)

	// Subscribe registers a handler for an event type.
	// sync=true means the handler runs in the publisher's goroutine
	// and blocks the request until complete. Use for critical side
	// effects that must succeed (e.g., badge award).
	// sync=false means the handler runs in a background goroutine.
	// Use for non-critical side effects (e.g., activity log, notifications).
	Subscribe(eventType string, handler Handler, sync bool)

	// Shutdown waits for all in-flight async handlers to complete,
	// respecting the provided context's deadline. Call this during
	// graceful server shutdown to avoid losing events.
	Shutdown(ctx context.Context) error
}
