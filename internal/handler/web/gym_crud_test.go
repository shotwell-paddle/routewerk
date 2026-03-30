package webhandler

import (
	"testing"
)

// ── nilIfEmpty ──────────────────────────────────────────────

func TestNilIfEmpty(t *testing.T) {
	// Empty string returns nil
	if got := nilIfEmpty(""); got != nil {
		t.Errorf("nilIfEmpty(\"\") = %v, want nil", got)
	}

	// Non-empty string returns pointer
	got := nilIfEmpty("hello")
	if got == nil {
		t.Fatal("nilIfEmpty(\"hello\") = nil, want non-nil")
	}
	if *got != "hello" {
		t.Errorf("nilIfEmpty(\"hello\") = %q, want %q", *got, "hello")
	}
}

// ── gymSlugify ──────────────────────────────────────────────

func TestGymSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"My Gym", "my-gym"},
		{"  Boulder Central  ", "boulder-central"},
		{"Rock & Rope", "rock-rope"},
		{"already-slugged", "already-slugged"},
		{"UPPER CASE GYM", "upper-case-gym"},
		{"", ""},
		{"Gym #1 (Main)", "gym-1-main"},
		{"café & bouldering", "caf-bouldering"}, // non-ASCII stripped
		{"123 Numbers Start", "123-numbers-start"},
		{"multi   space", "multi-space"},
		{"---leading-trailing---", "leading-trailing"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := gymSlugify(tc.input)
			if got != tc.want {
				t.Errorf("gymSlugify(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
