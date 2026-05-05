package sse

import (
	"bytes"
	"sync"
	"testing"
	"time"
)

// recvWithTimeout returns the next frame on ch or nil if nothing arrives
// within d. Failing tests get a clear "expected frame, got nothing" rather
// than blocking the test runner.
func recvWithTimeout(t *testing.T, ch <-chan []byte, d time.Duration) []byte {
	t.Helper()
	select {
	case msg := <-ch:
		return msg
	case <-time.After(d):
		t.Fatalf("timed out waiting %s for frame", d)
		return nil
	}
}

func TestHub_PublishToSingleSubscriber(t *testing.T) {
	h := New()
	defer h.Close()

	ch, unsub := h.Subscribe("topic-A")
	defer unsub()

	h.Publish("topic-A", []byte("hello"))

	got := recvWithTimeout(t, ch, time.Second)
	if !bytes.Equal(got, []byte("hello")) {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestHub_PublishFanOut(t *testing.T) {
	h := New()
	defer h.Close()

	ch1, unsub1 := h.Subscribe("comp:42:cat:abc")
	defer unsub1()
	ch2, unsub2 := h.Subscribe("comp:42:cat:abc")
	defer unsub2()

	h.Publish("comp:42:cat:abc", []byte("frame"))

	got1 := recvWithTimeout(t, ch1, time.Second)
	got2 := recvWithTimeout(t, ch2, time.Second)
	if !bytes.Equal(got1, []byte("frame")) || !bytes.Equal(got2, []byte("frame")) {
		t.Errorf("subscribers received %q and %q, want both %q", got1, got2, "frame")
	}
}

func TestHub_TopicIsolation(t *testing.T) {
	h := New()
	defer h.Close()

	chA, unsubA := h.Subscribe("topic-A")
	defer unsubA()
	chB, unsubB := h.Subscribe("topic-B")
	defer unsubB()

	h.Publish("topic-A", []byte("for A"))

	gotA := recvWithTimeout(t, chA, time.Second)
	if !bytes.Equal(gotA, []byte("for A")) {
		t.Errorf("topic-A subscriber got %q, want %q", gotA, "for A")
	}
	// topic-B should NOT have received anything.
	select {
	case msg := <-chB:
		t.Errorf("topic-B subscriber received unexpected frame: %q", msg)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestHub_PublishWithNoSubscribersIsNoop(t *testing.T) {
	h := New()
	defer h.Close()

	// Should not panic or block.
	h.Publish("nobody-home", []byte("dropped"))
	if got := h.SubscriberCount("nobody-home"); got != 0 {
		t.Errorf("SubscriberCount = %d, want 0", got)
	}
}

func TestHub_UnsubscribeStopsDelivery(t *testing.T) {
	h := New()
	defer h.Close()

	ch, unsub := h.Subscribe("topic-A")
	if h.SubscriberCount("topic-A") != 1 {
		t.Fatalf("SubscriberCount before unsub = %d, want 1", h.SubscriberCount("topic-A"))
	}

	unsub()

	if h.SubscriberCount("topic-A") != 0 {
		t.Errorf("SubscriberCount after unsub = %d, want 0", h.SubscriberCount("topic-A"))
	}
	// Channel should be closed; recv returns zero-value with ok=false.
	select {
	case msg, ok := <-ch:
		if ok {
			t.Errorf("expected closed channel, got frame %q", msg)
		}
	case <-time.After(50 * time.Millisecond):
		t.Error("channel was not closed by unsubscribe")
	}
}

func TestHub_DoubleUnsubscribeIsSafe(t *testing.T) {
	h := New()
	defer h.Close()
	_, unsub := h.Subscribe("topic-A")
	unsub()
	unsub() // must not panic or double-close
}

func TestHub_SlowSubscriberDoesNotBlockPublisher(t *testing.T) {
	h := New()
	defer h.Close()

	// One slow subscriber that never reads, plus one fast subscriber
	// that we use to assert the publisher made it through.
	_, unsubSlow := h.Subscribe("topic-A")
	defer unsubSlow()

	chFast, unsubFast := h.Subscribe("topic-A")
	defer unsubFast()

	// Spam past the buffer size so the slow subscriber's channel fills.
	// The publisher should keep going; the fast subscriber gets the
	// last frame written before its own buffer fills (which here is
	// exactly the buffer-size'th frame).
	for i := 0; i < SubscriberBufferSize+5; i++ {
		h.Publish("topic-A", []byte{byte(i)})
	}

	// Drain the fast subscriber and verify we got at least
	// SubscriberBufferSize frames (i.e. the publisher wasn't blocked).
	count := 0
loop:
	for {
		select {
		case <-chFast:
			count++
		case <-time.After(50 * time.Millisecond):
			break loop
		}
	}
	if count < SubscriberBufferSize {
		t.Errorf("fast subscriber received only %d frames, want at least %d (publisher likely blocked)",
			count, SubscriberBufferSize)
	}
}

func TestHub_CloseClosesAllSubscribers(t *testing.T) {
	h := New()
	chA, _ := h.Subscribe("topic-A")
	chB, _ := h.Subscribe("topic-B")

	h.Close()

	for _, ch := range []<-chan []byte{chA, chB} {
		select {
		case _, ok := <-ch:
			if ok {
				t.Error("expected closed channel")
			}
		case <-time.After(50 * time.Millisecond):
			t.Error("channel not closed by Hub.Close")
		}
	}
}

func TestHub_DoubleCloseIsSafe(t *testing.T) {
	h := New()
	h.Close()
	h.Close() // must not panic
}

func TestHub_SubscribeAfterCloseReturnsClosedChannel(t *testing.T) {
	h := New()
	h.Close()

	ch, unsub := h.Subscribe("topic-A")
	// Unsubscribe should be a no-op (no panic).
	unsub()

	// Channel should already be closed.
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("Subscribe after Close should return closed channel")
		}
	case <-time.After(50 * time.Millisecond):
		t.Error("channel was not closed")
	}
}

func TestHub_ConcurrentPublishAndSubscribe(t *testing.T) {
	// Race-detector smoke test: many goroutines hammering the hub at once.
	// Run with `go test -race ./internal/sse/` to catch lock issues.
	h := New()
	defer h.Close()

	const (
		subscribers = 20
		publishers  = 5
		framesEach  = 100
	)

	var wg sync.WaitGroup

	// Subscribers come and go.
	for i := 0; i < subscribers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch, unsub := h.Subscribe("topic-A")
			defer unsub()
			// Drain whatever shows up for a short window, then leave.
			t := time.After(100 * time.Millisecond)
			for {
				select {
				case <-ch:
				case <-t:
					return
				}
			}
		}()
	}

	// Publishers fire frames concurrently.
	for i := 0; i < publishers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < framesEach; j++ {
				h.Publish("topic-A", []byte{byte(i), byte(j)})
			}
		}(i)
	}

	wg.Wait()
	// If we got here without deadlock or panic, the test passes.
}
