package competition

import (
	"encoding/json"
	"testing"
)

// TestRegistry_HasAllBuiltins confirms the three out-of-the-box scorers
// self-register on import. The names are an external contract (saved in
// competitions.scoring_rule), so this also doubles as a regression test
// for accidental rename.
func TestRegistry_HasAllBuiltins(t *testing.T) {
	for _, name := range []string{"top_zone", "fixed", "decay"} {
		t.Run(name, func(t *testing.T) {
			s, ok := Get(name)
			if !ok {
				t.Fatalf("scorer %q not registered", name)
			}
			if s.Name() != name {
				t.Errorf("Name() = %q, want %q", s.Name(), name)
			}
		})
	}
}

func TestRegistry_GetUnknown(t *testing.T) {
	if s, ok := Get("not_a_real_scorer"); ok {
		t.Errorf("Get unknown scorer returned ok=true (s=%v)", s)
	}
}

func TestRegistry_NamesIncludesAllBuiltins(t *testing.T) {
	got := map[string]bool{}
	for _, n := range Names() {
		got[n] = true
	}
	for _, want := range []string{"top_zone", "fixed", "decay"} {
		if !got[want] {
			t.Errorf("Names() missing %q (got %v)", want, got)
		}
	}
}

func TestRegistry_DuplicateRegisterPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on duplicate Register, got none")
		}
	}()
	// top_zone is already registered via init(); re-registering panics.
	Register(topZone{})
}

func TestRegistry_EmptyNamePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on empty Name(), got none")
		}
	}()
	Register(emptyNameScorer{})
}

type emptyNameScorer struct{}

func (emptyNameScorer) Name() string { return "" }
func (emptyNameScorer) Score(string, []Attempt, []Problem, json.RawMessage) (ClimberScore, error) {
	return ClimberScore{}, nil
}
func (emptyNameScorer) Rank([]ClimberScore, json.RawMessage) []RankedScore { return nil }
