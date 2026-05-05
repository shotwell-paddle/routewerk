package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseAggregation(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantErr  bool
		wantMeth string
	}{
		{name: "empty bytes", input: "", wantMeth: ""},
		{name: "empty object", input: `{}`, wantMeth: ""},
		{name: "sum", input: `{"method":"sum"}`, wantMeth: "sum"},
		{
			name:     "weighted finals",
			input:    `{"method":"weighted_finals","weights":[1,1,1,2],"finals_event_id":"abc"}`,
			wantMeth: "weighted_finals",
		},
		{name: "malformed", input: `{not json`, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			agg, err := ParseAggregation(json.RawMessage(tc.input))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got nil (parsed: %+v)", agg)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if agg.Method != tc.wantMeth {
				t.Errorf("Method = %q, want %q", agg.Method, tc.wantMeth)
			}
		})
	}
}

func TestAggregationValidate(t *testing.T) {
	id := "evt-123"
	tests := []struct {
		name    string
		agg     Aggregation
		wantErr string // substring; "" = expect no error
	}{
		{name: "empty all-zero is ok", agg: Aggregation{}},
		{name: "method missing with extras set", agg: Aggregation{Drop: 1}, wantErr: "method is required"},
		{name: "sum needs nothing", agg: Aggregation{Method: AggMethodSum}},
		{name: "sum_drop_n requires drop", agg: Aggregation{Method: AggMethodSumDropN}, wantErr: "drop > 0"},
		{name: "sum_drop_n with drop=1 ok", agg: Aggregation{Method: AggMethodSumDropN, Drop: 1}},
		{name: "weighted_finals requires weights", agg: Aggregation{Method: AggMethodWeightedFinals, FinalsEventID: &id}, wantErr: "non-empty weights"},
		{name: "weighted_finals requires finals event id", agg: Aggregation{Method: AggMethodWeightedFinals, Weights: []int{1, 1, 2}}, wantErr: "finals_event_id"},
		{name: "weighted_finals complete is ok", agg: Aggregation{Method: AggMethodWeightedFinals, Weights: []int{1, 1, 2}, FinalsEventID: &id}},
		{name: "best_n requires best_n", agg: Aggregation{Method: AggMethodBestN}, wantErr: "best_n > 0"},
		{name: "best_n with value ok", agg: Aggregation{Method: AggMethodBestN, BestN: 3}},
		{name: "unknown method", agg: Aggregation{Method: "speed_chess"}, wantErr: "unknown method"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.agg.Validate()
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("want no error, got %v", err)
			case tc.wantErr != "" && err == nil:
				t.Fatalf("want error containing %q, got nil", tc.wantErr)
			case tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr):
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestCompetitionEventEffectiveScoring(t *testing.T) {
	override := "decay"
	overrideCfg := json.RawMessage(`{"decay_rate":0.2}`)
	compRule := "top_zone"
	compCfg := json.RawMessage(`{"flash_bonus":50}`)

	tests := []struct {
		name     string
		event    CompetitionEvent
		wantRule string
		wantCfg  string
	}{
		{
			name:     "no override falls through",
			event:    CompetitionEvent{},
			wantRule: compRule,
			wantCfg:  string(compCfg),
		},
		{
			name:     "override with config",
			event:    CompetitionEvent{ScoringRuleOverride: &override, ScoringConfigOverride: overrideCfg},
			wantRule: override,
			wantCfg:  string(overrideCfg),
		},
		{
			name: "override with no config",
			event: CompetitionEvent{
				ScoringRuleOverride: &override,
				// ScoringConfigOverride intentionally nil — overrides are
				// explicit, so an empty config means "no config" not
				// "fall through to comp config".
			},
			wantRule: override,
			wantCfg:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.event.EffectiveScoringRule(compRule); got != tc.wantRule {
				t.Errorf("EffectiveScoringRule = %q, want %q", got, tc.wantRule)
			}
			if got := string(tc.event.EffectiveScoringConfig(compCfg)); got != tc.wantCfg {
				t.Errorf("EffectiveScoringConfig = %q, want %q", got, tc.wantCfg)
			}
		})
	}
}

func TestRegistrationIsActive(t *testing.T) {
	r := CompetitionRegistration{}
	if !r.IsActive() {
		t.Error("zero-value registration should be active")
	}
	// Use the pgtype.Timestamptz Valid flag rather than constructing a time
	// — the helper only cares that WithdrawnAt is set.
	r.WithdrawnAt.Valid = true
	if r.IsActive() {
		t.Error("withdrawn registration should not be active")
	}
}

