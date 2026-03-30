package webhandler

import (
	"testing"

	"github.com/shotwell-paddle/routewerk/internal/repository"
)

// ── ascentTypeLabel ─────────────────────────────────────────

func TestAscentTypeLabel_AllTypes(t *testing.T) {
	tests := map[string]string{
		"send":    "sent",
		"flash":   "flashed",
		"attempt": "attempted",
		"project": "projected",
		"unknown": "unknown", // passthrough
		"":        "",        // passthrough
		"onsight": "onsight", // passthrough
	}
	for input, want := range tests {
		got := ascentTypeLabel(input)
		if got != want {
			t.Errorf("ascentTypeLabel(%q) = %q, want %q", input, got, want)
		}
	}
}

// ── buildPyramidBars ────────────────────────────────────────

func TestBuildPyramidBars_Empty(t *testing.T) {
	bars := buildPyramidBars(nil)
	if bars != nil {
		t.Errorf("expected nil for empty input, got %v", bars)
	}
}

func TestBuildPyramidBars_SingleEntry(t *testing.T) {
	entries := []repository.GradePyramidEntry{
		{Grade: "V3", GradingSystem: "v_scale", Count: 5},
	}
	bars := buildPyramidBars(entries)
	if len(bars) != 1 {
		t.Fatalf("len = %d, want 1", len(bars))
	}
	if bars[0].Grade != "V3" {
		t.Errorf("Grade = %q, want %q", bars[0].Grade, "V3")
	}
	if bars[0].WidthPct != 100 {
		t.Errorf("WidthPct = %d, want 100 (max entry)", bars[0].WidthPct)
	}
}

func TestBuildPyramidBars_MinimumWidth(t *testing.T) {
	entries := []repository.GradePyramidEntry{
		{Grade: "V10", GradingSystem: "v_scale", Count: 100},
		{Grade: "V1", GradingSystem: "v_scale", Count: 1},
	}
	bars := buildPyramidBars(entries)
	if len(bars) != 2 {
		t.Fatalf("len = %d, want 2", len(bars))
	}
	// V1 has count=1 out of max=100 → 1% → clamped to 5% minimum
	if bars[1].WidthPct != 5 {
		t.Errorf("small bar WidthPct = %d, want 5 (minimum)", bars[1].WidthPct)
	}
}

func TestBuildPyramidBars_Proportional(t *testing.T) {
	entries := []repository.GradePyramidEntry{
		{Grade: "5.10", GradingSystem: "yds", Count: 20},
		{Grade: "5.11", GradingSystem: "yds", Count: 10},
	}
	bars := buildPyramidBars(entries)
	if bars[0].WidthPct != 100 {
		t.Errorf("max bar WidthPct = %d, want 100", bars[0].WidthPct)
	}
	if bars[1].WidthPct != 50 {
		t.Errorf("half bar WidthPct = %d, want 50", bars[1].WidthPct)
	}
}

func TestBuildPyramidBars_PreservesFields(t *testing.T) {
	entries := []repository.GradePyramidEntry{
		{Grade: "V5", GradingSystem: "v_scale", Count: 7},
	}
	bars := buildPyramidBars(entries)
	if bars[0].System != "v_scale" {
		t.Errorf("System = %q, want %q", bars[0].System, "v_scale")
	}
	if bars[0].Count != 7 {
		t.Errorf("Count = %d, want 7", bars[0].Count)
	}
}

// ── loadConsensus percentage logic ──────────────────────────
// (loadConsensus itself needs a DB, but the math is inline)

func TestConsensusPercentages(t *testing.T) {
	// Verify the integer percentage calculation used in loadConsensus
	tests := []struct {
		easy, right, hard, total int
		wantEasy, wantRight, wantHard int
	}{
		{3, 5, 2, 10, 30, 50, 20},
		{1, 1, 1, 3, 33, 33, 33},     // integer division: 33+33+33=99
		{0, 0, 0, 0, 0, 0, 0},        // zero total handled by caller
		{10, 0, 0, 10, 100, 0, 0},
		{0, 0, 10, 10, 0, 0, 100},
	}
	for _, tc := range tests {
		if tc.total == 0 {
			continue
		}
		easyPct := tc.easy * 100 / tc.total
		rightPct := tc.right * 100 / tc.total
		hardPct := tc.hard * 100 / tc.total
		if easyPct != tc.wantEasy || rightPct != tc.wantRight || hardPct != tc.wantHard {
			t.Errorf("(%d,%d,%d)/%d: got (%d,%d,%d), want (%d,%d,%d)",
				tc.easy, tc.right, tc.hard, tc.total,
				easyPct, rightPct, hardPct,
				tc.wantEasy, tc.wantRight, tc.wantHard)
		}
	}
}
