package event

import (
	"context"
	"log/slog"
	"sync"
)

const (
	// eventWorkerCount caps the number of goroutines simultaneously running
	// async event handlers. Under burst publishes (e.g. a bulk import
	// enqueuing many activity events), this keeps the goroutine count
	// bounded instead of fanning out one-per-event. 8 is enough to keep
	// latency low for handlers that do a little DB work and then block on
	// I/O, while staying well under the pgxpool MaxConns=5 plus other
	// goroutines on the 256 MB tier. See perf audit 2026-04-22 #13.
	eventWorkerCount = 8

	// eventQueueSize is the task-channel buffer. Tasks are small closures
	// (~tens of bytes plus captured values), so 256 slots cost a few KB
	// of steady-state RSS. When the queue is full, Publish falls back to
	// spawning an overflow goroutine so publishers never block — see the
	// select in Publish below.
	eventQueueSize = 256
)

type subscription struct {
	handler Handler
	sync    bool
}

type memoryBus struct {
	mu          sync.RWMutex
	subscribers map[string][]subscription
	wg          sync.WaitGroup
	tasks       chan func()
	workerWg    sync.WaitGroup
	logger      *slog.Logger
}

// NewMemoryBus creates an in-memory event bus backed by a bounded worker
// pool. At gym scale, goroutines are sufficient for async dispatch; the
// worker pool caps peak goroutine count under bursts without adding a
// durability story. If durability becomes important, swap this for a
// Postgres-backed implementation without changing any publishers or
// subscribers.
func NewMemoryBus(logger *slog.Logger) Bus {
	b := &memoryBus{
		subscribers: make(map[string][]subscription),
		tasks:       make(chan func(), eventQueueSize),
		logger:      logger,
	}
	for i := 0; i < eventWorkerCount; i++ {
		b.workerWg.Add(1)
		go b.worker()
	}
	return b
}

// worker drains tasks from the queue until the bus is shut down (tasks
// channel closed). Each task is responsible for its own wg.Done.
func (b *memoryBus) worker() {
	defer b.workerWg.Done()
	for task := range b.tasks {
		task()
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
			continue
		}

		// Async handlers get a background context — they must not
		// depend on the HTTP request context.
		h := sub.handler
		eventType := e.Type
		b.wg.Add(1)
		task := func() {
			defer b.wg.Done()
			bgCtx := context.Background()
			if err := h(bgCtx, e); err != nil {
				b.logger.Error("async event handler failed",
					"event", eventType,
					"error", err,
				)
			}
		}

		// Bounded worker pool handles steady-state load. If the queue
		// is full (burst), fall back to a one-off goroutine so the
		// publisher — often an HTTP request goroutine — never blocks.
		// See perf audit 2026-04-22 #13.
		select {
		case b.tasks <- task:
		default:
			b.logger.Warn("event bus queue full, spawning overflow goroutine",
				"event", eventType,
				"queue_size", eventQueueSize,
			)
			go task()
		}
	}
}

// Shutdown waits for all in-flight async handlers to finish. It respects
// the context deadline so the server doesn't hang forever if a handler
// is stuck. Workers are daemon-style and exit on process termination.
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
