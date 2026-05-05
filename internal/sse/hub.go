// Package sse provides a generic Server-Sent Events fan-out hub.
//
// One Hub serves many topics; each subscriber receives raw byte frames
// for the topic(s) it cares about. Topic naming is the caller's
// responsibility — the comp module uses "comp:{id}:cat:{id}" for
// per-category leaderboard streams and "comp:{id}" for comp-level
// updates, but the hub doesn't care.
//
// Design choices:
//
//   - Non-blocking publish. If a subscriber's channel is full, the
//     hub drops the frame for that subscriber and continues. Leaderboard
//     updates are idempotent — the next frame supersedes the dropped one,
//     so dropping is preferable to backpressure that could stall the
//     write transaction that triggered the publish.
//
//   - In-process only. All subscribers must be on the same Go process.
//     Single-region Fly deploy makes this fine for v1; if we ever scale
//     horizontally, swap the hub for Postgres LISTEN/NOTIFY behind the
//     same Subscribe/Publish interface.
//
//   - Subscriber buffer is fixed at 16 frames. Tuned for "tail of recent
//     state" semantics, not "every event". Increase if you need higher
//     burst tolerance.
package sse

import "sync"

// SubscriberBufferSize is the per-subscriber channel capacity. Frames
// beyond this are dropped silently if the subscriber is too slow.
const SubscriberBufferSize = 16

// Hub fans out byte frames to subscribers, keyed by topic.
type Hub struct {
	mu          sync.RWMutex
	subscribers map[string]map[chan []byte]struct{}
	closed      bool
}

// New returns a fresh Hub.
func New() *Hub {
	return &Hub{
		subscribers: map[string]map[chan []byte]struct{}{},
	}
}

// Subscribe registers a subscriber on the named topic and returns a
// receive-only channel of frames plus an unsubscribe function. Always
// call the unsubscribe function (defer is the usual pattern) to avoid
// leaking goroutines and channels.
//
// Subscribing to a closed Hub returns a closed channel and a no-op
// unsubscribe. This lets callers keep their patterns the same during
// shutdown.
func (h *Hub) Subscribe(topic string) (<-chan []byte, func()) {
	ch := make(chan []byte, SubscriberBufferSize)

	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		close(ch)
		return ch, func() {}
	}
	subs, ok := h.subscribers[topic]
	if !ok {
		subs = map[chan []byte]struct{}{}
		h.subscribers[topic] = subs
	}
	subs[ch] = struct{}{}
	h.mu.Unlock()

	unsubscribe := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if subs, ok := h.subscribers[topic]; ok {
			if _, present := subs[ch]; present {
				delete(subs, ch)
				close(ch)
			}
			if len(subs) == 0 {
				delete(h.subscribers, topic)
			}
		}
	}
	return ch, unsubscribe
}

// Publish sends frame to every subscriber on topic. Subscribers whose
// buffer is full are skipped silently — this is the dropped-frame
// behavior described in the package doc.
//
// Calling Publish on a closed Hub is a no-op.
func (h *Hub) Publish(topic string, frame []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.closed {
		return
	}
	subs, ok := h.subscribers[topic]
	if !ok {
		return
	}
	for ch := range subs {
		select {
		case ch <- frame:
			// delivered
		default:
			// subscriber is too slow; drop this frame for them
		}
	}
}

// SubscriberCount returns the number of active subscribers on topic.
// Useful for tests and for "is anyone listening?" optimizations in
// callers that can skip building expensive frames.
func (h *Hub) SubscriberCount(topic string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subscribers[topic])
}

// Close marks the hub as shut down and closes every subscriber channel.
// Subsequent Subscribe and Publish calls are no-ops. Safe to call more
// than once.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	for topic, subs := range h.subscribers {
		for ch := range subs {
			close(ch)
		}
		delete(h.subscribers, topic)
	}
}
