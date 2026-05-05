package competition

import (
	"testing"
)

// Helpers for constructing test data without verbose pointer dance.
func intp(i int) *int             { return &i }
func problem(id string, sort int) Problem {
	return Problem{ID: id, Label: id, SortOrder: sort}
}

// ── topZone.Score ──────────────────────────────────────────

func TestTopZone_Score(t *testing.T) {
	problems := []Problem{
		problem("P1", 1), problem("P2", 2), problem("P3", 3), problem("P4", 4),
	}

	tests := []struct {
		name           string
		attempts       []Attempt
		wantTops       int
		wantZones      int
		wantAttToTop   int
		wantAttToZone  int
		wantPerProblem int // count of entries in PerProblem
	}{
		{
			name:           "no attempts",
			attempts:       nil,
			wantPerProblem: 0,
		},
		{
			name: "one flash top",
			attempts: []Attempt{
				{ProblemID: "P1", Attempts: 1, ZoneAttempts: intp(1), ZoneReached: true, TopReached: true},
			},
			wantTops: 1, wantZones: 1, wantAttToTop: 1, wantAttToZone: 1, wantPerProblem: 1,
		},
		{
			name: "zone only (no top)",
			attempts: []Attempt{
				{ProblemID: "P1", Attempts: 4, ZoneAttempts: intp(2), ZoneReached: true, TopReached: false},
			},
			wantZones: 1, wantAttToZone: 2, wantPerProblem: 1,
		},
		{
			name: "all four problems topped, mixed attempts",
			attempts: []Attempt{
				{ProblemID: "P1", Attempts: 1, ZoneAttempts: intp(1), ZoneReached: true, TopReached: true},
				{ProblemID: "P2", Attempts: 2, ZoneAttempts: intp(1), ZoneReached: true, TopReached: true},
				{ProblemID: "P3", Attempts: 3, ZoneAttempts: intp(2), ZoneReached: true, TopReached: true},
				{ProblemID: "P4", Attempts: 5, ZoneAttempts: intp(3), ZoneReached: true, TopReached: true},
			},
			wantTops: 4, wantZones: 4, wantAttToTop: 11, wantAttToZone: 7, wantPerProblem: 4,
		},
		{
			name: "attempt for unknown problem is skipped, not error",
			attempts: []Attempt{
				{ProblemID: "GHOST", Attempts: 99, TopReached: true},
				{ProblemID: "P1", Attempts: 1, TopReached: true},
			},
			wantTops: 1, wantAttToTop: 1, wantPerProblem: 1,
		},
		{
			name: "zone with nil ZoneAttempts: counts the zone but adds 0 to AttemptsToZone",
			attempts: []Attempt{
				{ProblemID: "P1", Attempts: 3, ZoneAttempts: nil, ZoneReached: true, TopReached: false},
			},
			wantZones: 1, wantAttToZone: 0, wantPerProblem: 1,
		},
	}

	tz := topZone{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tz.Score("reg-A", tc.attempts, problems, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.RegistrationID != "reg-A" {
				t.Errorf("RegistrationID = %q, want reg-A", got.RegistrationID)
			}
			if got.Tops != tc.wantTops {
				t.Errorf("Tops = %d, want %d", got.Tops, tc.wantTops)
			}
			if got.Zones != tc.wantZones {
				t.Errorf("Zones = %d, want %d", got.Zones, tc.wantZones)
			}
			if got.AttemptsToTop != tc.wantAttToTop {
				t.Errorf("AttemptsToTop = %d, want %d", got.AttemptsToTop, tc.wantAttToTop)
			}
			if got.AttemptsToZone != tc.wantAttToZone {
				t.Errorf("AttemptsToZone = %d, want %d", got.AttemptsToZone, tc.wantAttToZone)
			}
			if len(got.PerProblem) != tc.wantPerProblem {
				t.Errorf("len(PerProblem) = %d, want %d", len(got.PerProblem), tc.wantPerProblem)
			}
			if got.Points != 0 {
				t.Errorf("Points = %v, want 0 (top_zone doesn't use Points)", got.Points)
			}
		})
	}
}

// ── topZone.Rank: each tier of the comparator ─────────────

func TestTopZone_Rank_OrderingHierarchy(t *testing.T) {
	tz := topZone{}

	scoreFor := func(id string, tops, zones, attTop, attZone int) ClimberScore {
		return ClimberScore{
			RegistrationID: id,
			Tops:           tops,
			Zones:          zones,
			AttemptsToTop:  attTop,
			AttemptsToZone: attZone,
		}
	}

	tests := []struct {
		name      string
		input     []ClimberScore
		wantOrder []string // by RegistrationID
		wantRanks []int    // parallel to wantOrder
	}{
		{
			name: "more tops wins",
			input: []ClimberScore{
				scoreFor("A", 2, 4, 5, 8),
				scoreFor("B", 4, 4, 99, 99), // more tops, even with way more attempts
				scoreFor("C", 3, 4, 3, 3),
			},
			wantOrder: []string{"B", "C", "A"},
			wantRanks: []int{1, 2, 3},
		},
		{
			name: "tie on tops → more zones wins",
			input: []ClimberScore{
				scoreFor("A", 3, 3, 5, 5),
				scoreFor("B", 3, 4, 5, 5),
			},
			wantOrder: []string{"B", "A"},
			wantRanks: []int{1, 2},
		},
		{
			name: "tie on tops + zones → fewer attempts to top wins",
			input: []ClimberScore{
				scoreFor("A", 3, 3, 8, 5),
				scoreFor("B", 3, 3, 6, 5),
			},
			wantOrder: []string{"B", "A"},
			wantRanks: []int{1, 2},
		},
		{
			name: "tie on tops + zones + attempts to top → fewer attempts to zone wins",
			input: []ClimberScore{
				scoreFor("A", 3, 3, 6, 8),
				scoreFor("B", 3, 3, 6, 5),
			},
			wantOrder: []string{"B", "A"},
			wantRanks: []int{1, 2},
		},
		{
			name: "fully tied → standard ranking (T1, T1, 3)",
			input: []ClimberScore{
				scoreFor("A", 3, 3, 6, 5),
				scoreFor("B", 3, 3, 6, 5),
				scoreFor("C", 2, 3, 6, 5),
			},
			wantOrder: []string{"A", "B", "C"},
			wantRanks: []int{1, 1, 3},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ranked := tz.Rank(tc.input, nil)
			if len(ranked) != len(tc.wantOrder) {
				t.Fatalf("len = %d, want %d", len(ranked), len(tc.wantOrder))
			}
			for i, want := range tc.wantOrder {
				if ranked[i].RegistrationID != want {
					t.Errorf("position %d = %q, want %q (full order: %s)",
						i, ranked[i].RegistrationID, want, summarize(ranked))
				}
				if ranked[i].Rank != tc.wantRanks[i] {
					t.Errorf("rank at position %d = %d, want %d", i, ranked[i].Rank, tc.wantRanks[i])
				}
			}
		})
	}
}

// ── topZone.Rank: count-back tiebreak ─────────────────────

func TestTopZone_Rank_CountBackTiebreak(t *testing.T) {
	tz := topZone{}

	// Two climbers tied 2 tops / 2 zones / 4 attempts to top / 2 attempts
	// to zone. Distinguishing them needs count-back: A's best send was 1
	// attempt, B's best was 2 — A wins.
	a := ClimberScore{
		RegistrationID: "A",
		Tops:           2, Zones: 2, AttemptsToTop: 4, AttemptsToZone: 2,
		PerProblem: []ProblemScore{
			{ProblemID: "P1", Attempts: 1, TopReached: true, ZoneReached: true, ZoneAttempts: intp(1), SortOrder: 1},
			{ProblemID: "P2", Attempts: 3, TopReached: true, ZoneReached: true, ZoneAttempts: intp(1), SortOrder: 2},
		},
	}
	b := ClimberScore{
		RegistrationID: "B",
		Tops:           2, Zones: 2, AttemptsToTop: 4, AttemptsToZone: 2,
		PerProblem: []ProblemScore{
			{ProblemID: "P1", Attempts: 2, TopReached: true, ZoneReached: true, ZoneAttempts: intp(1), SortOrder: 1},
			{ProblemID: "P2", Attempts: 2, TopReached: true, ZoneReached: true, ZoneAttempts: intp(1), SortOrder: 2},
		},
	}
	ranked := tz.Rank([]ClimberScore{b, a}, nil)
	if ranked[0].RegistrationID != "A" {
		t.Errorf("count-back: expected A first, got %s (full: %s)",
			ranked[0].RegistrationID, summarize(ranked))
	}
	if ranked[0].Rank != 1 || ranked[1].Rank != 2 {
		t.Errorf("expected ranks 1,2 got %d,%d", ranked[0].Rank, ranked[1].Rank)
	}
}

func TestTopZone_Rank_CountBackZoneOnly(t *testing.T) {
	tz := topZone{}

	// Two climbers tied: 0 tops, 2 zones, 0 attempts-to-top, 4 attempts-to-zone.
	// Count-back compares zone attempts on the per-problem basis.
	a := ClimberScore{
		RegistrationID: "A", Zones: 2, AttemptsToZone: 4,
		PerProblem: []ProblemScore{
			{ProblemID: "P1", Attempts: 1, ZoneReached: true, ZoneAttempts: intp(1)},
			{ProblemID: "P2", Attempts: 5, ZoneReached: true, ZoneAttempts: intp(3)},
		},
	}
	b := ClimberScore{
		RegistrationID: "B", Zones: 2, AttemptsToZone: 4,
		PerProblem: []ProblemScore{
			{ProblemID: "P1", Attempts: 4, ZoneReached: true, ZoneAttempts: intp(2)},
			{ProblemID: "P2", Attempts: 4, ZoneReached: true, ZoneAttempts: intp(2)},
		},
	}
	ranked := tz.Rank([]ClimberScore{b, a}, nil)
	if ranked[0].RegistrationID != "A" {
		t.Errorf("count-back zone-only: expected A first, got %s", ranked[0].RegistrationID)
	}
}

func TestTopZone_Rank_EmptyInputReturnsEmpty(t *testing.T) {
	if got := (topZone{}).Rank(nil, nil); len(got) != 0 {
		t.Errorf("Rank(nil) = %v, want empty", got)
	}
}

// summarize helps debug ranking failures by dumping the produced order.
func summarize(rs []RankedScore) string {
	b := ""
	for _, r := range rs {
		if b != "" {
			b += ", "
		}
		b += r.RegistrationID
	}
	return "[" + b + "]"
}
