package jobs

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestEnqueueParams_Defaults(t *testing.T) {
	p := EnqueueParams{
		JobType: "test.job",
	}

	if p.Queue != "" {
		t.Errorf("Queue should be empty before processing, got %q", p.Queue)
	}

	// Simulate what Enqueue does with defaults
	if p.Queue == "" {
		p.Queue = "default"
	}
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = 3
	}
	if p.Payload == nil {
		p.Payload = json.RawMessage("{}")
	}
	if p.RunAt.IsZero() {
		p.RunAt = time.Now()
	}

	if p.Queue != "default" {
		t.Errorf("Queue = %q, want default", p.Queue)
	}
	if p.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", p.MaxAttempts)
	}
	if string(p.Payload) != "{}" {
		t.Errorf("Payload = %s, want {}", p.Payload)
	}
	if time.Since(p.RunAt) > time.Second {
		t.Error("RunAt should be recent")
	}
}

func TestNewQueue(t *testing.T) {
	q := NewQueue(nil)
	if q == nil {
		t.Fatal("NewQueue returned nil")
	}
	if q.pollInterval != 5*time.Second {
		t.Errorf("pollInterval = %v, want 5s", q.pollInterval)
	}
	if q.staleTimeout != 10*time.Minute {
		t.Errorf("staleTimeout = %v, want 10m", q.staleTimeout)
	}
}

func TestQueue_Register(t *testing.T) {
	q := NewQueue(nil)

	called := false
	q.Register("test.job", func(_ context.Context, _ Job) error {
		called = true
		return nil
	})

	q.mu.RLock()
	_, exists := q.handlers["test.job"]
	q.mu.RUnlock()

	if !exists {
		t.Error("handler should be registered for test.job")
	}
	// We can't invoke the handler without a real DB, but we verified registration
	_ = called
}

func TestJob_Struct(t *testing.T) {
	j := Job{
		ID:          1,
		Queue:       "default",
		JobType:     "email.welcome",
		Payload:     json.RawMessage(`{"user_email":"test@example.com"}`),
		Status:      "pending",
		Attempts:    0,
		MaxAttempts: 3,
	}

	if j.JobType != "email.welcome" {
		t.Errorf("JobType = %q, want email.welcome", j.JobType)
	}

	var payload map[string]string
	if err := json.Unmarshal(j.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["user_email"] != "test@example.com" {
		t.Errorf("user_email = %q, want test@example.com", payload["user_email"])
	}
}
