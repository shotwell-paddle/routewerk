package event

import (
	"context"
	"log/slog"
	"sync"
)

type subscription struct {
	handler Handler
	sync    bool
}

type memoryBus struct {
	mu          sync.RWMutex
	subscribers map[string][]subscription
	wg          sync.WaitGroup
	logger      *slog.Logger
}

// NewMemoryBus creates an in-memory event bus. At gym scale, goroutines
// are sufficient for async dispatch. If durability becomes important,
// swap this for a Postgres-backed implementation without changing any
// publishers or subscribers.
func NewMemoryBus(logger *slog.Logger) Bus {
	return &memoryBus{
		subscribers: make(map[string][]subscription),
		logger:      logger,
	}
}

func (b *memoryBus) Subscribe(eventType string, handler Handler, sync bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers[eventType] = append(b.subscribers[eventType], subscription{
		handler: handler,
		sync:    sync,
	})
}

func (b *memoryBus) Publish(ctx context.Context, e Event) {
	b.mu.RLock()
	subs := make([]subscription, len(b.subscribers[e.Type]))
	copy(subs, b.subscribers[e.Type])
	b.mu.RUnlock()

	for _, sub := range subs {
		if sub.sync {
			if err := sub.handler(ctx, e); err != nil {
				b.logger.Error("sync event handler failed",
					"event", e.Type,
					"error", err,
				)
			}
		} else {
			b.wg.Add(1)
			go func(h Handler) {
				defer b.wg.Done()
				// Async handlers get a background context — they
				// must not depend on the HTTP request context.
				bgCtx := context.Background()
				if err := h(bgCtx, e); err != nil {
					b.logger.Error("async event handler failed",
						"event", e.Type,
						"error", err,
					)
				}
			}(sub.handler)
		}
	}
}

// Shutdown waits for all in-flight async handlers to finish. It
// respects the context deadline so the server doesn't hang forever
// if a handler is stuck.
func (b *memoryBus) Shutdown(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		b.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
