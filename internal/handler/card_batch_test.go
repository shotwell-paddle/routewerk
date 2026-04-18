package handler

import (
	"testing"
)

// CanDeleteCardBatch mirrors the web handler's rule and is exported so the
// web package can re-use it. The test matrix is intentionally identical to
// the webhandler test so any future drift between the two copies shows up
// as a test failure rather than a silent authorization inconsistency.
func TestCanDeleteCardBatch(t *testing.T) {
	tests := []struct {
		name    string
		creator string
		actor   string
		role    string
		want    bool
	}{
		{"creator deletes own", "u-1", "u-1", "setter", true},
		{"setter cannot delete others", "u-1", "u-2", "setter", false},
		{"head_setter can delete others", "u-1", "u-2", "head_setter", true},
		{"gym_manager can delete others", "u-1", "u-2", "gym_manager", true},
		{"org_admin can delete others", "u-1", "u-2", "org_admin", true},
		{"climber-creator still passes", "u-1", "u-1", "climber", true},
		{"climber cannot delete others", "u-1", "u-2", "climber", false},
		{"empty role is rejected for non-creator", "u-1", "u-2", "", false},
		{"empty creator+actor doesn't pass creator check", "", "", "setter", false},
		{"unknown role treated as zero rank", "u-1", "u-2", "bogus", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CanDeleteCardBatch(tc.creator, tc.actor, tc.role)
			if got != tc.want {
				t.Errorf("CanDeleteCardBatch(%q, %q, %q) = %v, want %v",
					tc.creator, tc.actor, tc.role, got, tc.want)
			}
		})
	}
}
