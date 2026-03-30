package model

// ============================================================
// Gym Settings (location-level, managed by head_setter)
// ============================================================

// LocationSettings is the top-level settings object stored as JSONB
// in locations.settings_json.
type LocationSettings struct {
	Grading    GradingSettings    `json:"grading"`
	Circuits   CircuitSettings    `json:"circuits"`
	HoldColors HoldColorSettings  `json:"hold_colors"`
	Display    DisplaySettings    `json:"display"`
	Sessions   SessionSettings    `json:"sessions"`
}

// GradingSettings controls how grades are displayed and entered.
type GradingSettings struct {
	// BoulderMethod determines the primary grading method for boulders.
	// "v_scale" (default), "circuit", or "both"
	BoulderMethod string `json:"boulder_method"`

	// RouteGradeFormat determines how roped route grades are displayed.
	// "plus_minus" (5.9-, 5.9, 5.9+) or "letter" (5.9a, 5.9b, 5.9c, 5.9d)
	RouteGradeFormat string `json:"route_grade_format"`

	// ShowGradesOnCircuit controls whether climbers see the V-grade alongside
	// circuit color. Setters always see the grade regardless of this setting.
	ShowGradesOnCircuit bool `json:"show_grades_on_circuit"`

	// VScaleRange is the available V-scale grades (can be customized to limit range).
	VScaleRange []string `json:"v_scale_range,omitempty"`

	// YDSRange is the available YDS grades.
	YDSRange []string `json:"yds_range,omitempty"`
}

// CircuitColor represents a single circuit color with ordering.
type CircuitColor struct {
	Name      string `json:"name"`
	Hex       string `json:"hex"`
	SortOrder int    `json:"sort_order"`
}

// CircuitSettings manages the circuit color palette.
type CircuitSettings struct {
	Colors []CircuitColor `json:"colors"`
}

// HoldColor represents a single hold color with display info.
type HoldColor struct {
	Name string `json:"name"`
	Hex  string `json:"hex"`
}

// HoldColorSettings manages the hold color palette for route forms.
type HoldColorSettings struct {
	Colors []HoldColor `json:"colors"`
}

// DisplaySettings controls what's visible to climbers.
type DisplaySettings struct {
	ShowSetterName         bool `json:"show_setter_name"`
	ShowRouteAge           bool `json:"show_route_age"`
	ShowDifficultyConsensus bool `json:"show_difficulty_consensus"`
	DefaultStripAgeDays    int  `json:"default_strip_age_days"`
}

// SessionSettings controls setting session defaults.
type SessionSettings struct {
	DefaultPlaybookEnabled bool `json:"default_playbook_enabled"`
	RequireRoutePhoto      bool `json:"require_route_photo"`
}

// DefaultLocationSettings returns sensible defaults for a new location.
func DefaultLocationSettings() LocationSettings {
	return LocationSettings{
		Grading: GradingSettings{
			BoulderMethod:       "v_scale",
			RouteGradeFormat:    "plus_minus",
			ShowGradesOnCircuit: false,
		},
		Circuits: CircuitSettings{
			Colors: []CircuitColor{
				{Name: "red", Hex: "#e53935", SortOrder: 0},
				{Name: "orange", Hex: "#fc5200", SortOrder: 1},
				{Name: "yellow", Hex: "#f9a825", SortOrder: 2},
				{Name: "green", Hex: "#2e7d32", SortOrder: 3},
				{Name: "blue", Hex: "#1565c0", SortOrder: 4},
				{Name: "purple", Hex: "#7b1fa2", SortOrder: 5},
				{Name: "pink", Hex: "#e91e8a", SortOrder: 6},
				{Name: "white", Hex: "#e0e0e0", SortOrder: 7},
				{Name: "black", Hex: "#0a0a0a", SortOrder: 8},
			},
		},
		HoldColors: HoldColorSettings{
			Colors: []HoldColor{
				{Name: "Red", Hex: "#e53935"},
				{Name: "Orange", Hex: "#fc5200"},
				{Name: "Yellow", Hex: "#f9a825"},
				{Name: "Green", Hex: "#2e7d32"},
				{Name: "Blue", Hex: "#1565c0"},
				{Name: "Purple", Hex: "#7b1fa2"},
				{Name: "Pink", Hex: "#e91e8a"},
				{Name: "Black", Hex: "#0a0a0a"},
				{Name: "White", Hex: "#e0e0e0"},
				{Name: "Teal", Hex: "#00897b"},
			},
		},
		Display: DisplaySettings{
			ShowSetterName:          true,
			ShowRouteAge:            true,
			ShowDifficultyConsensus: true,
			DefaultStripAgeDays:     90,
		},
		Sessions: SessionSettings{
			DefaultPlaybookEnabled: true,
			RequireRoutePhoto:      false,
		},
	}
}

// ============================================================
// Organization Settings (org-level, managed by org_admin)
// ============================================================

// OrgSettings is stored as JSONB in organizations.settings_json.
type OrgSettings struct {
	Permissions OrgPermissions `json:"permissions"`
	Defaults    OrgDefaults    `json:"defaults"`
}

// OrgPermissions controls what head setters can configure at gym level.
type OrgPermissions struct {
	HeadSetterCanEditGrading    bool `json:"head_setter_can_edit_grading"`
	HeadSetterCanEditCircuits   bool `json:"head_setter_can_edit_circuits"`
	HeadSetterCanEditHoldColors bool `json:"head_setter_can_edit_hold_colors"`
	HeadSetterCanEditDisplay    bool `json:"head_setter_can_edit_display"`
	HeadSetterCanEditSessions   bool `json:"head_setter_can_edit_sessions"`
}

// OrgDefaults are default values pushed to new locations.
type OrgDefaults struct {
	BoulderMethod       string `json:"boulder_method"`
	RouteGradeFormat    string `json:"route_grade_format"`
	ShowGradesOnCircuit bool   `json:"show_grades_on_circuit"`
}

// DefaultOrgSettings returns permissive defaults (head setters can control everything).
func DefaultOrgSettings() OrgSettings {
	return OrgSettings{
		Permissions: OrgPermissions{
			HeadSetterCanEditGrading:    true,
			HeadSetterCanEditCircuits:   true,
			HeadSetterCanEditHoldColors: true,
			HeadSetterCanEditDisplay:    true,
			HeadSetterCanEditSessions:   true,
		},
		Defaults: OrgDefaults{
			BoulderMethod:       "v_scale",
			RouteGradeFormat:    "plus_minus",
			ShowGradesOnCircuit: false,
		},
	}
}

// ============================================================
// User Settings (privacy, preferences)
// ============================================================

// UserSettings is stored as JSONB in users.settings_json.
type UserSettings struct {
	Privacy PrivacySettings `json:"privacy"`
}

// PrivacySettings controls what other users can see.
type PrivacySettings struct {
	ShowProfile      bool `json:"show_profile"`       // profile visible to other users
	ShowTickList     bool `json:"show_tick_list"`      // tick list visible on public profile
	ShowStats        bool `json:"show_stats"`          // stats + grade pyramid visible
	ShowOnLeaderboard bool `json:"show_on_leaderboard"` // appear in gym leaderboards
}

// DefaultUserSettings returns sensible defaults (everything public).
func DefaultUserSettings() UserSettings {
	return UserSettings{
		Privacy: PrivacySettings{
			ShowProfile:       true,
			ShowTickList:      true,
			ShowStats:         true,
			ShowOnLeaderboard: true,
		},
	}
}
