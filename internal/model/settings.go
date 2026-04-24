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

// PalettePreset groups a Circuits + HoldColors pair tuned for a specific
// print workflow. Gym admins can switch between presets from the Gym
// Settings page; a preset swap rewrites both palettes in one click
// (rather than editing ~18 individual hex inputs).
type PalettePreset struct {
	Name        string // stable ID used in the settings UI and API
	DisplayName string // human-readable, shown in the UI
	Description string // one-line hint explaining when to pick it
	Circuits    []CircuitColor
	HoldColors  []HoldColor
}

// PalettePresetLaserTinted is the default for new locations — tinted
// saturated hues so CMYK coverage per pixel lands at ~75-95% instead of
// the 150-200% of pure saturation. Designed for laser printing onto
// Terra Slate / other polymer synthetic stocks where heavy toner pools
// and fuses blotchy. All tinted values land at luminance > 140 so the
// black-on-colour identifier text reads cleanly on every swatch.
var PalettePresetLaserTinted = PalettePreset{
	Name:        "laser",
	DisplayName: "Laser / Terra Slate (tinted)",
	Description: "Softer colors that fuse cleanly on polymer paper with a color laser.",
	Circuits: []CircuitColor{
		{Name: "red", Hex: "#e8666e", SortOrder: 0},
		{Name: "orange", Hex: "#f9a825", SortOrder: 1},
		{Name: "yellow", Hex: "#fce205", SortOrder: 2},
		{Name: "green", Hex: "#78be85", SortOrder: 3},
		{Name: "blue", Hex: "#6ca3db", SortOrder: 4},
		{Name: "purple", Hex: "#bc75d0", SortOrder: 5},
		{Name: "pink", Hex: "#ff7fb8", SortOrder: 6},
		{Name: "white", Hex: "#e0e0e0", SortOrder: 7},
		{Name: "black", Hex: "#0a0a0a", SortOrder: 8},
	},
	HoldColors: []HoldColor{
		{Name: "Red", Hex: "#e8666e"},
		{Name: "Orange", Hex: "#f9a825"},
		{Name: "Yellow", Hex: "#fce205"},
		{Name: "Green", Hex: "#78be85"},
		{Name: "Blue", Hex: "#6ca3db"},
		{Name: "Purple", Hex: "#bc75d0"},
		{Name: "Pink", Hex: "#ff7fb8"},
		{Name: "Black", Hex: "#0a0a0a"},
		{Name: "White", Hex: "#e0e0e0"},
		{Name: "Teal", Hex: "#00897b"},
	},
}

// PalettePresetInkjetSaturated is the fully saturated palette for gyms
// printing cards on an inkjet. Inkjet ink absorbs into Terra Slate's
// microporous receiver coat, so 150%+ CMYK coverage prints cleanly —
// saturated gym-tape-matching colors are the expected workload.
var PalettePresetInkjetSaturated = PalettePreset{
	Name:        "inkjet",
	DisplayName: "Inkjet / Terra Slate (saturated)",
	Description: "Tape-matching saturated colors that require inkjet printing onto polymer paper.",
	Circuits: []CircuitColor{
		{Name: "red", Hex: "#d32027", SortOrder: 0},
		{Name: "orange", Hex: "#f9a825", SortOrder: 1},
		{Name: "yellow", Hex: "#fce205", SortOrder: 2},
		{Name: "green", Hex: "#2e7d32", SortOrder: 3},
		{Name: "blue", Hex: "#1565c0", SortOrder: 4},
		{Name: "purple", Hex: "#7b1fa2", SortOrder: 5},
		{Name: "pink", Hex: "#ff4fa3", SortOrder: 6},
		{Name: "white", Hex: "#e0e0e0", SortOrder: 7},
		{Name: "black", Hex: "#0a0a0a", SortOrder: 8},
	},
	HoldColors: []HoldColor{
		{Name: "Red", Hex: "#d32027"},
		{Name: "Orange", Hex: "#f9a825"},
		{Name: "Yellow", Hex: "#fce205"},
		{Name: "Green", Hex: "#2e7d32"},
		{Name: "Blue", Hex: "#1565c0"},
		{Name: "Purple", Hex: "#7b1fa2"},
		{Name: "Pink", Hex: "#ff4fa3"},
		{Name: "Black", Hex: "#0a0a0a"},
		{Name: "White", Hex: "#e0e0e0"},
		{Name: "Teal", Hex: "#00897b"},
	},
}

// PalettePresets is the ordered list of presets shown in the gym
// settings UI. Keep the ordering intentional — the laser preset comes
// first because it's the default for new locations.
var PalettePresets = []PalettePreset{
	PalettePresetLaserTinted,
	PalettePresetInkjetSaturated,
}

// LookupPalettePreset returns the preset with the given Name, or nil if
// no match. Caller should fall back to a default (probably the laser
// preset) on nil.
func LookupPalettePreset(name string) *PalettePreset {
	for i := range PalettePresets {
		if PalettePresets[i].Name == name {
			return &PalettePresets[i]
		}
	}
	return nil
}

// DefaultLocationSettings returns sensible defaults for a new location.
// The palette defaults to PalettePresetLaserTinted; gyms can switch to
// another preset (or hand-edit individual colors) from the settings UI.
func DefaultLocationSettings() LocationSettings {
	return LocationSettings{
		Grading: GradingSettings{
			BoulderMethod:       "v_scale",
			RouteGradeFormat:    "plus_minus",
			ShowGradesOnCircuit: false,
		},
		Circuits: CircuitSettings{
			Colors: append([]CircuitColor(nil), PalettePresetLaserTinted.Circuits...),
		},
		HoldColors: HoldColorSettings{
			Colors: append([]HoldColor(nil), PalettePresetLaserTinted.HoldColors...),
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
