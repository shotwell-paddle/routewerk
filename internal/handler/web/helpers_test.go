package webhandler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

// ── ydsGradeBucket ──────────────────────────────────────────

func TestYdsGradeBucket(t *testing.T) {
	tests := []struct {
		grade  string
		expect string
	}{
		// 5.8 and under
		{"5.5", "5.8-under"},
		{"5.6", "5.8-under"},
		{"5.7", "5.8-under"},
		{"5.8", "5.8-under"},
		{"5.8-", "5.8-under"},
		{"5.8+", "5.8-under"},

		// 5.9 bucket
		{"5.9", "5.9"},
		{"5.9-", "5.9"},
		{"5.9+", "5.9"},
		{"5.9a", "5.9"},
		{"5.9b", "5.9"},
		{"5.9c", "5.9"},
		{"5.9d", "5.9"},

		// 5.10 bucket
		{"5.10", "5.10"},
		{"5.10a", "5.10"},
		{"5.10b", "5.10"},
		{"5.10c", "5.10"},
		{"5.10d", "5.10"},
		{"5.10-", "5.10"},
		{"5.10+", "5.10"},

		// 5.11 bucket
		{"5.11", "5.11"},
		{"5.11a", "5.11"},
		{"5.11d", "5.11"},
		{"5.11-", "5.11"},
		{"5.11+", "5.11"},

		// 5.12+ bucket
		{"5.12", "5.12-up"},
		{"5.12a", "5.12-up"},
		{"5.13b", "5.12-up"},
		{"5.14a", "5.12-up"},
		{"5.15a", "5.12-up"},

		// Edge cases
		{"", "5.8-under"},     // empty grade
		{"V4", "5.8-under"},   // wrong grading system
		{"4.10", "5.8-under"}, // wrong prefix
	}

	for _, tc := range tests {
		t.Run(tc.grade, func(t *testing.T) {
			got := ydsGradeBucket(tc.grade)
			if got != tc.expect {
				t.Errorf("ydsGradeBucket(%q) = %q, want %q", tc.grade, got, tc.expect)
			}
		})
	}
}

// ── expandGradeRange ────────────────────────────────────────

func TestExpandGradeRange(t *testing.T) {
	tests := []struct {
		value     string
		expectLen int
		contains  []string // spot-check elements
	}{
		{"vb-v1", 3, []string{"VB", "V0", "V1"}},
		{"v2-v3", 2, []string{"V2", "V3"}},
		{"v4-v5", 2, []string{"V4", "V5"}},
		{"v6-v7", 2, []string{"V6", "V7"}},
		{"v8-up", 10, []string{"V8", "V10", "V17"}},
		{"5.8-under", 12, []string{"5.5", "5.7+", "5.8-", "5.8+"}},
		{"5.9", 3, []string{"5.9-", "5.9", "5.9+"}},
		{"5.10", 7, []string{"5.10-", "5.10+", "5.10a", "5.10d"}},
		{"5.11", 7, []string{"5.11-", "5.11+", "5.11a", "5.11d"}},
		{"5.12-up", 25, []string{"5.12-", "5.13a", "5.14d", "5.15d"}},
	}

	for _, tc := range tests {
		t.Run(tc.value, func(t *testing.T) {
			got := expandGradeRange(tc.value)
			if len(got) != tc.expectLen {
				t.Errorf("expandGradeRange(%q) len = %d, want %d", tc.value, len(got), tc.expectLen)
			}
			m := make(map[string]bool)
			for _, g := range got {
				m[g] = true
			}
			for _, expect := range tc.contains {
				if !m[expect] {
					t.Errorf("expandGradeRange(%q) missing %q", tc.value, expect)
				}
			}
		})
	}
}

func TestExpandGradeRange_UnknownValue(t *testing.T) {
	got := expandGradeRange("unknown")
	if got != nil {
		t.Errorf("expandGradeRange(unknown) = %v, want nil", got)
	}
}

func TestExpandGradeRange_EmptyValue(t *testing.T) {
	got := expandGradeRange("")
	if got != nil {
		t.Errorf("expandGradeRange(\"\") = %v, want nil", got)
	}
}

// ── buildGradeGroups with circuits ──────────────────────────

func TestBuildGradeGroups_CircuitColors(t *testing.T) {
	settings := model.DefaultLocationSettings()
	settings.Circuits.Colors = []model.CircuitColor{
		{Name: "green", Hex: "#2e7d32"},
		{Name: "blue", Hex: "#1565c0"},
		{Name: "red", Hex: "#e53935"},
	}

	dist := []repository.GradeCount{
		{GradingSystem: "circuit", Grade: "red", RouteType: "boulder", Count: 5},
		{GradingSystem: "circuit", Grade: "blue", RouteType: "boulder", Count: 3},
		{GradingSystem: "circuit", Grade: "green", RouteType: "boulder", Count: 7},
	}

	groups := buildGradeGroups(dist, &settings)

	// Circuit groups should be first, in the gym's configured order
	if len(groups) != 3 {
		t.Fatalf("expected 3 circuit groups, got %d", len(groups))
	}

	// Order should follow settings: green, blue, red
	expected := []struct {
		Value string
		Count int
		Color string
	}{
		{"circuit:green", 7, "#2e7d32"},
		{"circuit:blue", 3, "#1565c0"},
		{"circuit:red", 5, "#e53935"},
	}

	for i, exp := range expected {
		if groups[i].Value != exp.Value {
			t.Errorf("group[%d].Value = %q, want %q", i, groups[i].Value, exp.Value)
		}
		if groups[i].Count != exp.Count {
			t.Errorf("group[%d].Count = %d, want %d", i, groups[i].Count, exp.Count)
		}
		if groups[i].Color != exp.Color {
			t.Errorf("group[%d].Color = %q, want %q", i, groups[i].Color, exp.Color)
		}
		if !groups[i].IsColor {
			t.Errorf("group[%d].IsColor should be true", i)
		}
	}
}

func TestBuildGradeGroups_CircuitsBeforeGrades(t *testing.T) {
	settings := model.DefaultLocationSettings()
	settings.Circuits.Colors = []model.CircuitColor{
		{Name: "red", Hex: "#e53935"},
	}

	dist := []repository.GradeCount{
		{GradingSystem: "v_scale", Grade: "V3", RouteType: "boulder", Count: 10},
		{GradingSystem: "circuit", Grade: "red", RouteType: "boulder", Count: 5},
		{GradingSystem: "yds", Grade: "5.10a", RouteType: "route", Count: 3},
	}

	groups := buildGradeGroups(dist, &settings)

	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(groups))
	}

	// Order: circuit first, then V-scale, then YDS
	if groups[0].Value != "circuit:red" {
		t.Errorf("first group should be circuit:red, got %s", groups[0].Value)
	}
	if groups[1].Value != "v2-v3" {
		t.Errorf("second group should be v2-v3, got %s", groups[1].Value)
	}
	if groups[2].Value != "5.10" {
		t.Errorf("third group should be 5.10, got %s", groups[2].Value)
	}
}

func TestBuildGradeGroups_UnknownCircuitColorIgnored(t *testing.T) {
	settings := model.DefaultLocationSettings()
	settings.Circuits.Colors = []model.CircuitColor{
		{Name: "red", Hex: "#e53935"},
	}

	dist := []repository.GradeCount{
		{GradingSystem: "circuit", Grade: "purple", RouteType: "boulder", Count: 5}, // not in settings
	}

	groups := buildGradeGroups(dist, &settings)
	if len(groups) != 0 {
		t.Errorf("expected 0 groups for unconfigured circuit color, got %d", len(groups))
	}
}

// ── roleDisplayName ─────────────────────────────────────────

func TestRoleDisplayName(t *testing.T) {
	tests := map[string]string{
		"org_admin":   "Admin",
		"gym_manager": "Manager",
		"head_setter": "Head Setter",
		"setter":      "Setter",
		"climber":     "Climber",
		"unknown":     "Climber", // default
		"":            "Climber",
	}

	for role, want := range tests {
		got := roleDisplayName(role)
		if got != want {
			t.Errorf("roleDisplayName(%q) = %q, want %q", role, got, want)
		}
	}
}

// ── templateDataFromContext ──────────────────────────────────

func TestTemplateDataFromContext_NoSession(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	td := templateDataFromContext(req, "dashboard")

	if td.ActiveNav != "dashboard" {
		t.Errorf("ActiveNav = %q, want %q", td.ActiveNav, "dashboard")
	}
	if td.UserName != "Guest" {
		t.Errorf("UserName = %q, want %q", td.UserName, "Guest")
	}
	if td.UserInitial != "?" {
		t.Errorf("UserInitial = %q, want %q", td.UserInitial, "?")
	}
	// IsSetter is true when effectiveRole != "climber" — empty string qualifies.
	// In practice, unauthenticated users never reach setter pages (middleware redirects).
	if !td.IsSetter {
		t.Error("IsSetter should be true (empty role != climber)")
	}
	if td.IsHeadSetter {
		t.Error("IsHeadSetter should be false with no session")
	}
	if td.IsOrgAdmin {
		t.Error("IsOrgAdmin should be false with no session")
	}
}

func TestTemplateDataFromContext_WithUser(t *testing.T) {
	user := &model.User{
		ID:          "user-123",
		DisplayName: "Chris",
	}

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	ctx := middleware.SetWebUser(req.Context(), user)
	ctx = middleware.SetWebRole(ctx, "head_setter")
	ctx = middleware.SetWebRealRole(ctx, "head_setter")
	req = req.WithContext(ctx)

	td := templateDataFromContext(req, "dashboard")

	if td.UserName != "Chris" {
		t.Errorf("UserName = %q, want %q", td.UserName, "Chris")
	}
	if td.UserInitial != "C" {
		t.Errorf("UserInitial = %q, want %q", td.UserInitial, "C")
	}
	if !td.IsSetter {
		t.Error("IsSetter should be true for head_setter")
	}
	if !td.IsHeadSetter {
		t.Error("IsHeadSetter should be true for head_setter")
	}
	if td.IsOrgAdmin {
		t.Error("IsOrgAdmin should be false for head_setter")
	}
	if td.UserRole != "Head Setter" {
		t.Errorf("UserRole = %q, want %q", td.UserRole, "Head Setter")
	}
}

func TestTemplateDataFromContext_ViewAsOverride(t *testing.T) {
	user := &model.User{
		ID:          "user-456",
		DisplayName: "Admin User",
	}

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	ctx := middleware.SetWebUser(req.Context(), user)
	ctx = middleware.SetWebRole(ctx, "setter")       // effective = setter (overridden)
	ctx = middleware.SetWebRealRole(ctx, "org_admin") // real = org_admin
	req = req.WithContext(ctx)

	td := templateDataFromContext(req, "dashboard")

	if td.ViewAsRole != "setter" {
		t.Errorf("ViewAsRole = %q, want %q", td.ViewAsRole, "setter")
	}
	if td.RealRole != "org_admin" {
		t.Errorf("RealRole = %q, want %q", td.RealRole, "org_admin")
	}
	// Effective role is setter, so IsSetter=true but IsOrgAdmin=false
	if !td.IsSetter {
		t.Error("IsSetter should be true for effective setter")
	}
	if td.IsOrgAdmin {
		t.Error("IsOrgAdmin should be false when viewing as setter")
	}
}

// ── Utility helpers ─────────────────────────────────────────

func TestDerefString(t *testing.T) {
	s := "hello"
	if derefString(&s) != "hello" {
		t.Error("derefString should return pointed-to value")
	}
	if derefString(nil) != "" {
		t.Error("derefString(nil) should return empty string")
	}
}

func TestDerefFloat64(t *testing.T) {
	f := 3.14
	if derefFloat64(&f) != 3.14 {
		t.Error("derefFloat64 should return pointed-to value")
	}
	if derefFloat64(nil) != 0 {
		t.Error("derefFloat64(nil) should return 0")
	}
}

func TestDerefInt(t *testing.T) {
	n := 42
	if derefInt(&n) != 42 {
		t.Error("derefInt should return pointed-to value")
	}
	if derefInt(nil) != 0 {
		t.Error("derefInt(nil) should return 0")
	}
}

func TestStrPtr(t *testing.T) {
	p := strPtr("test")
	if p == nil {
		t.Fatal("strPtr should return non-nil pointer")
	}
	if *p != "test" {
		t.Errorf("strPtr value = %q, want %q", *p, "test")
	}
}

func TestSeq_EdgeCases(t *testing.T) {
	// Single element
	got := seq(5, 5)
	if len(got) != 1 || got[0] != 5 {
		t.Errorf("seq(5,5) = %v, want [5]", got)
	}

	// Note: seq(10, 5) panics (negative capacity) — callers ensure start <= end.

	// Negative numbers
	got3 := seq(-2, 2)
	expected := []int{-2, -1, 0, 1, 2}
	if len(got3) != 5 {
		t.Fatalf("seq(-2,2) len = %d, want 5", len(got3))
	}
	for i, v := range expected {
		if got3[i] != v {
			t.Errorf("seq(-2,2)[%d] = %d, want %d", i, got3[i], v)
		}
	}
}

// ── relativeTime ────────────────────────────────────────────

func TestRelativeTime(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name   string
		t      time.Time
		expect string
	}{
		{"just now", now, "just now"},
		{"30 seconds ago", now.Add(-30 * time.Second), "just now"},
		{"1 min ago", now.Add(-90 * time.Second), "1 min ago"},
		{"5 min ago", now.Add(-5 * time.Minute), "5m ago"},
		{"1 hour ago", now.Add(-90 * time.Minute), "1 hour ago"},
		{"3 hours ago", now.Add(-3 * time.Hour), "3h ago"},
		{"yesterday", now.Add(-36 * time.Hour), "yesterday"},
		{"3 days ago", now.Add(-3 * 24 * time.Hour), "3d ago"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := relativeTime(tc.t)
			if got != tc.expect {
				t.Errorf("relativeTime(%v) = %q, want %q", tc.t, got, tc.expect)
			}
		})
	}

	// Older dates should format as "Jan 2"
	oldDate := now.Add(-30 * 24 * time.Hour)
	got := relativeTime(oldDate)
	expected := oldDate.Format("Jan 2")
	if got != expected {
		t.Errorf("relativeTime(30 days ago) = %q, want %q", got, expected)
	}
}

// ── realIP ──────────────────────────────────────────────────

func TestRealIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		expect     string
	}{
		{"ipv4 with port", "192.168.1.1:12345", "192.168.1.1"},
		{"ipv4 no port", "192.168.1.1", "192.168.1.1"},
		{"ipv6 loopback with port", "[::1]:12345", "::1"},
		// Note: bare "::1" without brackets never appears in net/http RemoteAddr.
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tc.remoteAddr
			got := realIP(req)
			if got != tc.expect {
				t.Errorf("realIP() = %q, want %q", got, tc.expect)
			}
		})
	}
}

// ── truncateUA ──────────────────────────────────────────────

func TestTruncateUA(t *testing.T) {
	short := "Mozilla/5.0"
	if truncateUA(short) != short {
		t.Error("short UA should not be truncated")
	}

	// Create a very long user agent
	long := ""
	for i := 0; i < 100; i++ {
		long += "Mozilla/5.0 "
	}
	truncated := truncateUA(long)
	if len(truncated) != 512 {
		t.Errorf("truncated UA len = %d, want 512", len(truncated))
	}

	if truncateUA("") != "" {
		t.Error("empty UA should stay empty")
	}
}

// ── circuitColorHex map ─────────────────────────────────────

func TestCircuitColorHex_AllColorsPresent(t *testing.T) {
	colors := []string{"red", "orange", "yellow", "green", "blue", "purple", "pink", "white", "black"}
	for _, c := range colors {
		if _, ok := circuitColorHex[c]; !ok {
			t.Errorf("circuitColorHex missing color %q", c)
		}
	}
}

func TestCircuitColorHex_ValidHex(t *testing.T) {
	for name, hex := range circuitColorHex {
		if !validHexColor.MatchString(hex) {
			t.Errorf("circuitColorHex[%q] = %q is not a valid hex color", name, hex)
		}
	}
}

// ── RouteView CircuitVGrade ─────────────────────────────────

func TestCircuitVGrade(t *testing.T) {
	tests := []struct {
		name             string
		rv               RouteView
		expect           string
	}{
		{
			name:   "circuit with V-grade",
			rv:     RouteView{Route: routeWithGrading("circuit", "V3", strPtr("red"))},
			expect: "V3",
		},
		{
			name:   "circuit grade same as color - no V-grade",
			rv:     RouteView{Route: routeWithGrading("circuit", "red", strPtr("red"))},
			expect: "",
		},
		{
			name:   "circuit no grade",
			rv:     RouteView{Route: routeWithGrading("circuit", "", strPtr("blue"))},
			expect: "",
		},
		{
			name:   "not circuit - no V-grade",
			rv:     RouteView{Route: routeWithGrading("v_scale", "V5", nil)},
			expect: "",
		},
		{
			name:   "circuit with HideCircuitGrade",
			rv:     RouteView{Route: routeWithGrading("circuit", "V4", strPtr("green")), HideCircuitGrade: true},
			expect: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.rv.CircuitVGrade()
			if got != tc.expect {
				t.Errorf("CircuitVGrade() = %q, want %q", got, tc.expect)
			}
		})
	}
}

// ── RoutePreviewData SafeColor ──────────────────────────────

func TestRoutePreviewData_SafeColor(t *testing.T) {
	rp := RoutePreviewData{Color: "#e53935"}
	if rp.SafeColor() != "#e53935" {
		t.Error("valid color should pass through")
	}

	rp2 := RoutePreviewData{Color: "malicious"}
	if rp2.SafeColor() != "#999999" {
		t.Error("invalid color should return default")
	}
}

// ── TickListItem SafeColor ──────────────────────────────────

func TestTickListItem_SafeColor(t *testing.T) {
	item := TickListItem{RouteColor: "#1565c0"}
	if item.SafeColor() != "#1565c0" {
		t.Error("valid color should pass through")
	}

	item2 := TickListItem{RouteColor: ""}
	if item2.SafeColor() != "#999999" {
		t.Error("empty color should return default")
	}
}

// ── GradeGroup SafeColor ────────────────────────────────────

func TestGradeGroup_SafeColor(t *testing.T) {
	gg := GradeGroup{Color: "#e53935"}
	if gg.SafeColor() != "#e53935" {
		t.Error("valid color should pass through")
	}
}

// ── RouteFormValues HasTag ──────────────────────────────────

func TestRouteFormValues_HasTag(t *testing.T) {
	v := RouteFormValues{
		TagIDs: map[string]bool{
			"tag-1": true,
			"tag-2": true,
		},
	}

	if !v.HasTag("tag-1") {
		t.Error("HasTag should return true for present tag")
	}
	if v.HasTag("tag-3") {
		t.Error("HasTag should return false for missing tag")
	}

	// Nil map
	v2 := RouteFormValues{}
	if v2.HasTag("anything") {
		t.Error("HasTag should return false on nil map")
	}
}

// ── buildRouteFields ────────────────────────────────────────

func TestBuildRouteFields_BoulderWall(t *testing.T) {
	settings := model.DefaultLocationSettings()

	rf := buildRouteFields("", "wall-1", "boulder", "v_scale", "", settings, nil)
	if rf.WallType != "boulder" {
		t.Errorf("WallType = %q, want %q", rf.WallType, "boulder")
	}
	if len(rf.TypeOptions) == 0 {
		t.Fatal("TypeOptions should not be empty for boulder wall")
	}
	if rf.TypeOptions[0].Value != "boulder" {
		t.Errorf("first TypeOption = %q, want %q", rf.TypeOptions[0].Value, "boulder")
	}
	if rf.GradeLabel != "Grade" {
		t.Errorf("GradeLabel = %q, want %q", rf.GradeLabel, "Grade")
	}
}

func TestBuildRouteFields_BoulderWallCircuit(t *testing.T) {
	settings := model.DefaultLocationSettings()
	settings.Circuits.Colors = []model.CircuitColor{
		{Name: "red", Hex: "#e53935"},
		{Name: "blue", Hex: "#1565c0"},
	}

	rf := buildRouteFields("", "wall-1", "boulder", "circuit", "", settings, nil)
	if len(rf.TypeOptions) != 1 {
		t.Fatalf("TypeOptions len = %d, want 1", len(rf.TypeOptions))
	}
	if rf.TypeOptions[0].Value != "boulder_circuit" {
		t.Errorf("TypeOption = %q, want %q", rf.TypeOptions[0].Value, "boulder_circuit")
	}
	if rf.GradeLabel != "Circuit Color" {
		t.Errorf("GradeLabel = %q, want %q", rf.GradeLabel, "Circuit Color")
	}
	if !rf.ShowVGrade {
		t.Error("ShowVGrade should be true for circuit boulders")
	}
}

func TestBuildRouteFields_BoulderWallBoth(t *testing.T) {
	settings := model.DefaultLocationSettings()

	rf := buildRouteFields("", "wall-1", "boulder", "both", "", settings, nil)
	if len(rf.TypeOptions) != 2 {
		t.Fatalf("TypeOptions len = %d, want 2", len(rf.TypeOptions))
	}
}

func TestBuildRouteFields_RouteWall(t *testing.T) {
	settings := model.DefaultLocationSettings()

	rf := buildRouteFields("", "wall-1", "route", "v_scale", "", settings, nil)
	if len(rf.TypeOptions) != 1 {
		t.Fatalf("TypeOptions len = %d, want 1", len(rf.TypeOptions))
	}
	if rf.TypeOptions[0].Value != "route" {
		t.Errorf("TypeOption = %q, want %q", rf.TypeOptions[0].Value, "route")
	}
	if len(rf.GradeOptions) == 0 {
		t.Error("GradeOptions should contain YDS grades for route wall")
	}
}

func TestBuildRouteFields_EmptyWallType(t *testing.T) {
	settings := model.DefaultLocationSettings()

	rf := buildRouteFields("", "wall-1", "", "v_scale", "", settings, nil)
	if len(rf.TypeOptions) != 0 {
		t.Error("TypeOptions should be empty when wall type is empty")
	}
}

func TestBuildRouteFields_SessionID(t *testing.T) {
	settings := model.DefaultLocationSettings()

	rf := buildRouteFields("session-abc", "wall-1", "boulder", "v_scale", "", settings, nil)
	if rf.SessionID != "session-abc" {
		t.Errorf("SessionID = %q, want %q", rf.SessionID, "session-abc")
	}
}

// ── holdColorsFromSettings ──────────────────────────────────

func TestHoldColorsFromSettings_DefaultColors(t *testing.T) {
	settings := model.DefaultLocationSettings()
	colors := holdColorsFromSettings(settings)

	if len(colors) != len(defaultHoldColors) {
		t.Errorf("default colors len = %d, want %d", len(colors), len(defaultHoldColors))
	}
}

func TestHoldColorsFromSettings_CustomColors(t *testing.T) {
	settings := model.DefaultLocationSettings()
	settings.HoldColors.Colors = []model.HoldColor{
		{Name: "Neon Green", Hex: "#39ff14"},
		{Name: "Hot Pink", Hex: "#ff69b4"},
	}

	colors := holdColorsFromSettings(settings)
	if len(colors) != 2 {
		t.Fatalf("custom colors len = %d, want 2", len(colors))
	}
	if colors[0].Name != "Neon Green" || colors[0].Hex != "#39ff14" {
		t.Errorf("first color = %+v, want Neon Green/#39ff14", colors[0])
	}
}

// ── Grade lists completeness ────────────────────────────────

func TestVScaleGrades_SortedAndComplete(t *testing.T) {
	if len(vScaleGrades) == 0 {
		t.Fatal("vScaleGrades should not be empty")
	}
	if vScaleGrades[0] != "VB" {
		t.Errorf("first V-scale grade = %q, want VB", vScaleGrades[0])
	}
	if vScaleGrades[len(vScaleGrades)-1] != "V12" {
		t.Errorf("last V-scale grade = %q, want V12", vScaleGrades[len(vScaleGrades)-1])
	}
}

func TestYDSGrades_IncludesPlusMinusFormat(t *testing.T) {
	gradeSet := make(map[string]bool)
	for _, g := range ydsGrades {
		gradeSet[g] = true
	}

	// Chris uses plus/minus grades
	for _, expected := range []string{"5.9-", "5.9", "5.9+", "5.10-", "5.10", "5.10+"} {
		if !gradeSet[expected] {
			t.Errorf("ydsGrades missing %q", expected)
		}
	}
}

func TestCircuitColors_Complete(t *testing.T) {
	expected := map[string]bool{
		"red": true, "orange": true, "yellow": true, "green": true,
		"blue": true, "purple": true, "pink": true, "white": true, "black": true,
	}
	for _, c := range circuitColors {
		if !expected[c] {
			t.Errorf("unexpected circuit color %q", c)
		}
		delete(expected, c)
	}
	for missing := range expected {
		t.Errorf("missing circuit color %q", missing)
	}
}

// ── Wall data ───────────────────────────────────────────────

func TestWallTypes_ValidOptions(t *testing.T) {
	if len(wallTypes) != 2 {
		t.Errorf("wallTypes len = %d, want 2", len(wallTypes))
	}
	validValues := map[string]bool{"boulder": true, "route": true}
	for _, wt := range wallTypes {
		if !validValues[wt.Value] {
			t.Errorf("unexpected wall type value %q", wt.Value)
		}
		if wt.Label == "" {
			t.Errorf("wall type %q has empty label", wt.Value)
		}
	}
}

func TestWallAngles_NonEmpty(t *testing.T) {
	if len(wallAngles) == 0 {
		t.Error("wallAngles should not be empty")
	}
	for _, a := range wallAngles {
		if a == "" {
			t.Error("wall angle should not be empty")
		}
	}
}

func TestSurfaceTypes_NonEmpty(t *testing.T) {
	if len(surfaceTypes) == 0 {
		t.Error("surfaceTypes should not be empty")
	}
}

// ── Benchmark ───────────────────────────────────────────────

func BenchmarkBuildGradeGroups(b *testing.B) {
	dist := make([]repository.GradeCount, 0, 30)
	for i := 0; i <= 12; i++ {
		dist = append(dist, repository.GradeCount{
			GradingSystem: "v_scale",
			Grade:         fmt.Sprintf("V%d", i),
			RouteType:     "boulder",
			Count:         10 + i,
		})
	}
	for _, g := range []string{"5.7", "5.8", "5.9", "5.10a", "5.10b", "5.10c", "5.10d", "5.11a", "5.11b", "5.11c", "5.11d", "5.12a", "5.12b", "5.13a"} {
		dist = append(dist, repository.GradeCount{
			GradingSystem: "yds",
			Grade:         g,
			RouteType:     "route",
			Count:         5,
		})
	}

	settings := model.DefaultLocationSettings()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buildGradeGroups(dist, &settings)
	}
}
