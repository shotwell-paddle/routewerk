package competition

import (
	"encoding/json"
	"sort"
)

// topZone implements IFSC-style count-based bouldering scoring:
//
//   Primary:    most tops, then most zones
//   Secondary:  fewest attempts to those tops
//   Tertiary:   fewest attempts to those zones
//   Tiebreak:   count-back across each climber's per-problem results,
//               sorted best-to-worst — the climber whose best send took
//               fewer attempts wins, then second-best, etc.
//
// Points is unused for ranking; we leave it at 0 in ClimberScore. The
// API layer surfaces (tops, zones, attempts_to_top, attempts_to_zone)
// directly. No config; cfg is ignored.
type topZone struct{}

func init() { Register(topZone{}) }

func (topZone) Name() string { return "top_zone" }

func (topZone) Score(registrationID string, attempts []Attempt, problems []Problem, _ json.RawMessage) (ClimberScore, error) {
	score := ClimberScore{RegistrationID: registrationID}
	pIdx := indexProblems(problems)

	for _, a := range attempts {
		p, ok := pIdx[a.ProblemID]
		if !ok {
			// Attempt referencing an unknown problem (deleted? wrong event?).
			// Skip rather than error so a stale row doesn't poison the
			// whole scorecard.
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

func (topZone) Rank(scores []ClimberScore, _ json.RawMessage) []RankedScore {
	sorted := make([]ClimberScore, len(scores))
	copy(sorted, scores)
	// SliceStable so a tie that count-back can't break preserves the
	// caller's input order (typically registration time) deterministically.
	sort.SliceStable(sorted, func(i, j int) bool {
		return cmpTopZone(sorted[i], sorted[j]) < 0
	})
	return assignStandardRanks(sorted, cmpTopZone)
}

// cmpTopZone returns negative when a ranks BETTER than b (should sort
// earlier), positive when worse, 0 when truly tied.
func cmpTopZone(a, b ClimberScore) int {
	if a.Tops != b.Tops {
		return b.Tops - a.Tops // more tops = better
	}
	if a.Zones != b.Zones {
		return b.Zones - a.Zones // more zones = better
	}
	if a.AttemptsToTop != b.AttemptsToTop {
		return a.AttemptsToTop - b.AttemptsToTop // fewer attempts = better
	}
	if a.AttemptsToZone != b.AttemptsToZone {
		return a.AttemptsToZone - b.AttemptsToZone
	}
	return cmpCountBack(a, b)
}

// cmpCountBack compares two climbers' per-problem results in
// best-to-worst order. The climber whose nth-best send is better wins
// the tiebreak. Returns 0 if every comparable position is identical.
func cmpCountBack(a, b ClimberScore) int {
	aResults := sortedByGoodnessDesc(a.PerProblem)
	bResults := sortedByGoodnessDesc(b.PerProblem)
	n := len(aResults)
	if len(bResults) < n {
		n = len(bResults)
	}
	for i := 0; i < n; i++ {
		if c := cmpGoodness(aResults[i], bResults[i]); c != 0 {
			return c
		}
	}
	return 0
}

// sortedByGoodnessDesc returns a copy sorted best-to-worst by goodness.
func sortedByGoodnessDesc(ps []ProblemScore) []ProblemScore {
	out := make([]ProblemScore, len(ps))
	copy(out, ps)
	sort.SliceStable(out, func(i, j int) bool {
		return cmpGoodness(out[i], out[j]) < 0
	})
	return out
}

// cmpGoodness ranks one climber's two problem results. Negative = a
// is better than b. Top with N attempts beats zone-only beats nothing;
// at the same tier, fewer attempts wins.
func cmpGoodness(a, b ProblemScore) int {
	aTier, bTier := goodnessTier(a), goodnessTier(b)
	if aTier != bTier {
		return bTier - aTier // higher tier = better
	}
	switch aTier {
	case 2: // both topped: compare attempts to top
		return a.Attempts - b.Attempts
	case 1: // both zone-only: compare zone attempts
		return derefIntOr(a.ZoneAttempts, 0) - derefIntOr(b.ZoneAttempts, 0)
	default:
		return 0
	}
}

// goodnessTier: 2 = topped, 1 = zone only, 0 = neither.
func goodnessTier(p ProblemScore) int {
	switch {
	case p.TopReached:
		return 2
	case p.ZoneReached:
		return 1
	default:
		return 0
	}
}
