package competition

import (
	"encoding/json"
	"math"
	"testing"
)

// approxEq compares floats with a small tolerance — decay's division
// produces results that won't match exact decimal expectations.
func approxEq(a, b float64) bool {
	return math.Abs(a-b) < 0.001
}

// ── decay.Score ────────────────────────────────────────────

func TestDecay_Score_Defaults(t *testing.T) {
	// With default base=1000 and rate=0.1:
	//   1 attempt → 1000
	//   2 attempts → 1000 / 1.1 = 909.09...
	//   5 attempts → 1000 / 1.4 = 714.28...
	problems := []Problem{
		problemP("P1", 1, 0), // points unused for decay
		problemP("P2", 2, 0),
		problemP("P3", 3, 0),
	}
	attempts := []Attempt{
		{ProblemID: "P1", Attempts: 1, TopReached: true},
		{ProblemID: "P2", Attempts: 2, TopReached: true},
		{ProblemID: "P3", Attempts: 5, TopReached: true},
	}
	got, err := (decay{}).Score("reg-A", attempts, problems, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := 1000.0 + 1000.0/1.1 + 1000.0/1.4
	if !approxEq(got.Points, want) {
		t.Errorf("Points = %v, want ≈ %v", got.Points, want)
	}
	if got.Tops != 3 {
		t.Errorf("Tops = %d, want 3", got.Tops)
	}
}

func TestDecay_Score_CustomConfig(t *testing.T) {
	cfg := json.RawMessage(`{"base_points": 500, "decay_rate": 0.5, "flash_bonus": 100}`)
	problems := []Problem{problemP("P1", 1, 0), problemP("P2", 2, 0)}
	attempts := []Attempt{
		{ProblemID: "P1", Attempts: 1, TopReached: true}, // flash → 500 + 100 bonus
		{ProblemID: "P2", Attempts: 3, TopReached: true}, // 500 / (1 + 0.5*2) = 250
	}
	got, err := (decay{}).Score("reg-A", attempts, problems, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := 500.0 + 100 + 250.0
	if !approxEq(got.Points, want) {
		t.Errorf("Points = %v, want %v", got.Points, want)
	}
}

func TestDecay_Score_DecayRateZeroIsFlat(t *testing.T) {
	// rate=0 means every send is worth base_points regardless of attempts.
	cfg := json.RawMessage(`{"base_points": 200, "decay_rate": 0}`)
	problems := []Problem{problemP("P1", 1, 0), problemP("P2", 2, 0)}
	attempts := []Attempt{
		{ProblemID: "P1", Attempts: 1, TopReached: true},
		{ProblemID: "P2", Attempts: 99, TopReached: true},
	}
	got, err := (decay{}).Score("reg-A", attempts, problems, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !approxEq(got.Points, 400) {
		t.Errorf("Points = %v, want 400 (flat 200 + 200)", got.Points)
	}
}

func TestDecay_Score_ZeroAttemptsGuard(t *testing.T) {
	// Defensive: top_reached but attempts=0 should be impossible (every
	// send takes at least one try) but the guard treats it as 1 attempt.
	problems := []Problem{problemP("P1", 1, 0)}
	attempts := []Attempt{{ProblemID: "P1", Attempts: 0, TopReached: true}}
	got, err := (decay{}).Score("reg-A", attempts, problems, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !approxEq(got.Points, 1000) {
		t.Errorf("Points = %v, want 1000 (clamped to 1 attempt)", got.Points)
	}
}

func TestDecay_Score_UnSentProblemContributesNothing(t *testing.T) {
	problems := []Problem{problemP("P1", 1, 0)}
	attempts := []Attempt{{ProblemID: "P1", Attempts: 10, TopReached: false, ZoneReached: true}}
	got, err := (decay{}).Score("reg-A", attempts, problems, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Points != 0 {
		t.Errorf("Points = %v, want 0 (zone-only doesn't count)", got.Points)
	}
	if got.Zones != 1 {
		t.Errorf("Zones = %d, want 1", got.Zones)
	}
}

func TestDecay_Score_MalformedConfigErrors(t *testing.T) {
	if _, err := (decay{}).Score("reg-A", nil, nil, json.RawMessage(`bad`)); err == nil {
		t.Error("expected error on malformed config, got nil")
	}
}

// ── decay.Rank ─────────────────────────────────────────────

func TestDecay_Rank_HigherPointsWins(t *testing.T) {
	scores := []ClimberScore{
		{RegistrationID: "A", Points: 500, AttemptsToTop: 5},
		{RegistrationID: "B", Points: 800, AttemptsToTop: 5},
	}
	ranked := (decay{}).Rank(scores, nil)
	if ranked[0].RegistrationID != "B" {
		t.Errorf("position 0 = %q, want B (higher points)", ranked[0].RegistrationID)
	}
}

func TestDecay_Rank_TieOnPointsBreaksOnAttempts(t *testing.T) {
	scores := []ClimberScore{
		{RegistrationID: "A", Points: 500, AttemptsToTop: 7},
		{RegistrationID: "B", Points: 500, AttemptsToTop: 4},
	}
	ranked := (decay{}).Rank(scores, nil)
	if ranked[0].RegistrationID != "B" {
		t.Errorf("position 0 = %q, want B (fewer attempts)", ranked[0].RegistrationID)
	}
}
