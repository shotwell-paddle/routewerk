// Package competition implements the scoring engine for the comp module.
//
// Three scorers ship out of the box: top_zone, fixed, and decay. A comp
// picks one via competitions.scoring_rule and configures it via
// competitions.scoring_config (jsonb). Per-event overrides live on
// competition_events.scoring_rule_override / scoring_config_override —
// see model.CompetitionEvent.EffectiveScoringRule.
//
// Scorers are PURE FUNCTIONS — no DB, no side effects. They take attempt
// data + problem metadata + config in, return scores out. Tests are
// table-driven and exhaustive (CLAUDE.md convention; this is the right
// place for rigorous coverage).
//
// Adding a new scorer:
//  1. Implement the Scorer interface in a new file (see topzone.go etc.).
//  2. Register it from init() with Register(...).
//  3. Document the expected scoring_config jsonb shape on the type.
package competition

import (
	"encoding/json"
	"fmt"
	"sync"
)

// Attempt is a climber's state on one problem. Mirrors
// model.CompetitionAttempt but stripped to the fields a scorer cares
// about. The service layer converts before calling Score.
type Attempt struct {
	RegistrationID string
	ProblemID      string
	Attempts       int
	ZoneAttempts   *int
	ZoneReached    bool
	TopReached     bool
}

// Problem is the comp problem metadata a scorer needs (label, points,
// sort order for tiebreak count-back). Mirrors model.CompetitionProblem.
type Problem struct {
	ID         string
	Label      string
	Points     *float64 // nil when the scorer doesn't use per-problem points
	ZonePoints *float64
	SortOrder  int
}

// ProblemScore is the per-problem breakdown carried inside ClimberScore
// so that ranking algorithms can do count-back tiebreaks (compare the
// best problem, then the second-best, etc.).
type ProblemScore struct {
	ProblemID    string
	Attempts     int
	ZoneAttempts *int
	Points       float64
	TopReached   bool
	ZoneReached  bool
	SortOrder    int
}

// ClimberScore is the per-climber result of Score. Different scorers
// populate different subsets of these fields:
//   - top_zone uses Tops/Zones/AttemptsToTop/AttemptsToZone (Points unused)
//   - fixed and decay use Points (Tops/Zones unused for ranking)
//
// PerProblem is always populated so Rank can do count-back when needed.
// Detail carries scorer-specific extras for the API response.
type ClimberScore struct {
	RegistrationID string         `json:"registration_id"`
	Points         float64        `json:"points"`
	Tops           int            `json:"tops"`
	Zones          int            `json:"zones"`
	AttemptsToTop  int            `json:"attempts_to_top"`
	AttemptsToZone int            `json:"attempts_to_zone"`
	PerProblem     []ProblemScore `json:"per_problem,omitempty"`
	Detail         map[string]any `json:"detail,omitempty"`
}

// RankedScore is a ClimberScore with a final placement (1-indexed).
// Climbers tied on every comparison get the same rank ("dense" ranking
// is intentional — IFSC-style standings show ties as e.g. T1, T1, 3
// rather than 1, 2, 3).
type RankedScore struct {
	ClimberScore
	Rank int `json:"rank"`
}

// Scorer is the contract every scoring rule implements.
//
// Score computes one climber's total. attempts may be empty (registered
// climber who hasn't logged anything); scorers should still return a
// well-formed zero-value ClimberScore with the registration ID set.
//
// Rank takes the full set of climber scores for one category and orders
// them. The same scorer instance handles both — it knows which fields it
// populated in Score and how to break ties.
type Scorer interface {
	Name() string
	Score(registrationID string, attempts []Attempt, problems []Problem, cfg json.RawMessage) (ClimberScore, error)
	Rank(scores []ClimberScore, cfg json.RawMessage) []RankedScore
}

// ── Registry ───────────────────────────────────────────────

var (
	registryMu sync.RWMutex
	registry   = map[string]Scorer{}
)

// Register adds a scorer to the global registry. Idiomatic call site is
// from a scorer's init(), which means scorers self-register on package
// import. Panics on duplicate Name() to surface accidental shadowing —
// scorer names are an external contract (saved in competitions.scoring_rule)
// and must stay stable.
func Register(s Scorer) {
	registryMu.Lock()
	defer registryMu.Unlock()
	name := s.Name()
	if name == "" {
		panic("competition: cannot register scorer with empty Name()")
	}
	if _, dup := registry[name]; dup {
		panic(fmt.Sprintf("competition: duplicate scorer registered for %q", name))
	}
	registry[name] = s
}

// Get returns the scorer registered under name, or (nil, false).
func Get(name string) (Scorer, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	s, ok := registry[name]
	return s, ok
}

// Names returns the registered scorer names. Intended for the API
// "list available scoring rules" endpoint and for tests.
func Names() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]string, 0, len(registry))
	for n := range registry {
		out = append(out, n)
	}
	return out
}

// reset clears the registry. Test-only — package code should not call this.
func reset() {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry = map[string]Scorer{}
}
