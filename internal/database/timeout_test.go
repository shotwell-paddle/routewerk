package database

import (
	"context"
	"testing"
	"time"
)

func TestQueryTimeout_SetsDeadline(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := QueryTimeout(ctx, TimeoutLong)
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected deadline to be set")
	}
	remaining := time.Until(deadline)
	if remaining < 14*time.Second || remaining > 16*time.Second {
		t.Errorf("deadline should be ~15s from now, got %v", remaining)
	}
}

func TestQueryTimeout_PreservesShortDeadline(t *testing.T) {
	ctx, cancel1 := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel1()

	ctx2, cancel2 := QueryTimeout(ctx, TimeoutLong)
	defer cancel2()

	deadline, ok := ctx2.Deadline()
	if !ok {
		t.Fatal("expected deadline to be set")
	}
	remaining := time.Until(deadline)
	// Should still be ~1s, not extended to 15s
	if remaining > 2*time.Second {
		t.Errorf("deadline should not be extended, got %v remaining", remaining)
	}
}

func TestQueryTimeout_Constants(t *testing.T) {
	if TimeoutFast >= TimeoutDefault {
		t.Errorf("TimeoutFast (%v) should be < TimeoutDefault (%v)", TimeoutFast, TimeoutDefault)
	}
	if TimeoutDefault >= TimeoutLong {
		t.Errorf("TimeoutDefault (%v) should be < TimeoutLong (%v)", TimeoutDefault, TimeoutLong)
	}
	if TimeoutLong >= TimeoutBatch {
		t.Errorf("TimeoutLong (%v) should be < TimeoutBatch (%v)", TimeoutLong, TimeoutBatch)
	}
}
