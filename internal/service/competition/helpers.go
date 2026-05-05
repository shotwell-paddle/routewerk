package competition

import (
	"encoding/json"
	"sort"
)

// indexProblems is a tiny ID→Problem lookup table. Scorers use it to skip
// attempt rows whose problem has been deleted.
func indexProblems(problems []Problem) map[string]Problem {
	out := make(map[string]Problem, len(problems))
	for _, p := range problems {
		out[p.ID] = p
	}
	return out
}

func derefIntOr(p *int, fallback int) int {
	if p == nil {
		return fallback
	}
	return *p
}

func derefFloatOr(p *float64, fallback float64) float64 {
	if p == nil {
		return fallback
	}
	return *p
}

// rankByPointsDescAttemptsAsc sorts climbers by total Points DESC, with
// AttemptsToTop ASC as the tiebreaker. Used by `fixed` and `decay` —
// scorers where the per-climber Points field carries the full ranking
// signal (no count-back).
func rankByPointsDescAttemptsAsc(scores []ClimberScore) []RankedScore {
	sorted := make([]ClimberScore, len(scores))
	copy(sorted, scores)
	sort.SliceStable(sorted, func(i, j int) bool {
		return cmpPointsDesc(sorted[i], sorted[j]) < 0
	})
	return assignStandardRanks(sorted, cmpPointsDesc)
}

func cmpPointsDesc(a, b ClimberScore) int {
	if a.Points != b.Points {
		// More points = better. Use a sign rather than a cast to avoid
		// floating-point overflow on extreme values.
		if a.Points > b.Points {
			return -1
		}
		return 1
	}
	if a.AttemptsToTop != b.AttemptsToTop {
		return a.AttemptsToTop - b.AttemptsToTop // fewer attempts = better
	}
	return 0
}

// assignStandardRanks expects scores already sorted best-first and
// produces RankedScore[] using "standard" (1224) ranking — climbers tied
// on every field share the lowest of their tied positions, and the next
// distinct climber's rank skips ahead.
//
// Example: A and B tied at the top, C distinct, D tied with C →
//   A:1, B:1, C:3, D:3, E:5, …
//
// cmp is the comparator used to detect ties. It must return 0 for tied
// climbers (i.e. the same comparator that produced the sort order).
func assignStandardRanks(sorted []ClimberScore, cmp func(a, b ClimberScore) int) []RankedScore {
	out := make([]RankedScore, len(sorted))
	for i, s := range sorted {
		out[i] = RankedScore{ClimberScore: s, Rank: i + 1}
	}
	for i := 1; i < len(out); i++ {
		if cmp(out[i].ClimberScore, out[i-1].ClimberScore) == 0 {
			out[i].Rank = out[i-1].Rank
		}
	}
	return out
}

// parseConfig is a tiny convenience around json.Unmarshal that treats an
// empty payload as "use defaults" (return the zero value, no error).
// Scorers call this with their config struct as the target.
func parseConfig[T any](raw json.RawMessage) (T, error) {
	var cfg T
	if len(raw) == 0 {
		return cfg, nil
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}
