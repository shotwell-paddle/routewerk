package competition

import (
	"encoding/json"
	"fmt"
)

// decay rewards both sending a problem AND sending it cleanly: each top
// is worth `base_points / (1 + decay_rate * (attempts - 1))`, plus an
// optional flash bonus. Total = sum across topped problems.
//
// Config (with defaults if omitted):
//   {"base_points": 1000, "decay_rate": 0.1, "flash_bonus": 0}
//
// Examples with default base=1000, decay=0.1:
//   1 attempt → 1000           (flash; gets flash_bonus too if set)
//   2 attempts → ~909
//   5 attempts → ~714
//   10 attempts → ~526
//
// Setting decay_rate = 0 collapses to flat per-problem scoring (similar
// to `fixed` but with a global base instead of per-problem points).
//
// Ranking: total Points DESC, then AttemptsToTop ASC as tiebreaker.
type decay struct{}

func init() { Register(decay{}) }

func (decay) Name() string { return "decay" }

// Pointer fields distinguish "absent" (apply default) from "explicit zero"
// (caller intentionally chose flat scoring or no flash bonus). Without
// pointers, a missing decay_rate would silently default to 0 and the comp
// would score every send the same — a footgun.
type decayConfig struct {
	BasePoints *float64 `json:"base_points,omitempty"`
	DecayRate  *float64 `json:"decay_rate,omitempty"`
	FlashBonus *float64 `json:"flash_bonus,omitempty"`
}

const (
	defaultDecayBase = 1000.0
	defaultDecayRate = 0.1
)

// resolved unpacks a decayConfig into concrete values with defaults
// applied. Returns base, rate, flashBonus.
func (c decayConfig) resolved() (float64, float64, float64) {
	return derefFloatOr(c.BasePoints, defaultDecayBase),
		derefFloatOr(c.DecayRate, defaultDecayRate),
		derefFloatOr(c.FlashBonus, 0)
}

func (decay) Score(registrationID string, attempts []Attempt, problems []Problem, raw json.RawMessage) (ClimberScore, error) {
	cfg, err := parseConfig[decayConfig](raw)
	if err != nil {
		return ClimberScore{}, fmt.Errorf("decay scorer: parse config: %w", err)
	}
	base, rate, flashBonus := cfg.resolved()

	pIdx := indexProblems(problems)
	score := ClimberScore{RegistrationID: registrationID}

	for _, a := range attempts {
		p, ok := pIdx[a.ProblemID]
		if !ok {
			continue
		}
		ps := ProblemScore{
			ProblemID:    a.ProblemID,
			Attempts:     a.Attempts,
			ZoneAttempts: a.ZoneAttempts,
			TopReached:   a.TopReached,
			ZoneReached:  a.ZoneReached,
			SortOrder:    p.SortOrder,
		}
		if a.TopReached {
			pts := decayPoints(base, rate, a.Attempts)
			if a.Attempts == 1 {
				pts += flashBonus
			}
			ps.Points = pts
			score.Points += pts
			score.Tops++
			score.AttemptsToTop += a.Attempts
		}
		if a.ZoneReached {
			score.Zones++
			if a.ZoneAttempts != nil {
				score.AttemptsToZone += *a.ZoneAttempts
			}
		}
		score.PerProblem = append(score.PerProblem, ps)
	}
	return score, nil
}

func decayPoints(base, rate float64, attempts int) float64 {
	if attempts < 1 {
		// Defensive: top_reached without attempts should be impossible —
		// every send takes at least one try — but guard against bad data.
		attempts = 1
	}
	return base / (1 + rate*float64(attempts-1))
}

func (decay) Rank(scores []ClimberScore, _ json.RawMessage) []RankedScore {
	return rankByPointsDescAttemptsAsc(scores)
}
