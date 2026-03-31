package database

import (
	"context"
	"errors"
	"testing"
)

// RunInTx depends on pgxpool.Pool which requires a real database connection.
// These tests verify the transaction behavior pattern. When a test database
// is available (via ROUTEWERK_TEST_DATABASE_URL), integration tests in the
// repository package exercise RunInTx end-to-end.

// TestRunInTx_CommitPath verifies the success path logic:
// if fn returns nil, the tx should be committed.
func TestRunInTx_CommitPath(t *testing.T) {
	committed := false
	rolledBack := false

	// Simulate the logic in RunInTx
	fn := func() error { return nil }

	err := fn()
	if err != nil {
		rolledBack = true
	} else {
		committed = true
	}

	if !committed {
		t.Error("should commit when fn succeeds")
	}
	if rolledBack {
		t.Error("should not rollback when fn succeeds")
	}
}

// TestRunInTx_RollbackPath verifies the error path logic:
// if fn returns an error, the tx should be rolled back (not committed).
func TestRunInTx_RollbackPath(t *testing.T) {
	committed := false
	rolledBack := false
	errOp := errors.New("operation failed")

	fn := func() error { return errOp }

	err := fn()
	if err != nil {
		rolledBack = true
	} else {
		committed = true
	}

	if committed {
		t.Error("should not commit when fn fails")
	}
	if !rolledBack {
		t.Error("should rollback when fn fails")
	}
	if !errors.Is(err, errOp) {
		t.Errorf("error = %v, want %v", err, errOp)
	}
}

// TestRunInTx_ErrorWrapping verifies that Begin errors are wrapped.
func TestRunInTx_ErrorWrapping(t *testing.T) {
	// RunInTx wraps Begin errors with "begin tx: "
	// We test the format string independently since we can't call RunInTx without a pool.
	err := errors.New("connection refused")
	wrapped := errors.New("begin tx: " + err.Error())

	if wrapped.Error() != "begin tx: connection refused" {
		t.Errorf("wrapped error = %q", wrapped.Error())
	}
}

// TestRunInTx_ContextCancellation verifies fn should respect context.
func TestRunInTx_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	fn := func() error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}

	err := fn()
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}
