package competition

import (
	"encoding/json"
	"fmt"
)

// fixed is the simplest scorer: each problem has a fixed point value;
// you get the points if you top it, otherwise zero. Total = sum.
//
// Optional config:
//   {"flash_bonus": 50}
// adds 50 points to any flash (top in 1 attempt). Default: 0.
//
// Ranking: total Points DESC, then AttemptsToTop ASC as a tiebreaker.
type fixed struct{}

func init() { Register(fixed{}) }

func (fixed) Name() string { return "fixed" }

type fixedConfig struct {
	FlashBonus float64 `json:"flash_bonus"`
}

func (fixed) Score(registrationID string, attempts []Attempt, problems []Problem, raw json.RawMessage) (ClimberScore, error) {
	cfg, err := parseConfig[fixedConfig](raw)
	if err != nil {
		return ClimberScore{}, fmt.Errorf("fixed scorer: parse config: %w", err)
	}
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
			pts := derefFloatOr(p.Points, 0)
			if a.Attempts == 1 {
				pts += cfg.FlashBonus
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

func (fixed) Rank(scores []ClimberScore, _ json.RawMessage) []RankedScore {
	return rankByPointsDescAttemptsAsc(scores)
}
