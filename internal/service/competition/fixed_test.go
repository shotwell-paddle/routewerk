package competition

import (
	"encoding/json"
	"testing"
)

func floatp(f float64) *float64 { return &f }

func problemP(id string, sort int, points float64) Problem {
	return Problem{ID: id, Label: id, SortOrder: sort, Points: floatp(points)}
}

// ── fixed.Score ────────────────────────────────────────────

func TestFixed_Score_BasicSum(t *testing.T) {
	problems := []Problem{
		problemP("P1", 1, 100),
		problemP("P2", 2, 200),
		problemP("P3", 3, 300),
	}
	attempts := []Attempt{
		{ProblemID: "P1", Attempts: 2, TopReached: true},
		{ProblemID: "P2", Attempts: 5, TopReached: true},
		{ProblemID: "P3", Attempts: 3, TopReached: false}, // un-sent: zero contribution
	}
	got, err := (fixed{}).Score("reg-A", attempts, problems, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Points != 300 {
		t.Errorf("Points = %v, want 300 (P1=100 + P2=200 + P3=0)", got.Points)
	}
	if got.Tops != 2 {
		t.Errorf("Tops = %d, want 2", got.Tops)
	}
}

func TestFixed_Score_FlashBonus(t *testing.T) {
	problems := []Problem{
		problemP("P1", 1, 100), // flashed → +bonus
		problemP("P2", 2, 200), // not flashed → no bonus
	}
	attempts := []Attempt{
		{ProblemID: "P1", Attempts: 1, TopReached: true},
		{ProblemID: "P2", Attempts: 3, TopReached: true},
	}
	cfg := json.RawMessage(`{"flash_bonus": 50}`)

	got, err := (fixed{}).Score("reg-A", attempts, problems, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := 100.0 + 50 + 200.0
	if got.Points != want {
		t.Errorf("Points = %v, want %v", got.Points, want)
	}
}

func TestFixed_Score_MalformedConfigErrors(t *testing.T) {
	if _, err := (fixed{}).Score("reg-A", nil, nil, json.RawMessage(`{not json`)); err == nil {
		t.Error("expected error on malformed config, got nil")
	}
}

func TestFixed_Score_NilProblemPointsTreatedAsZero(t *testing.T) {
	// Problem with no Points value: a top still counts toward Tops but
	// contributes 0 to Points. Useful if staff forgot to set points.
	problems := []Problem{{ID: "P1", Label: "P1", SortOrder: 1}}
	attempts := []Attempt{{ProblemID: "P1", Attempts: 1, TopReached: true}}

	got, err := (fixed{}).Score("reg-A", attempts, problems, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Points != 0 {
		t.Errorf("Points = %v, want 0 (no per-problem points configured)", got.Points)
	}
	if got.Tops != 1 {
		t.Errorf("Tops = %d, want 1", got.Tops)
	}
}

// ── fixed.Rank ─────────────────────────────────────────────

func TestFixed_Rank_PointsDescAttemptsAsc(t *testing.T) {
	scores := []ClimberScore{
		{RegistrationID: "A", Points: 300, AttemptsToTop: 8},
		{RegistrationID: "B", Points: 500, AttemptsToTop: 12},
		{RegistrationID: "C", Points: 500, AttemptsToTop: 5}, // ties B on points but fewer attempts
	}
	ranked := (fixed{}).Rank(scores, nil)
	want := []string{"C", "B", "A"}
	for i, w := range want {
		if ranked[i].RegistrationID != w {
			t.Errorf("position %d = %q, want %q", i, ranked[i].RegistrationID, w)
		}
	}
}

func TestFixed_Rank_FullyTied(t *testing.T) {
	scores := []ClimberScore{
		{RegistrationID: "A", Points: 100, AttemptsToTop: 3},
		{RegistrationID: "B", Points: 100, AttemptsToTop: 3},
	}
	ranked := (fixed{}).Rank(scores, nil)
	if ranked[0].Rank != 1 || ranked[1].Rank != 1 {
		t.Errorf("expected both rank 1, got %d, %d", ranked[0].Rank, ranked[1].Rank)
	}
}
