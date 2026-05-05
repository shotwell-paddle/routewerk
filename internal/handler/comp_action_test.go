package handler

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/shotwell-paddle/routewerk/internal/api"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

// ── computeNewState — exhaustive table-driven coverage ─────

func TestComputeNewState(t *testing.T) {
	zero := repository.AttemptState{}

	tests := []struct {
		name    string
		current repository.AttemptState
		action  api.ActionType
		want    repository.AttemptState
		wantErr string
	}{
		// ── increment ────────────────────────────────────
		{
			name:    "increment from zero",
			current: zero,
			action:  api.Increment,
			want:    repository.AttemptState{Attempts: 1},
		},
		{
			name:    "increment from existing",
			current: repository.AttemptState{Attempts: 3, ZoneReached: true, ZoneAttempts: ip(2)},
			action:  api.Increment,
			want:    repository.AttemptState{Attempts: 4, ZoneReached: true, ZoneAttempts: ip(2)},
		},
		{
			name:    "increment never resets zone",
			current: repository.AttemptState{Attempts: 5, ZoneReached: true, ZoneAttempts: ip(3)},
			action:  api.Increment,
			want:    repository.AttemptState{Attempts: 6, ZoneReached: true, ZoneAttempts: ip(3)},
		},

		// ── zone ─────────────────────────────────────────
		{
			name:    "zone from zero coerces attempts to 1",
			current: zero,
			action:  api.Zone,
			want:    repository.AttemptState{Attempts: 1, ZoneReached: true, ZoneAttempts: ip(1)},
		},
		{
			name:    "zone records current attempts as zone_attempts",
			current: repository.AttemptState{Attempts: 4},
			action:  api.Zone,
			want:    repository.AttemptState{Attempts: 4, ZoneReached: true, ZoneAttempts: ip(4)},
		},
		{
			name:    "zone re-tap overwrites previous zone_attempts",
			current: repository.AttemptState{Attempts: 7, ZoneReached: true, ZoneAttempts: ip(3)},
			action:  api.Zone,
			want:    repository.AttemptState{Attempts: 7, ZoneReached: true, ZoneAttempts: ip(7)},
		},

		// ── top ──────────────────────────────────────────
		{
			name:    "top from zero coerces attempts and implies zone",
			current: zero,
			action:  api.Top,
			want:    repository.AttemptState{Attempts: 1, ZoneReached: true, ZoneAttempts: ip(1), TopReached: true},
		},
		{
			name:    "top after zone keeps existing zone_attempts",
			current: repository.AttemptState{Attempts: 4, ZoneReached: true, ZoneAttempts: ip(2)},
			action:  api.Top,
			want:    repository.AttemptState{Attempts: 4, ZoneReached: true, ZoneAttempts: ip(2), TopReached: true},
		},
		{
			name:    "top sets zone_attempts when no prior zone",
			current: repository.AttemptState{Attempts: 3},
			action:  api.Top,
			want:    repository.AttemptState{Attempts: 3, ZoneReached: true, ZoneAttempts: ip(3), TopReached: true},
		},
		{
			name:    "top is idempotent (re-tapping doesn't reset attempts)",
			current: repository.AttemptState{Attempts: 5, ZoneReached: true, ZoneAttempts: ip(2), TopReached: true},
			action:  api.Top,
			want:    repository.AttemptState{Attempts: 5, ZoneReached: true, ZoneAttempts: ip(2), TopReached: true},
		},

		// ── reset ────────────────────────────────────────
		{
			name:    "reset from any state returns zero",
			current: repository.AttemptState{Attempts: 9, ZoneReached: true, ZoneAttempts: ip(4), TopReached: true},
			action:  api.Reset,
			want:    repository.AttemptState{},
		},
		{
			name:    "reset from already-zero is no-op",
			current: zero,
			action:  api.Reset,
			want:    repository.AttemptState{},
		},

		// ── undo ────────────────────────────────────────
		{
			name:    "undo errors (handled outside the pure function)",
			current: repository.AttemptState{Attempts: 1},
			action:  api.Undo,
			wantErr: "undo cannot",
		},

		// ── unknown ─────────────────────────────────────
		{
			name:    "unknown action errors",
			current: zero,
			action:  api.ActionType("nonsense"),
			wantErr: "unknown action type",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := computeNewState(tc.current, tc.action)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("want error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Attempts != tc.want.Attempts {
				t.Errorf("Attempts = %d, want %d", got.Attempts, tc.want.Attempts)
			}
			if got.ZoneReached != tc.want.ZoneReached {
				t.Errorf("ZoneReached = %v, want %v", got.ZoneReached, tc.want.ZoneReached)
			}
			if got.TopReached != tc.want.TopReached {
				t.Errorf("TopReached = %v, want %v", got.TopReached, tc.want.TopReached)
			}
			if !intPtrEqual(got.ZoneAttempts, tc.want.ZoneAttempts) {
				t.Errorf("ZoneAttempts = %v, want %v", deref(got.ZoneAttempts), deref(tc.want.ZoneAttempts))
			}
		})
	}
}

func ip(i int) *int { return &i }

func intPtrEqual(a, b *int) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func deref(p *int) interface{} {
	if p == nil {
		return "<nil>"
	}
	return *p
}

// ── stateToAPI / zeroState / rejection ─────────────────────

func TestStateToAPI(t *testing.T) {
	pid := uuid.New()
	in := repository.AttemptState{Attempts: 3, ZoneReached: true, ZoneAttempts: ip(2), TopReached: true}
	out := stateToAPI(pid, in)
	if out.ProblemId != pid {
		t.Errorf("ProblemId mismatch")
	}
	if out.Attempts != 3 || !out.TopReached || !out.ZoneReached {
		t.Errorf("flags/counters: %+v", out)
	}
	if out.ZoneAttempts == nil || *out.ZoneAttempts != 2 {
		t.Errorf("ZoneAttempts = %v, want 2", out.ZoneAttempts)
	}
}

func TestZeroState(t *testing.T) {
	pid := uuid.New().String()
	out := zeroState(pid)
	if out.ProblemId.String() != pid {
		t.Errorf("ProblemId = %s, want %s", out.ProblemId, pid)
	}
	if out.Attempts != 0 || out.ZoneReached || out.TopReached {
		t.Errorf("zeroState should be all false/0; got %+v", out)
	}
}

func TestRejection(t *testing.T) {
	key := uuid.New()
	pid := uuid.New()
	action := api.ActionItem{IdempotencyKey: &key, ProblemId: pid, Type: api.Increment}
	rej := rejection(action, api.EventClosed)
	if rej.IdempotencyKey == nil || *rej.IdempotencyKey != key {
		t.Errorf("idempotency key not preserved")
	}
	if rej.ProblemId != pid {
		t.Errorf("ProblemId not preserved")
	}
	if rej.Reason != api.EventClosed {
		t.Errorf("Reason = %q, want %q", rej.Reason, api.EventClosed)
	}
}
