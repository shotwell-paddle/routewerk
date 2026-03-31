package webhandler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// ── parseRouteForm ─────────────────────────────────────────────────

func TestParseRouteForm_BasicBoulder(t *testing.T) {
	form := url.Values{
		"wall_id":        {"wall-1"},
		"setter_id":      {"setter-1"},
		"route_type":     {"boulder"},
		"grading_system": {"v_scale"},
		"grade":          {"V5"},
		"name":           {"  Cool Problem  "},
		"color":          {"#e53935"},
		"description":    {" A fun boulder problem "},
		"date_set":       {"2026-03-15"},
	}

	r := httptest.NewRequest(http.MethodPost, "/routes", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	fv := parseRouteForm(r)

	if fv.WallID != "wall-1" {
		t.Errorf("WallID = %q, want %q", fv.WallID, "wall-1")
	}
	if fv.SetterID != "setter-1" {
		t.Errorf("SetterID = %q, want %q", fv.SetterID, "setter-1")
	}
	if fv.RouteType != "boulder" {
		t.Errorf("RouteType = %q, want %q", fv.RouteType, "boulder")
	}
	if fv.GradingSystem != "v_scale" {
		t.Errorf("GradingSystem = %q, want %q", fv.GradingSystem, "v_scale")
	}
	if fv.Grade != "V5" {
		t.Errorf("Grade = %q, want %q", fv.Grade, "V5")
	}
	// Name should be trimmed
	if fv.Name != "Cool Problem" {
		t.Errorf("Name = %q, want %q", fv.Name, "Cool Problem")
	}
	if fv.Color != "#e53935" {
		t.Errorf("Color = %q, want %q", fv.Color, "#e53935")
	}
	// Description should be trimmed
	if fv.Description != "A fun boulder problem" {
		t.Errorf("Description = %q, want %q", fv.Description, "A fun boulder problem")
	}
	if fv.DateSet != "2026-03-15" {
		t.Errorf("DateSet = %q, want %q", fv.DateSet, "2026-03-15")
	}
	if fv.TagIDs == nil {
		t.Fatal("TagIDs should be initialized, not nil")
	}
}

func TestParseRouteForm_BoulderCircuit(t *testing.T) {
	form := url.Values{
		"wall_id":          {"wall-1"},
		"route_type":       {"boulder_circuit"},
		"grade":            {"red"},
		"circuit_vgrade":   {"V3"},
	}

	r := httptest.NewRequest(http.MethodPost, "/routes", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	fv := parseRouteForm(r)

	// boulder_circuit should be decomposed
	if fv.RouteType != "boulder" {
		t.Errorf("RouteType = %q, want %q", fv.RouteType, "boulder")
	}
	if fv.GradingSystem != "circuit" {
		t.Errorf("GradingSystem = %q, want %q", fv.GradingSystem, "circuit")
	}
	if fv.CircuitColor != "red" {
		t.Errorf("CircuitColor = %q, want %q", fv.CircuitColor, "red")
	}
	// V-grade from the optional field should be used as the display grade
	if fv.Grade != "V3" {
		t.Errorf("Grade = %q, want %q (should be circuit_vgrade)", fv.Grade, "V3")
	}
}

func TestParseRouteForm_BoulderCircuit_NoVGrade(t *testing.T) {
	form := url.Values{
		"wall_id":    {"wall-1"},
		"route_type": {"boulder_circuit"},
		"grade":      {"blue"},
	}

	r := httptest.NewRequest(http.MethodPost, "/routes", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	fv := parseRouteForm(r)

	if fv.RouteType != "boulder" {
		t.Errorf("RouteType = %q, want %q", fv.RouteType, "boulder")
	}
	if fv.GradingSystem != "circuit" {
		t.Errorf("GradingSystem = %q, want %q", fv.GradingSystem, "circuit")
	}
	if fv.CircuitColor != "blue" {
		t.Errorf("CircuitColor = %q, want %q", fv.CircuitColor, "blue")
	}
	// Without circuit_vgrade, grade stays as the color name
	// (the form puts the color in the grade field for circuit)
}

func TestParseRouteForm_Route(t *testing.T) {
	form := url.Values{
		"wall_id":    {"wall-2"},
		"route_type": {"route"},
		"grade":      {"5.10+"},
		"name":       {"The Crux"},
	}

	r := httptest.NewRequest(http.MethodPost, "/routes", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	fv := parseRouteForm(r)

	if fv.RouteType != "route" {
		t.Errorf("RouteType = %q, want %q (should stay as route)", fv.RouteType, "route")
	}
	if fv.GradingSystem != "yds" {
		t.Errorf("GradingSystem = %q, want %q (route type should default to yds)", fv.GradingSystem, "yds")
	}
	if fv.Grade != "5.10+" {
		t.Errorf("Grade = %q, want %q", fv.Grade, "5.10+")
	}
}

func TestParseRouteForm_UnknownRouteType_DefaultsToBoulder(t *testing.T) {
	form := url.Values{
		"route_type": {"something_weird"},
		"grade":      {"V4"},
	}

	r := httptest.NewRequest(http.MethodPost, "/routes", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	fv := parseRouteForm(r)

	if fv.RouteType != "boulder" {
		t.Errorf("RouteType = %q, want %q (unknown should default to boulder)", fv.RouteType, "boulder")
	}
	if fv.GradingSystem != "v_scale" {
		t.Errorf("GradingSystem = %q, want %q (should default to v_scale)", fv.GradingSystem, "v_scale")
	}
}

func TestParseRouteForm_EmptyRouteType_DefaultsToBoulder(t *testing.T) {
	form := url.Values{
		"grade": {"V2"},
	}

	r := httptest.NewRequest(http.MethodPost, "/routes", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	fv := parseRouteForm(r)

	if fv.RouteType != "boulder" {
		t.Errorf("RouteType = %q, want %q", fv.RouteType, "boulder")
	}
}

func TestParseRouteForm_Tags(t *testing.T) {
	form := url.Values{
		"route_type": {"boulder"},
		"grade":      {"V5"},
		"tag_ids":    {"tag-1", "tag-2", "tag-3"},
	}

	r := httptest.NewRequest(http.MethodPost, "/routes", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	fv := parseRouteForm(r)

	if len(fv.TagIDs) != 3 {
		t.Fatalf("TagIDs len = %d, want 3", len(fv.TagIDs))
	}
	for _, id := range []string{"tag-1", "tag-2", "tag-3"} {
		if !fv.TagIDs[id] {
			t.Errorf("TagIDs missing %q", id)
		}
	}
}

func TestParseRouteForm_ProjectedStripDate(t *testing.T) {
	form := url.Values{
		"route_type":           {"boulder"},
		"grade":                {"V3"},
		"projected_strip_date": {"2026-06-15"},
	}

	r := httptest.NewRequest(http.MethodPost, "/routes", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	fv := parseRouteForm(r)

	if fv.ProjectedStripDate != "2026-06-15" {
		t.Errorf("ProjectedStripDate = %q, want %q", fv.ProjectedStripDate, "2026-06-15")
	}
}

func TestParseRouteForm_CircuitPreservesGradingSystem(t *testing.T) {
	// When route_type is "boulder" and grading_system is "circuit",
	// the grading system should be preserved (not overridden to v_scale)
	form := url.Values{
		"route_type":     {"boulder"},
		"grading_system": {"circuit"},
		"grade":          {"green"},
	}

	r := httptest.NewRequest(http.MethodPost, "/routes", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	fv := parseRouteForm(r)

	if fv.GradingSystem != "circuit" {
		t.Errorf("GradingSystem = %q, want %q (circuit should be preserved)", fv.GradingSystem, "circuit")
	}
}
