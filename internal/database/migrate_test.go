package database

import (
	"testing"
)

// TestAllowDirtyForce verifies the ALLOW_DIRTY_MIGRATION_FORCE gate.
// Only an explicit truthy value opts in to automatic dirty-state recovery;
// unset, falsy, and garbage all mean "refuse and make the operator decide".
func TestAllowDirtyForce(t *testing.T) {
	tests := []struct {
		name  string
		value string
		set   bool
		want  bool
	}{
		{"unset", "", false, false},
		{"empty", "", true, false},
		{"true", "true", true, true},
		{"TRUE", "TRUE", true, true},
		{"1", "1", true, true},
		{"false", "false", true, false},
		{"0", "0", true, false},
		{"garbage", "yes please", true, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv("ALLOW_DIRTY_MIGRATION_FORCE", tc.value)
			}
			if got := allowDirtyForce(); got != tc.want {
				t.Errorf("allowDirtyForce() with %q = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}
