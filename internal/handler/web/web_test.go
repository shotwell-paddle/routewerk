package webhandler

import (
	"testing"

	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

// ── buildGradeGroups ─────────────────────────────────────────

func TestBuildGradeGroups_VScale(t *testing.T) {
	dist := []repository.GradeCount{
		{GradingSystem: "v_scale", Grade: "VB", RouteType: "boulder", Count: 3},
		{GradingSystem: "v_scale", Grade: "V0", RouteType: "boulder", Count: 2},
		{GradingSystem: "v_scale", Grade: "V1", RouteType: "boulder", Count: 4},
		{GradingSystem: "v_scale", Grade: "V2", RouteType: "boulder", Count: 3},
		{GradingSystem: "v_scale", Grade: "V3", RouteType: "boulder", Count: 5},
		{GradingSystem: "v_scale", Grade: "V4", RouteType: "boulder", Count: 6},
		{GradingSystem: "v_scale", Grade: "V5", RouteType: "boulder", Count: 3},
		{GradingSystem: "v_scale", Grade: "V6", RouteType: "boulder", Count: 1},
		{GradingSystem: "v_scale", Grade: "V7", RouteType: "boulder", Count: 2},
		{GradingSystem: "v_scale", Grade: "V8", RouteType: "boulder", Count: 4},
		{GradingSystem: "v_scale", Grade: "V9", RouteType: "boulder", Count: 2},
		{GradingSystem: "v_scale", Grade: "V10", RouteType: "boulder", Count: 1},
	}

	groups := buildGradeGroups(dist, ptrLocSettings())

	// Should produce 5 V-scale buckets
	if len(groups) != 5 {
		t.Fatalf("expected 5 grade groups, got %d", len(groups))
	}

	expected := []struct {
		Value string
		Count int
	}{
		{"vb-v1", 9},  // VB(3) + V0(2) + V1(4)
		{"v2-v3", 8},  // V2(3) + V3(5)
		{"v4-v5", 9},  // V4(6) + V5(3)
		{"v6-v7", 3},  // V6(1) + V7(2)
		{"v8-up", 7},  // V8(4) + V9(2) + V10(1)
	}

	for i, exp := range expected {
		if groups[i].Value != exp.Value {
			t.Errorf("group[%d]: expected value %q, got %q", i, exp.Value, groups[i].Value)
		}
		if groups[i].Count != exp.Count {
			t.Errorf("group[%d] (%s): expected count %d, got %d", i, exp.Value, exp.Count, groups[i].Count)
		}
	}
}

func TestBuildGradeGroups_YDS(t *testing.T) {
	dist := []repository.GradeCount{
		{GradingSystem: "yds", Grade: "5.7", RouteType: "route", Count: 3},
		{GradingSystem: "yds", Grade: "5.8", RouteType: "route", Count: 4},
		{GradingSystem: "yds", Grade: "5.9", RouteType: "route", Count: 6},
		{GradingSystem: "yds", Grade: "5.10a", RouteType: "route", Count: 2},
		{GradingSystem: "yds", Grade: "5.10c", RouteType: "route", Count: 3},
		{GradingSystem: "yds", Grade: "5.11a", RouteType: "route", Count: 4},
		{GradingSystem: "yds", Grade: "5.11d", RouteType: "route", Count: 2},
		{GradingSystem: "yds", Grade: "5.12a", RouteType: "route", Count: 3},
		{GradingSystem: "yds", Grade: "5.13b", RouteType: "route", Count: 1},
	}

	groups := buildGradeGroups(dist, ptrLocSettings())

	if len(groups) != 5 {
		t.Fatalf("expected 5 grade groups, got %d", len(groups))
	}

	expected := []struct {
		Value string
		Count int
	}{
		{"5.8-under", 7}, // 5.7(3) + 5.8(4)
		{"5.9", 6},
		{"5.10", 5},      // 5.10a(2) + 5.10c(3)
		{"5.11", 6},      // 5.11a(4) + 5.11d(2)
		{"5.12-up", 4},   // 5.12a(3) + 5.13b(1)
	}

	for i, exp := range expected {
		if groups[i].Value != exp.Value {
			t.Errorf("group[%d]: expected value %q, got %q", i, exp.Value, groups[i].Value)
		}
		if groups[i].Count != exp.Count {
			t.Errorf("group[%d] (%s): expected count %d, got %d", i, exp.Value, exp.Count, groups[i].Count)
		}
	}
}

func TestBuildGradeGroups_Mixed(t *testing.T) {
	dist := []repository.GradeCount{
		{GradingSystem: "v_scale", Grade: "V4", RouteType: "boulder", Count: 5},
		{GradingSystem: "yds", Grade: "5.10a", RouteType: "route", Count: 3},
	}

	groups := buildGradeGroups(dist, ptrLocSettings())

	// V-scale comes first, then YDS
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if groups[0].Value != "v4-v5" {
		t.Errorf("first group should be v4-v5, got %s", groups[0].Value)
	}
	if groups[1].Value != "5.10" {
		t.Errorf("second group should be 5.10, got %s", groups[1].Value)
	}
}

func TestBuildGradeGroups_Empty(t *testing.T) {
	groups := buildGradeGroups(nil, ptrLocSettings())
	if len(groups) != 0 {
		t.Errorf("expected 0 groups for nil input, got %d", len(groups))
	}

	groups = buildGradeGroups([]repository.GradeCount{}, ptrLocSettings())
	if len(groups) != 0 {
		t.Errorf("expected 0 groups for empty input, got %d", len(groups))
	}
}

func TestBuildGradeGroups_SkipsEmptyBuckets(t *testing.T) {
	// Only V8+ data — should produce exactly 1 bucket
	dist := []repository.GradeCount{
		{GradingSystem: "v_scale", Grade: "V9", RouteType: "boulder", Count: 2},
	}

	groups := buildGradeGroups(dist, ptrLocSettings())
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].Value != "v8-up" || groups[0].Count != 2 {
		t.Errorf("expected v8-up:2, got %s:%d", groups[0].Value, groups[0].Count)
	}
}

// ── Template helpers ─────────────────────────────────────────

func TestSanitizeColor(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"#e53935", "#e53935"},
		{"#fff", "#fff"},
		{"#AABBCC", "#AABBCC"},
		{"", "#999999"},
		{"red", "#999999"},
		{"#gggggg", "#999999"},
		{"<script>", "#999999"},
		{"#e53935; background: red", "#999999"},
	}

	for _, tc := range tests {
		got := sanitizeColor(tc.input)
		if got != tc.expected {
			t.Errorf("sanitizeColor(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestTitleCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"red", "Red"},
		{"blue monday", "Blue Monday"},
		{"", ""},
		{"UPPER", "UPPER"},
		{"a", "A"},
	}

	for _, tc := range tests {
		got := titleCase(tc.input)
		if got != tc.expected {
			t.Errorf("titleCase(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

// relativeTime tests are in helpers_test.go

func TestAbsInt(t *testing.T) {
	if absInt(5) != 5 {
		t.Error("absInt(5) should be 5")
	}
	if absInt(-5) != 5 {
		t.Error("absInt(-5) should be 5")
	}
	if absInt(0) != 0 {
		t.Error("absInt(0) should be 0")
	}
}

func TestSeq(t *testing.T) {
	got := seq(1, 5)
	expected := []int{1, 2, 3, 4, 5}
	if len(got) != len(expected) {
		t.Fatalf("seq(1,5) length = %d, want %d", len(got), len(expected))
	}
	for i := range got {
		if got[i] != expected[i] {
			t.Errorf("seq(1,5)[%d] = %d, want %d", i, got[i], expected[i])
		}
	}
}

func TestRouteViewDisplayGrade(t *testing.T) {
	tests := []struct {
		name   string
		rv     RouteView
		expect string
	}{
		{
			name: "v_scale with circuit color shows grade (not color)",
			rv: RouteView{
				Route: routeWithGrading("v_scale", "V4", strPtr("blue")),
			},
			expect: "V4",
		},
		{
			name: "circuit with circuit color shows color",
			rv: RouteView{
				Route: routeWithGrading("circuit", "", strPtr("red")),
			},
			expect: "Red",
		},
		{
			name: "yds shows grade string",
			rv: RouteView{
				Route: routeWithGrading("yds", "5.10c", nil),
			},
			expect: "5.10c",
		},
		{
			name: "v_scale without circuit color shows grade",
			rv: RouteView{
				Route: routeWithGrading("v_scale", "V4", nil),
			},
			expect: "V4",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.rv.DisplayGrade()
			if got != tc.expect {
				t.Errorf("DisplayGrade() = %q, want %q", got, tc.expect)
			}
		})
	}
}

func TestRouteViewIsCircuit(t *testing.T) {
	rv := RouteView{Route: routeWithGrading("circuit", "", strPtr("red"))}
	if !rv.IsCircuit() {
		t.Error("IsCircuit() should be true for circuit grading system")
	}

	rv2 := RouteView{Route: routeWithGrading("v_scale", "V4", nil)}
	if rv2.IsCircuit() {
		t.Error("IsCircuit() should be false for v_scale grading system")
	}
}

func TestRouteViewSafeColor(t *testing.T) {
	rv := RouteView{Route: routeWithColor("#e53935")}
	if rv.SafeColor() != "#e53935" {
		t.Error("SafeColor should return valid hex")
	}

	rv2 := RouteView{Route: routeWithColor("javascript:alert(1)")}
	if rv2.SafeColor() != "#999999" {
		t.Error("SafeColor should sanitize injection attempt")
	}
}

func TestValidRouteTypes(t *testing.T) {
	// All expected route types should be valid
	for _, rt := range []string{"", "boulder", "sport", "top_rope"} {
		if !validRouteTypes[rt] {
			t.Errorf("expected %q to be a valid route type", rt)
		}
	}

	// Invalid types should be rejected
	for _, rt := range []string{"lead", "trad", "aid", "<script>"} {
		if validRouteTypes[rt] {
			t.Errorf("expected %q to be an invalid route type", rt)
		}
	}
}

func TestValidRouteIDPattern(t *testing.T) {
	valid := []string{
		"abc123",
		"a0000000-0000-4000-8000-000000000001",
		"route_1",
		"r-1",
	}
	for _, id := range valid {
		if !validRouteID.MatchString(id) {
			t.Errorf("expected %q to match validRouteID", id)
		}
	}

	invalid := []string{
		"",
		"../../../etc/passwd",
		"route id with spaces",
		"<script>alert(1)</script>",
	}
	for _, id := range invalid {
		if validRouteID.MatchString(id) {
			t.Errorf("expected %q to NOT match validRouteID", id)
		}
	}
}

// ── Test helpers ─────────────────────────────────────────────

func routeWithGrading(system, grade string, circuitColor *string) model.Route {
	return model.Route{
		GradingSystem: system,
		Grade:         grade,
		CircuitColor:  circuitColor,
		Color:         "#999999",
	}
}

func routeWithColor(color string) model.Route {
	return model.Route{Color: color}
}

func ptrLocSettings() *model.LocationSettings {
	s := model.DefaultLocationSettings()
	return &s
}
