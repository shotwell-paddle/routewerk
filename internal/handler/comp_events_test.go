package handler

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/shotwell-paddle/routewerk/internal/api"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

func float32p(f float32) *float32 { return &f }

// ── validateEventCreate ────────────────────────────────────

func TestValidateEventCreate(t *testing.T) {
	now := time.Now()
	later := now.Add(time.Hour)
	base := func() api.EventCreate {
		return api.EventCreate{
			Name: "Week 1", Sequence: 1, StartsAt: now, EndsAt: later,
		}
	}
	tests := []struct {
		name    string
		mutate  func(*api.EventCreate)
		wantErr string
	}{
		{name: "valid", mutate: func(e *api.EventCreate) {}},
		{name: "missing name", mutate: func(e *api.EventCreate) { e.Name = "" }, wantErr: "name"},
		{name: "zero sequence", mutate: func(e *api.EventCreate) { e.Sequence = 0 }, wantErr: "sequence"},
		{name: "negative sequence", mutate: func(e *api.EventCreate) { e.Sequence = -1 }, wantErr: "sequence"},
		{name: "ends before starts", mutate: func(e *api.EventCreate) { e.EndsAt = e.StartsAt.Add(-time.Hour) }, wantErr: "ends_at"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ev := base()
			tc.mutate(&ev)
			err := validateEventCreate(&ev)
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

// ── eventCreateToModel ─────────────────────────────────────

func TestEventCreateToModel(t *testing.T) {
	compID := uuid.New().String()
	now := time.Now()

	t.Run("defaults", func(t *testing.T) {
		ev := eventCreateToModel(compID, &api.EventCreate{
			Name: "x", Sequence: 1, StartsAt: now, EndsAt: now.Add(time.Hour),
		})
		if ev.Weight != 1.0 {
			t.Errorf("Weight = %v, want default 1.0", ev.Weight)
		}
		if ev.ScoringRuleOverride != nil {
			t.Errorf("ScoringRuleOverride = %v, want nil by default", ev.ScoringRuleOverride)
		}
	})

	t.Run("with weight + override", func(t *testing.T) {
		w := float32(2.5)
		rule := "decay"
		cfg := map[string]interface{}{"base_points": float64(500)}
		ev := eventCreateToModel(compID, &api.EventCreate{
			Name: "x", Sequence: 1, StartsAt: now, EndsAt: now.Add(time.Hour),
			Weight:                &w,
			ScoringRuleOverride:   &rule,
			ScoringConfigOverride: &cfg,
		})
		if ev.Weight != 2.5 {
			t.Errorf("Weight = %v, want 2.5", ev.Weight)
		}
		if ev.ScoringRuleOverride == nil || *ev.ScoringRuleOverride != "decay" {
			t.Errorf("ScoringRuleOverride = %v, want 'decay'", ev.ScoringRuleOverride)
		}
		var roundtrip map[string]interface{}
		_ = json.Unmarshal(ev.ScoringConfigOverride, &roundtrip)
		if roundtrip["base_points"] != float64(500) {
			t.Errorf("ScoringConfigOverride didn't roundtrip: %+v", roundtrip)
		}
	})

	t.Run("empty rule string treated as no override", func(t *testing.T) {
		empty := ""
		ev := eventCreateToModel(compID, &api.EventCreate{
			Name: "x", Sequence: 1, StartsAt: now, EndsAt: now.Add(time.Hour),
			ScoringRuleOverride: &empty,
		})
		if ev.ScoringRuleOverride != nil {
			t.Errorf("empty override string should be nil; got %v", ev.ScoringRuleOverride)
		}
	})
}

// ── applyEventUpdate ───────────────────────────────────────

func TestApplyEventUpdate(t *testing.T) {
	now := time.Now()
	existing := &model.CompetitionEvent{
		Name: "Old", Sequence: 1, StartsAt: now, EndsAt: now.Add(time.Hour), Weight: 1.0,
	}
	newName := "New"
	newSeq := 3
	w := float32(1.5)
	if err := applyEventUpdate(existing, &api.EventUpdate{
		Name: &newName, Sequence: &newSeq, Weight: &w,
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if existing.Name != "New" || existing.Sequence != 3 || existing.Weight != 1.5 {
		t.Errorf("update did not apply: %+v", existing)
	}
}

func TestApplyEventUpdate_RejectsBadInputs(t *testing.T) {
	now := time.Now()
	mk := func() *model.CompetitionEvent {
		return &model.CompetitionEvent{
			Name: "Old", Sequence: 1, StartsAt: now, EndsAt: now.Add(time.Hour),
		}
	}
	t.Run("empty name", func(t *testing.T) {
		empty := ""
		if err := applyEventUpdate(mk(), &api.EventUpdate{Name: &empty}); err == nil {
			t.Error("expected error on empty name")
		}
	})
	t.Run("zero sequence", func(t *testing.T) {
		zero := 0
		if err := applyEventUpdate(mk(), &api.EventUpdate{Sequence: &zero}); err == nil {
			t.Error("expected error on zero sequence")
		}
	})
	t.Run("ends before starts", func(t *testing.T) {
		earlier := now.Add(-time.Hour)
		if err := applyEventUpdate(mk(), &api.EventUpdate{EndsAt: &earlier}); err == nil {
			t.Error("expected error on ends_at < starts_at")
		}
	})
	t.Run("clear override with empty string", func(t *testing.T) {
		ev := mk()
		rule := "decay"
		ev.ScoringRuleOverride = &rule
		empty := ""
		if err := applyEventUpdate(ev, &api.EventUpdate{ScoringRuleOverride: &empty}); err != nil {
			t.Fatalf("apply: %v", err)
		}
		if ev.ScoringRuleOverride != nil {
			t.Errorf("override should be cleared by empty string; got %v", ev.ScoringRuleOverride)
		}
	})
}

// ── eventToAPI ─────────────────────────────────────────────

func TestEventToAPI_RoundTrip(t *testing.T) {
	id := uuid.New().String()
	cid := uuid.New().String()
	now := time.Now().UTC().Truncate(time.Second)
	rule := "fixed"
	in := &model.CompetitionEvent{
		ID: id, CompetitionID: cid, Name: "x", Sequence: 2,
		StartsAt: now, EndsAt: now.Add(time.Hour), Weight: 2.0,
		ScoringRuleOverride:   &rule,
		ScoringConfigOverride: json.RawMessage(`{"flash_bonus":50}`),
	}
	out, err := eventToAPI(in)
	if err != nil {
		t.Fatalf("eventToAPI: %v", err)
	}
	if out.Id.String() != id || out.CompetitionId.String() != cid {
		t.Errorf("ID roundtrip failed")
	}
	if out.Sequence != 2 || out.Weight != 2.0 {
		t.Errorf("sequence/weight: %d / %v", out.Sequence, out.Weight)
	}
	if out.ScoringRuleOverride == nil || *out.ScoringRuleOverride != "fixed" {
		t.Errorf("override roundtrip failed: %+v", out.ScoringRuleOverride)
	}
}

// ── categoryCreateToModel + categoryToAPI ──────────────────

func TestCategoryCreateToModel_Defaults(t *testing.T) {
	cat := categoryCreateToModel(uuid.New().String(), &api.CategoryCreate{Name: "Open"})
	if cat.SortOrder != 0 {
		t.Errorf("SortOrder = %d, want default 0", cat.SortOrder)
	}
	if string(cat.Rules) != "{}" {
		t.Errorf("Rules = %s, want '{}'", cat.Rules)
	}
}

func TestCategoryToAPI_RoundTrip(t *testing.T) {
	id := uuid.New().String()
	cid := uuid.New().String()
	in := &model.CompetitionCategory{
		ID: id, CompetitionID: cid, Name: "Masters", SortOrder: 5,
		Rules: json.RawMessage(`{"min_age":40}`),
	}
	out, err := categoryToAPI(in)
	if err != nil {
		t.Fatalf("categoryToAPI: %v", err)
	}
	if out.Name != "Masters" || out.SortOrder != 5 {
		t.Errorf("fields: %+v", out)
	}
	if out.Rules["min_age"] != float64(40) {
		t.Errorf("Rules min_age = %v, want 40", out.Rules["min_age"])
	}
}

// ── problemCreateToModel + problemToAPI ────────────────────

func TestProblemCreateToModel(t *testing.T) {
	rid := uuid.New()
	color := "yellow"
	pts := float32(100)
	zone := float32(50)
	sort := 7
	p := problemCreateToModel(uuid.New().String(), &api.ProblemCreate{
		Label:      "M1",
		RouteId:    &rid,
		Points:     &pts,
		ZonePoints: &zone,
		Color:      &color,
		SortOrder:  &sort,
	})
	if p.Label != "M1" {
		t.Errorf("Label = %q", p.Label)
	}
	if p.RouteID == nil || *p.RouteID != rid.String() {
		t.Errorf("RouteID = %v, want %s", p.RouteID, rid)
	}
	if p.Points == nil || *p.Points != 100 {
		t.Errorf("Points = %v", p.Points)
	}
	if p.SortOrder != 7 {
		t.Errorf("SortOrder = %d", p.SortOrder)
	}
}

func TestApplyProblemUpdate_PartialFields(t *testing.T) {
	existing := &model.CompetitionProblem{Label: "Old", SortOrder: 1}
	newLabel := "New"
	newSort := 9
	applyProblemUpdate(existing, &api.ProblemUpdate{Label: &newLabel, SortOrder: &newSort})
	if existing.Label != "New" || existing.SortOrder != 9 {
		t.Errorf("update did not apply: %+v", existing)
	}
}

func TestProblemToAPI_RoundTrip(t *testing.T) {
	id := uuid.New().String()
	eid := uuid.New().String()
	pts := float64(200)
	in := &model.CompetitionProblem{
		ID: id, EventID: eid, Label: "B5", SortOrder: 5,
		Points: &pts,
	}
	out, err := problemToAPI(in)
	if err != nil {
		t.Fatalf("problemToAPI: %v", err)
	}
	if out.Label != "B5" || out.SortOrder != 5 {
		t.Errorf("fields: %+v", out)
	}
	if out.Points == nil || *out.Points != 200 {
		t.Errorf("Points = %v", out.Points)
	}
}
