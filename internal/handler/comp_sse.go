package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// Phase 1f wave 5 — leaderboard streaming via Server-Sent Events.
//
//   GET /api/v1/competitions/{id}/leaderboard/stream?category=<uuid>
//
// Long-lived text/event-stream connection. The handler:
//
//   1. Subscribes to the comp's hub topic.
//   2. Emits an initial `leaderboard` event with the current snapshot.
//   3. On each Publish (driven by SubmitActions / VerifyAttempt /
//      OverrideAttempt), rebuilds the leaderboard via the same cached
//      `buildLeaderboard` path and emits a fresh frame.
//   4. Sends a `: keepalive` comment every 30 seconds so reverse
//      proxies don't kill an idle connection.
//   5. Cleans up on client disconnect (r.Context().Done()).
//
// Topic strategy: we publish to a single topic per comp ("comp:{id}").
// Subscribers re-render with their own category filter via
// buildLeaderboard. This trades a small amount of redundant compute
// for simpler publisher logic — the 2-second cache layer means any
// burst of connected subscribers re-fetching the same payload only
// pays for one aggregateLeaderboard call.

const (
	sseHeartbeatInterval = 30 * time.Second
	sseLeaderboardEvent  = "leaderboard"
)

// CompTopic is the SSE topic name format used by the leaderboard stream.
// Action endpoint publishers must use this exact format. Exported so
// tests in other packages can produce valid topic strings.
func CompTopic(compID string) string { return "comp:" + compID }

// StreamLeaderboard handles GET /api/v1/competitions/{id}/leaderboard/stream.
func (h *CompHandler) StreamLeaderboard(w http.ResponseWriter, r *http.Request) {
	compID := chi.URLParam(r, "id")
	if !isUUID(compID) {
		Error(w, http.StatusBadRequest, "invalid competition id")
		return
	}
	categoryID := r.URL.Query().Get("category")
	if categoryID != "" && !isUUID(categoryID) {
		Error(w, http.StatusBadRequest, "invalid category id")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		// Should never happen with stdlib net/http; defensive against
		// custom middleware that might wrap the response writer.
		Error(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// Send the initial snapshot before subscribing — confirms the comp
	// exists and gives the client something immediately. If this fails,
	// we haven't yet committed any SSE headers so a normal error
	// response is still possible.
	initial, err := h.buildLeaderboard(r, compID, categoryID)
	if err != nil {
		if errors.Is(err, errLeaderboardCompNotFound) {
			Error(w, http.StatusNotFound, "competition not found")
			return
		}
		slog.Error("sse: initial build", "competition_id", compID, "error", err)
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Switch the response into streaming mode.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// X-Accel-Buffering disables nginx response buffering. Fly.io's
	// proxy doesn't need it but we may sit behind nginx some day.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	if err := writeSSEEvent(w, sseLeaderboardEvent, initial); err != nil {
		slog.Info("sse: client disconnected before first frame", "competition_id", compID)
		return
	}
	flusher.Flush()

	// Subscribe AFTER the initial frame so we don't race a publish-
	// during-build into a duplicate first frame.
	ch, unsubscribe := h.hub.Subscribe(CompTopic(compID))
	defer unsubscribe()

	heartbeat := time.NewTicker(sseHeartbeatInterval)
	defer heartbeat.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return

		case _, ok := <-ch:
			if !ok {
				// Hub closed (server shutting down).
				return
			}
			// We don't actually use the published payload — re-fetch via
			// buildLeaderboard so each subscriber renders against its own
			// category filter and benefits from the cache.
			board, err := h.buildLeaderboard(r, compID, categoryID)
			if err != nil {
				slog.Error("sse: rebuild on publish", "competition_id", compID, "error", err)
				continue
			}
			if err := writeSSEEvent(w, sseLeaderboardEvent, board); err != nil {
				return
			}
			flusher.Flush()

		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// writeSSEEvent emits one named SSE event with a JSON `data:` line.
// Returns the underlying write error so the caller can treat it as
// "client gone" and bail out of the stream loop.
func writeSSEEvent(w http.ResponseWriter, event string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var b strings.Builder
	if event != "" {
		b.WriteString("event: ")
		b.WriteString(event)
		b.WriteString("\n")
	}
	// SSE requires each `data:` line to end with `\n` and the event
	// terminator is a blank line. We inline the JSON onto a single
	// data line — it cannot contain raw newlines (json.Marshal escapes).
	b.WriteString("data: ")
	b.Write(body)
	b.WriteString("\n\n")
	_, err = w.Write([]byte(b.String()))
	return err
}

// publishLeaderboardChange notifies subscribers of the comp's stream
// that the leaderboard may have changed. The payload is empty — the
// SSE handler re-renders via buildLeaderboard rather than trusting any
// per-publish data — so this is just a "wake up and re-render" signal.
//
// Called by SubmitActions, VerifyAttempt, OverrideAttempt after their
// cache invalidation. Failure to publish is silent (slog only): the
// next subscriber poll or a fresh GET will eventually pick up the
// change anyway.
func (h *CompHandler) publishLeaderboardChange(compID string) {
	if h.hub == nil {
		return
	}
	h.hub.Publish(CompTopic(compID), []byte("changed"))
}

