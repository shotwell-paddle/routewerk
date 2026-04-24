package webhandler

import (
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

// buildRouteFields assembles the route form fields based on wall type, boulder method, and selected route type.
func buildRouteFields(sessionID, wallID, wallType, boulderMethod, selectedType string, settings model.LocationSettings, setters []repository.SetterAtLocation) RouteFieldsData {
	rf := RouteFieldsData{
		SessionID:    sessionID,
		WallID:       wallID,
		WallType:     wallType,
		VScaleGrades: vScaleGrades,
		HoldColors:   holdColorsFromSettings(settings),
		Setters:      setters,
	}

	if wallType == "" {
		return rf
	}

	// Build type options based on wall type + boulder method
	if wallType == "boulder" {
		switch boulderMethod {
		case "circuit":
			rf.TypeOptions = []SelectOption{{Value: "boulder_circuit", Label: "Boulder (Circuit)"}}
			if selectedType == "" {
				selectedType = "boulder_circuit"
			}
		case "both":
			rf.TypeOptions = []SelectOption{
				{Value: "boulder", Label: "Boulder"},
				{Value: "boulder_circuit", Label: "Boulder (Circuit)"},
			}
			if selectedType == "" {
				selectedType = "boulder"
			}
		default: // v_scale
			rf.TypeOptions = []SelectOption{{Value: "boulder", Label: "Boulder"}}
			if selectedType == "" {
				selectedType = "boulder"
			}
		}
	} else {
		// Route wall — only rope climbing types
		rf.TypeOptions = []SelectOption{{Value: "route", Label: "Route"}}
		selectedType = "route"
	}

	// Mark selected
	for i := range rf.TypeOptions {
		if rf.TypeOptions[i].Value == selectedType {
			rf.TypeOptions[i].Selected = true
		}
	}

	// Build grade options based on selected type
	switch selectedType {
	case "boulder_circuit":
		rf.GradeLabel = "Circuit Color"
		rf.GradePlaceholder = "color"
		rf.ShowVGrade = true
		// Use circuit colors from settings
		for _, c := range settings.Circuits.Colors {
			rf.GradeOptions = append(rf.GradeOptions, c.Name)
		}
	case "route":
		rf.GradeLabel = "Grade"
		rf.GradePlaceholder = "grade"
		rf.GradeOptions = ydsGrades
	default: // boulder
		rf.GradeLabel = "Grade"
		rf.GradePlaceholder = "grade"
		rf.GradeOptions = vScaleGrades
	}

	return rf
}

// holdColorsFromSettings returns HoldColors from gym settings, falling back to defaults.
func holdColorsFromSettings(settings model.LocationSettings) []HoldColor {
	if len(settings.HoldColors.Colors) > 0 {
		colors := make([]HoldColor, len(settings.HoldColors.Colors))
		for i, c := range settings.HoldColors.Colors {
			colors[i] = HoldColor{Name: c.Name, Hex: c.Hex}
		}
		return colors
	}
	return defaultHoldColors
}

// Wall types for form dropdown.
var wallTypes = []struct{ Value, Label string }{
	{"boulder", "Boulder"},
	{"route", "Route (Sport / Top Rope)"},
}

// Common wall angles.
var wallAngles = []string{
	"Slab", "Vertical", "Slight overhang", "Overhang", "Steep overhang", "Roof",
}

// Surface types.
var surfaceTypes = []string{
	"Textured plywood", "Smooth plywood", "Concrete", "Brick", "Natural rock",
}

// Standard hold colors used across climbing gyms (fallback when a location
// hasn't customized its palette). Saturated hues are tinted to fuse
// cleanly on Terra Slate / polymer laser stocks. Must stay in sync with
// model.DefaultLocationSettings — the test suite asserts matching lengths.
var defaultHoldColors = []HoldColor{
	{"Red", "#e8666e"},
	{"Orange", "#f9a825"},
	{"Yellow", "#fce205"},
	{"Green", "#78be85"},
	{"Blue", "#6ca3db"},
	{"Purple", "#bc75d0"},
	{"Pink", "#ff7fb8"},
	{"Black", "#0a0a0a"},
	{"White", "#e0e0e0"},
	{"Teal", "#00897b"},
}

// Grade lists for form dropdowns.
var vScaleGrades = []string{
	"VB", "V0", "V1", "V2", "V3", "V4", "V5", "V6", "V7", "V8", "V9", "V10", "V11", "V12",
}

var ydsGrades = []string{
	"5.5", "5.6", "5.7", "5.8-", "5.8", "5.8+",
	"5.9-", "5.9", "5.9+",
	"5.10-", "5.10", "5.10+",
	"5.11-", "5.11", "5.11+",
	"5.12-", "5.12", "5.12+",
	"5.13-", "5.13", "5.13+",
	"5.14-", "5.14",
}

var circuitColors = []string{
	"red", "orange", "yellow", "green", "blue", "purple", "pink", "white", "black",
}
