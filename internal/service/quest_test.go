package service

import (
	"testing"
)

// ── Error Sentinels ───────────────────────────────────────────────

func TestQuestErrors_Distinct(t *testing.T) {
	errs := []error{
		ErrQuestNotFound,
		ErrQuestNotActive,
		ErrQuestNotAvailable,
		ErrAlreadyEnrolled,
		ErrClimberQuestNotFound,
		ErrNotOwner,
	}

	seen := make(map[string]bool)
	for _, e := range errs {
		msg := e.Error()
		if msg == "" {
			t.Error("quest error has empty message")
		}
		if seen[msg] {
			t.Errorf("duplicate quest error message: %q", msg)
		}
		seen[msg] = true
	}
}

func TestQuestErrors_NonNil(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"ErrQuestNotFound", ErrQuestNotFound},
		{"ErrQuestNotActive", ErrQuestNotActive},
		{"ErrQuestNotAvailable", ErrQuestNotAvailable},
		{"ErrAlreadyEnrolled", ErrAlreadyEnrolled},
		{"ErrClimberQuestNotFound", ErrClimberQuestNotFound},
		{"ErrNotOwner", ErrNotOwner},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Errorf("%s is nil", tt.name)
			}
		})
	}
}

// ── Constructor ───────────────────────────────────────────────────

func TestNewQuestService(t *testing.T) {
	svc := NewQuestService(nil, nil, nil)
	if svc == nil {
		t.Fatal("NewQuestService returned nil")
	}
}

// ── Helpers ───────────────────────────────────────────────────────

func TestDerefStr(t *testing.T) {
	tests := []struct {
		name string
		in   *string
		want string
	}{
		{"nil", nil, ""},
		{"empty", strPtr(""), ""},
		{"value", strPtr("hello"), "hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := derefStr(tt.in); got != tt.want {
				t.Errorf("derefStr() = %q, want %q", got, tt.want)
			}
		})
	}
}
