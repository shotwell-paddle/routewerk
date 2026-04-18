package webhandler

import (
	"testing"
)

// ── canDeleteCardBatch ──────────────────────────────────────
//
// The delete authorization rule: batch creator OR a user at head_setter+.
// Kept as a pure function specifically so it can be unit-tested here
// without a DB or HTTP dependency.

func TestCanDeleteCardBatch(t *testing.T) {
	tests := []struct {
		name      string
		creator   string
		actor     string
		role      string
		want      bool
	}{
		{"creator deletes own", "u-1", "u-1", "setter", true},
		{"setter cannot delete others", "u-1", "u-2", "setter", false},
		{"head_setter can delete others", "u-1", "u-2", "head_setter", true},
		{"gym_manager can delete others", "u-1", "u-2", "gym_manager", true},
		{"org_admin can delete others", "u-1", "u-2", "org_admin", true},
		{"climber cannot even own", "u-1", "u-1", "climber", true}, // creator carve-out wins
		{"climber cannot delete others", "u-1", "u-2", "climber", false},
		{"empty role is rejected for non-creator", "u-1", "u-2", "", false},
		{"empty creator+actor doesn't pass creator check", "", "", "setter", false},
		{"unknown role treated as zero rank", "u-1", "u-2", "bogus", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := canDeleteCardBatch(tc.creator, tc.actor, tc.role)
			if got != tc.want {
				t.Errorf("canDeleteCardBatch(%q, %q, %q) = %v, want %v",
					tc.creator, tc.actor, tc.role, got, tc.want)
			}
		})
	}
}

// ── CardBatchFormValues helpers ─────────────────────────────

func TestCardBatchFormValues_IsEdit(t *testing.T) {
	if (CardBatchFormValues{}).IsEdit() {
		t.Error("empty form should not be edit mode")
	}
	if !(CardBatchFormValues{EditBatchID: "abc"}).IsEdit() {
		t.Error("form with EditBatchID should be edit mode")
	}
}

func TestCardBatchFormValues_FormAction(t *testing.T) {
	tests := []struct {
		name string
		form CardBatchFormValues
		want string
	}{
		{"create mode posts to /new", CardBatchFormValues{}, "/card-batches/new"},
		{"edit mode posts to /{id}/edit", CardBatchFormValues{EditBatchID: "b-42"}, "/card-batches/b-42/edit"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.form.FormAction(); got != tc.want {
				t.Errorf("FormAction() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCardBatchFormValues_Selected(t *testing.T) {
	form := CardBatchFormValues{RouteIDs: []string{"a", "b", "c"}}
	set := form.Selected()
	if len(set) != 3 {
		t.Fatalf("len = %d, want 3", len(set))
	}
	for _, id := range []string{"a", "b", "c"} {
		if !set[id] {
			t.Errorf("Selected()[%q] = false, want true", id)
		}
	}
	if set["missing"] {
		t.Error("Selected()[missing] = true, want false (map zero value)")
	}
}

func TestCardBatchFormValues_Selected_Empty(t *testing.T) {
	form := CardBatchFormValues{}
	set := form.Selected()
	if set == nil {
		t.Fatal("Selected() returned nil, want empty non-nil map")
	}
	if len(set) != 0 {
		t.Errorf("len = %d, want 0", len(set))
	}
}
