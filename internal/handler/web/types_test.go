package webhandler

import (
	"strings"
	"testing"

	"github.com/shotwell-paddle/routewerk/internal/model"
)

// ── ArchiveFilterURL ───────────────────────────────────────────────

func TestArchiveFilterURL_NoFilters(t *testing.T) {
	pd := &PageData{}
	got := pd.ArchiveFilterURL("")
	if got != "/archive" {
		t.Errorf("ArchiveFilterURL() = %q, want %q", got, "/archive")
	}
}

func TestArchiveFilterURL_GradeOnly(t *testing.T) {
	pd := &PageData{}
	got := pd.ArchiveFilterURL("v2-v3")
	if got != "/archive?grade=v2-v3" {
		t.Errorf("ArchiveFilterURL(v2-v3) = %q, want %q", got, "/archive?grade=v2-v3")
	}
}

func TestArchiveFilterURL_AllFilters(t *testing.T) {
	pd := &PageData{
		TypeFilter: "boulder",
		WallFilter: "wall-1",
		DateFrom:   "2026-01-01",
		DateTo:     "2026-03-01",
	}
	got := pd.ArchiveFilterURL("v4-v5")

	// Verify all params are present (order may vary due to url.Values encoding)
	for _, param := range []string{"grade=v4-v5", "type=boulder", "wall=wall-1", "from=2026-01-01", "to=2026-03-01"} {
		if !strings.Contains(got, param) {
			t.Errorf("ArchiveFilterURL missing param %q in %q", param, got)
		}
	}
	if got[:9] != "/archive?" {
		t.Errorf("should start with /archive?, got %q", got[:9])
	}
}

func TestArchiveFilterURL_TypeFilterOnly(t *testing.T) {
	pd := &PageData{TypeFilter: "route"}
	got := pd.ArchiveFilterURL("")
	if got != "/archive?type=route" {
		t.Errorf("ArchiveFilterURL() = %q, want %q", got, "/archive?type=route")
	}
}

// ── DisplayGrade ───────────────────────────────────────────────────

func TestDisplayGrade_VScale(t *testing.T) {
	rv := RouteView{Route: model.Route{GradingSystem: "v_scale", Grade: "V5"}}
	got := rv.DisplayGrade()
	if got != "V5" {
		t.Errorf("DisplayGrade() = %q, want %q", got, "V5")
	}
}

func TestDisplayGrade_YDS(t *testing.T) {
	rv := RouteView{Route: model.Route{GradingSystem: "yds", Grade: "5.10+"}}
	got := rv.DisplayGrade()
	if got != "5.10+" {
		t.Errorf("DisplayGrade() = %q, want %q", got, "5.10+")
	}
}

func TestDisplayGrade_Circuit(t *testing.T) {
	color := "red"
	rv := RouteView{Route: model.Route{
		GradingSystem: "circuit",
		Grade:         "V3",
		CircuitColor:  &color,
	}}
	got := rv.DisplayGrade()
	if got != "Red" { // titleCase
		t.Errorf("DisplayGrade() = %q, want %q", got, "Red")
	}
}

func TestDisplayGrade_CircuitEmptyColor(t *testing.T) {
	empty := ""
	rv := RouteView{Route: model.Route{
		GradingSystem: "circuit",
		Grade:         "V3",
		CircuitColor:  &empty,
	}}
	got := rv.DisplayGrade()
	// Empty circuit color falls through to Grade
	if got != "V3" {
		t.Errorf("DisplayGrade() = %q, want %q", got, "V3")
	}
}

func TestDisplayGrade_CircuitNilColor(t *testing.T) {
	rv := RouteView{Route: model.Route{
		GradingSystem: "circuit",
		Grade:         "V3",
	}}
	got := rv.DisplayGrade()
	if got != "V3" {
		t.Errorf("DisplayGrade() = %q, want %q", got, "V3")
	}
}

// ── IsCircuit ──────────────────────────────────────────────────────

func TestIsCircuit(t *testing.T) {
	tests := []struct {
		system string
		want   bool
	}{
		{"circuit", true},
		{"v_scale", false},
		{"yds", false},
		{"font", false},
		{"", false},
	}
	for _, tc := range tests {
		rv := RouteView{Route: model.Route{GradingSystem: tc.system}}
		if got := rv.IsCircuit(); got != tc.want {
			t.Errorf("IsCircuit() for %q = %v, want %v", tc.system, got, tc.want)
		}
	}
}

// ── RouteView SafeColor ────────────────────────────────────────────

func TestRouteView_SafeColor(t *testing.T) {
	rv := RouteView{Route: model.Route{Color: "#e53935"}}
	if got := rv.SafeColor(); got != "#e53935" {
		t.Errorf("SafeColor() = %q, want %q", got, "#e53935")
	}

	rv2 := RouteView{Route: model.Route{Color: "not-a-color"}}
	if got := rv2.SafeColor(); got != "#999999" {
		t.Errorf("SafeColor() for invalid = %q, want %q", got, "#999999")
	}

	rv3 := RouteView{Route: model.Route{Color: ""}}
	if got := rv3.SafeColor(); got != "#999999" {
		t.Errorf("SafeColor() for empty = %q, want %q", got, "#999999")
	}
}

