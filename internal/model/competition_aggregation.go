package model

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ParseAggregation decodes a competition's aggregation jsonb. An empty or
// `{}` payload is allowed for single-event comps where aggregation is a
// no-op; callers should handle the zero-value Aggregation accordingly.
func ParseAggregation(raw json.RawMessage) (Aggregation, error) {
	var agg Aggregation
	if len(raw) == 0 {
		return agg, nil
	}
	// `{}` is fine — leaves all fields at zero value.
	if err := json.Unmarshal(raw, &agg); err != nil {
		return agg, fmt.Errorf("parse aggregation: %w", err)
	}
	return agg, nil
}

// Validate checks that the aggregation's method-specific fields are
// internally consistent. Eventless comps (status=draft) may skip this;
// the API enforces validity before transitioning to status=open.
//
// errAggregationMissingMethod is returned when the method is empty but
// other fields are set (likely a typo or a stripped field).
func (a Aggregation) Validate() error {
	switch a.Method {
	case "":
		// Empty method is OK only when no other fields are set; otherwise
		// the caller forgot to specify a method.
		if a.Drop != 0 || len(a.Weights) > 0 || a.FinalsEventID != nil || a.BestN != 0 {
			return errAggregationMissingMethod
		}
		return nil

	case AggMethodSum:
		// No extra fields required. Anything extra is silently allowed —
		// callers may carry method-specific config that other parts of the
		// system ignore.
		return nil

	case AggMethodSumDropN:
		if a.Drop <= 0 {
			return errors.New("aggregation: sum_drop_n requires drop > 0")
		}
		return nil

	case AggMethodWeightedFinals:
		if len(a.Weights) == 0 {
			return errors.New("aggregation: weighted_finals requires non-empty weights")
		}
		if a.FinalsEventID == nil || *a.FinalsEventID == "" {
			return errors.New("aggregation: weighted_finals requires finals_event_id")
		}
		return nil

	case AggMethodBestN:
		if a.BestN <= 0 {
			return errors.New("aggregation: best_n requires best_n > 0")
		}
		return nil

	default:
		return fmt.Errorf("aggregation: unknown method %q", a.Method)
	}
}

var errAggregationMissingMethod = errors.New("aggregation: method is required when other fields are set")
