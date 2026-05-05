package handler

import (
	"net/http"
	"strings"
	"testing"

	"github.com/shotwell-paddle/routewerk/internal/sse"
)

// ── writeSSEEvent ──────────────────────────────────────────

func TestWriteSSEEvent_NamedEvent(t *testing.T) {
	rw := &captureWriter{}
	payload := map[string]any{"hello": "world"}

	if err := writeSSEEvent(rw, "leaderboard", payload); err != nil {
		t.Fatalf("writeSSEEvent: %v", err)
	}
	got := rw.buf.String()
	if !strings.HasPrefix(got, "event: leaderboard\n") {
		t.Errorf("missing event line; got: %q", got)
	}
	if !strings.Contains(got, "data: {\"hello\":\"world\"}\n") {
		t.Errorf("missing data line; got: %q", got)
	}
	if !strings.HasSuffix(got, "\n\n") {
		t.Errorf("missing event terminator (blank line); got: %q", got)
	}
}

func TestWriteSSEEvent_AnonymousEvent(t *testing.T) {
	rw := &captureWriter{}
	if err := writeSSEEvent(rw, "", "ok"); err != nil {
		t.Fatalf("writeSSEEvent: %v", err)
	}
	got := rw.buf.String()
	// Anonymous (no event name) should be just data + terminator.
	if strings.Contains(got, "event:") {
		t.Errorf("anonymous event should not have event line; got: %q", got)
	}
	if !strings.HasPrefix(got, "data: \"ok\"\n") {
		t.Errorf("data line shape unexpected: %q", got)
	}
}

func TestWriteSSEEvent_UnmarshalableErrors(t *testing.T) {
	rw := &captureWriter{}
	// chan can't be JSON-encoded.
	err := writeSSEEvent(rw, "x", make(chan int))
	if err == nil {
		t.Error("expected error encoding chan, got nil")
	}
	if rw.buf.Len() != 0 {
		t.Errorf("nothing should have been written on encode failure; got %q", rw.buf.String())
	}
}

func TestWriteSSEEvent_PropagatesWriteError(t *testing.T) {
	rw := &errorWriter{}
	err := writeSSEEvent(rw, "leaderboard", "ok")
	if err == nil {
		t.Error("expected writer error to propagate, got nil")
	}
}

// ── CompTopic ──────────────────────────────────────────────

func TestCompTopic(t *testing.T) {
	if got := CompTopic("abc"); got != "comp:abc" {
		t.Errorf("CompTopic(abc) = %q, want comp:abc", got)
	}
}

// ── publishLeaderboardChange ───────────────────────────────

func TestPublishLeaderboardChange_NilHubIsSafe(t *testing.T) {
	h := &CompHandler{} // hub == nil
	// Must not panic.
	h.publishLeaderboardChange("any-comp-id")
}

func TestPublishLeaderboardChange_DeliversToSubscriber(t *testing.T) {
	hub := sse.New()
	defer hub.Close()
	h := &CompHandler{hub: hub}

	compID := "comp-42"
	ch, unsub := hub.Subscribe(CompTopic(compID))
	defer unsub()

	h.publishLeaderboardChange(compID)

	select {
	case <-ch:
		// got it
	default:
		t.Fatal("subscriber did not receive publish")
	}
}

func TestPublishLeaderboardChange_NoCrossCompDelivery(t *testing.T) {
	hub := sse.New()
	defer hub.Close()
	h := &CompHandler{hub: hub}

	chA, unsubA := hub.Subscribe(CompTopic("comp-A"))
	defer unsubA()
	chB, unsubB := hub.Subscribe(CompTopic("comp-B"))
	defer unsubB()

	h.publishLeaderboardChange("comp-A")

	if _, ok := <-chA; !ok {
		t.Error("comp-A subscriber should have received publish")
	}
	select {
	case <-chB:
		t.Error("comp-B subscriber received unexpected publish")
	default:
	}
}

// ── tiny ResponseWriter shims for the SSE write tests ──────

// captureWriter implements just enough of http.ResponseWriter to let
// writeSSEEvent's `w.Write(...)` succeed and capture the bytes for
// inspection. The full http.ResponseWriter surface (Header, WriteHeader)
// isn't exercised by writeSSEEvent.
type captureWriter struct {
	buf strings.Builder
}

func (c *captureWriter) Header() http.Header        { return http.Header{} }
func (c *captureWriter) Write(p []byte) (int, error) { return c.buf.Write(p) }
func (c *captureWriter) WriteHeader(int)             {}

// errorWriter always errors on Write. Used to verify writeSSEEvent
// propagates the error rather than swallowing it.
type errorWriter struct{}

func (e *errorWriter) Header() http.Header        { return http.Header{} }
func (e *errorWriter) Write(p []byte) (int, error) { return 0, http.ErrAbortHandler }
func (e *errorWriter) WriteHeader(int)             {}
