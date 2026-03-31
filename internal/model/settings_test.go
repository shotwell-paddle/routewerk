package model

import "testing"

// ── DefaultLocationSettings ────────────────────────────────────────

func TestDefaultLocationSettings_GradingDefaults(t *testing.T) {
	s := DefaultLocationSettings()

	if s.Grading.BoulderMethod != "v_scale" {
		t.Errorf("BoulderMethod = %q, want %q", s.Grading.BoulderMethod, "v_scale")
	}
	if s.Grading.RouteGradeFormat != "plus_minus" {
		t.Errorf("RouteGradeFormat = %q, want %q", s.Grading.RouteGradeFormat, "plus_minus")
	}
	if s.Grading.ShowGradesOnCircuit {
		t.Error("ShowGradesOnCircuit should default to false")
	}
}

func TestDefaultLocationSettings_CircuitColorsPresent(t *testing.T) {
	s := DefaultLocationSettings()

	if len(s.Circuits.Colors) != 9 {
		t.Fatalf("Circuits.Colors len = %d, want 9", len(s.Circuits.Colors))
	}

	expected := []string{"red", "orange", "yellow", "green", "blue", "purple", "pink", "white", "black"}
	for i, name := range expected {
		if s.Circuits.Colors[i].Name != name {
			t.Errorf("circuit color[%d] = %q, want %q", i, s.Circuits.Colors[i].Name, name)
		}
		if s.Circuits.Colors[i].SortOrder != i {
			t.Errorf("circuit color[%d].SortOrder = %d, want %d", i, s.Circuits.Colors[i].SortOrder, i)
		}
		if s.Circuits.Colors[i].Hex == "" {
			t.Errorf("circuit color[%d].Hex is empty", i)
		}
	}
}

func TestDefaultLocationSettings_HoldColorsPresent(t *testing.T) {
	s := DefaultLocationSettings()

	if len(s.HoldColors.Colors) != 10 {
		t.Fatalf("HoldColors.Colors len = %d, want 10", len(s.HoldColors.Colors))
	}

	// All hold colors should have a name and hex
	for i, hc := range s.HoldColors.Colors {
		if hc.Name == "" {
			t.Errorf("hold color[%d].Name is empty", i)
		}
		if hc.Hex == "" {
			t.Errorf("hold color[%d].Hex is empty", i)
		}
	}
}

func TestDefaultLocationSettings_DisplayDefaults(t *testing.T) {
	s := DefaultLocationSettings()

	if !s.Display.ShowSetterName {
		t.Error("ShowSetterName should default to true")
	}
	if !s.Display.ShowRouteAge {
		t.Error("ShowRouteAge should default to true")
	}
	if !s.Display.ShowDifficultyConsensus {
		t.Error("ShowDifficultyConsensus should default to true")
	}
	if s.Display.DefaultStripAgeDays != 90 {
		t.Errorf("DefaultStripAgeDays = %d, want 90", s.Display.DefaultStripAgeDays)
	}
}

func TestDefaultLocationSettings_SessionDefaults(t *testing.T) {
	s := DefaultLocationSettings()

	if !s.Sessions.DefaultPlaybookEnabled {
		t.Error("DefaultPlaybookEnabled should default to true")
	}
	if s.Sessions.RequireRoutePhoto {
		t.Error("RequireRoutePhoto should default to false")
	}
}

// ── DefaultOrgSettings ─────────────────────────────────────────────

func TestDefaultOrgSettings_PermissionsAllTrue(t *testing.T) {
	s := DefaultOrgSettings()

	perms := s.Permissions
	checks := map[string]bool{
		"HeadSetterCanEditGrading":    perms.HeadSetterCanEditGrading,
		"HeadSetterCanEditCircuits":   perms.HeadSetterCanEditCircuits,
		"HeadSetterCanEditHoldColors": perms.HeadSetterCanEditHoldColors,
		"HeadSetterCanEditDisplay":    perms.HeadSetterCanEditDisplay,
		"HeadSetterCanEditSessions":   perms.HeadSetterCanEditSessions,
	}

	for name, val := range checks {
		if !val {
			t.Errorf("%s should default to true", name)
		}
	}
}

func TestDefaultOrgSettings_DefaultsMatchLocation(t *testing.T) {
	org := DefaultOrgSettings()
	loc := DefaultLocationSettings()

	if org.Defaults.BoulderMethod != loc.Grading.BoulderMethod {
		t.Errorf("org default BoulderMethod %q != location default %q",
			org.Defaults.BoulderMethod, loc.Grading.BoulderMethod)
	}
	if org.Defaults.RouteGradeFormat != loc.Grading.RouteGradeFormat {
		t.Errorf("org default RouteGradeFormat %q != location default %q",
			org.Defaults.RouteGradeFormat, loc.Grading.RouteGradeFormat)
	}
}

// ── DefaultUserSettings ────────────────────────────────────────────

func TestDefaultUserSettings_AllPublic(t *testing.T) {
	s := DefaultUserSettings()

	if !s.Privacy.ShowProfile {
		t.Error("ShowProfile should default to true")
	}
	if !s.Privacy.ShowTickList {
		t.Error("ShowTickList should default to true")
	}
	if !s.Privacy.ShowStats {
		t.Error("ShowStats should default to true")
	}
	if !s.Privacy.ShowOnLeaderboard {
		t.Error("ShowOnLeaderboard should default to true")
	}
}
