package webhandler

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

// ── Role Display ─────────────────────────────────────────────

// roleDisplayName returns a human-friendly label for a role key.
func roleDisplayName(role string) string {
	switch role {
	case middleware.RoleOrgAdmin:
		return "Admin"
	case middleware.RoleGymManager:
		return "Manager"
	case middleware.RoleHeadSetter:
		return "Head Setter"
	case middleware.RoleSetter:
		return "Setter"
	default:
		return "Climber"
	}
}

// ── Context Helpers ──────────────────────────────────────────

// templateDataFromContext builds the shared TemplateData from the session context.
// Falls back to safe defaults if no session is present.
func templateDataFromContext(r *http.Request, activeNav string) TemplateData {
	user := middleware.GetWebUser(r.Context())
	effectiveRole := middleware.GetWebRole(r.Context())
	realRole := middleware.GetWebRealRole(r.Context())

	td := TemplateData{
		ActiveNav: activeNav,
		CSRFToken: middleware.TokenFromRequest(r),
		RealRole:  realRole,
	}

	if user != nil {
		td.UserName = user.DisplayName
		if len(user.DisplayName) > 0 {
			td.UserInitial = strings.ToUpper(user.DisplayName[:1])
		}
	} else {
		td.UserName = "Guest"
		td.UserInitial = "?"
	}

	// If the effective role differs from real, we have a view-as override active
	if effectiveRole != realRole {
		td.ViewAsRole = effectiveRole
	}

	// Determine role display + setter flag from effective role
	td.UserRole = roleDisplayName(effectiveRole)
	td.IsSetter = effectiveRole != middleware.RoleClimber
	td.IsHeadSetter = middleware.RoleRankValue(effectiveRole) >= 3
	td.IsOrgAdmin = middleware.RoleRankValue(effectiveRole) >= 5
	// IsAppAdmin is a per-user flag (not a role), but it must still respect
	// the view-as override: the whole point of view-as is to preview the app
	// as a lower-privilege user, so the Admin sidebar section should disappear
	// while it's active. Security is still enforced server-side by
	// RequireAppAdmin, which checks the real user.IsAppAdmin — this only
	// controls UI visibility.
	if user != nil && effectiveRole == realRole {
		td.IsAppAdmin = user.IsAppAdmin
	}

	return td
}

// enrichTemplateData populates location-switcher and view-as-role data.
// Called from render() so every page gets this data for the sidebar.
func (h *Handler) enrichTemplateData(r *http.Request, td *TemplateData) {
	ctx := r.Context()
	user := middleware.GetWebUser(ctx)
	if user == nil {
		return
	}

	// Load current location
	locationID := middleware.GetWebLocationID(ctx)
	if locationID != "" && td.Location == nil {
		loc, err := h.locationRepo.GetByID(ctx, locationID)
		if err == nil && loc != nil {
			td.Location = loc
		}
	}
	td.ProgressionsEnabled = td.Location != nil && td.Location.ProgressionsEnabled

	// Load user's available locations for the switcher
	locations, err := h.locationRepo.ListForUser(ctx, user.ID)
	if err != nil {
		slog.Error("failed to load user locations", "user_id", user.ID, "error", err)
	} else {
		td.UserLocations = locations
		td.HasMultipleLocations = len(locations) > 1
	}

	// Unread notification count is loaded asynchronously by the sidebar
	// via HTMX polling against /notifications/badge — keeping it off the
	// page-load critical path. See perf audit 2026-04-22 finding #1.

	// Build view-as options if user has a role higher than setter
	realRank := middleware.RoleRankValue(td.RealRole)
	if realRank >= 2 { // setter or above
		td.CanViewAs = realRank > 2 // only show if above setter (head_setter+)

		if td.CanViewAs {
			// Build options for all roles below the user's real role
			td.ViewAsOptions = []ViewAsOption{
				{Value: "", Label: roleDisplayName(td.RealRole) + " (actual)", Active: td.ViewAsRole == ""},
			}
			roleOrder := []string{middleware.RoleGymManager, middleware.RoleHeadSetter, middleware.RoleSetter, middleware.RoleClimber}
			for _, role := range roleOrder {
				if middleware.RoleRankValue(role) < realRank {
					td.ViewAsOptions = append(td.ViewAsOptions, ViewAsOption{
						Value:  role,
						Label:  roleDisplayName(role),
						Active: td.ViewAsRole == role,
					})
				}
			}
		}
	}
}

// ── Grade Distribution Helpers ───────────────────────────────

// circuitColorHex maps circuit color names to hex codes for filter chips.
var circuitColorHex = map[string]string{
	"red":    "#e53935",
	"orange": "#fc5200",
	"yellow": "#f9a825",
	"green":  "#2e7d32",
	"blue":   "#1565c0",
	"purple": "#7b1fa2",
	"pink":   "#e91e8a",
	"white":  "#e0e0e0",
	"black":  "#0a0a0a",
}

// ydsGradeBucket maps a YDS grade string (e.g. "5.10a", "5.9") to a display bucket.
// Lexicographic comparison fails for YDS because "5.10" < "5.9" as strings.
func ydsGradeBucket(grade string) string {
	// Strip "5." prefix for easier numeric comparison
	if len(grade) < 3 || grade[:2] != "5." {
		return "5.8-under"
	}
	rest := grade[2:] // e.g. "10a", "9", "12d", "7"

	// Extract the numeric part
	numStr := ""
	for _, ch := range rest {
		if ch >= '0' && ch <= '9' {
			numStr += string(ch)
		} else {
			break
		}
	}

	num := 0
	for _, ch := range numStr {
		num = num*10 + int(ch-'0')
	}

	switch {
	case num <= 8:
		return "5.8-under"
	case num == 9:
		return "5.9"
	case num == 10:
		return "5.10"
	case num == 11:
		return "5.11"
	default:
		return "5.12-up"
	}
}

// expandGradeRange maps a filter chip value to a list of individual grades for SQL IN (...).
func expandGradeRange(value string) []string {
	switch value {
	case "vb-v1":
		return []string{"VB", "V0", "V1"}
	case "v2-v3":
		return []string{"V2", "V3"}
	case "v4-v5":
		return []string{"V4", "V5"}
	case "v6-v7":
		return []string{"V6", "V7"}
	case "v8-up":
		return []string{"V8", "V9", "V10", "V11", "V12", "V13", "V14", "V15", "V16", "V17"}
	case "5.8-under":
		return []string{"5.4", "5.5", "5.5+", "5.6-", "5.6", "5.6+", "5.7-", "5.7", "5.7+", "5.8-", "5.8", "5.8+"}
	case "5.9":
		return []string{"5.9-", "5.9", "5.9+"}
	case "5.10":
		return []string{"5.10-", "5.10", "5.10+", "5.10a", "5.10b", "5.10c", "5.10d"}
	case "5.11":
		return []string{"5.11-", "5.11", "5.11+", "5.11a", "5.11b", "5.11c", "5.11d"}
	case "5.12-up":
		return []string{"5.12-", "5.12", "5.12+", "5.12a", "5.12b", "5.12c", "5.12d",
			"5.13-", "5.13", "5.13+", "5.13a", "5.13b", "5.13c", "5.13d",
			"5.14-", "5.14", "5.14+", "5.14a", "5.14b", "5.14c", "5.14d",
			"5.15a", "5.15b", "5.15c", "5.15d"}
	default:
		return nil
	}
}

// buildGradeGroups converts a raw grade distribution into filter chip groups.
//
// The isSetter flag controls V-scale visibility for circuit boulders:
// when the gym's boulder method is "circuit" and ShowGradesOnCircuit is false,
// climbers only see circuit color chips (no V-scale) because that's all they
// see on route cards. Setters always get both.
func buildGradeGroups(dist []repository.GradeCount, locSettings *model.LocationSettings, isSetter bool) []GradeGroup {
	// Build a lookup from circuit color name to hex
	circuitHex := make(map[string]string)
	var circuitOrder []string // preserve gym's sort order
	for _, cc := range locSettings.Circuits.Colors {
		circuitHex[cc.Name] = cc.Hex
		circuitOrder = append(circuitOrder, cc.Name)
	}

	// Determine whether V-scale chips should be shown for boulders.
	// When the gym uses circuit grading and hides V-grades from climbers,
	// only circuit color chips make sense as filter options.
	showVScale := isSetter ||
		locSettings.Grading.BoulderMethod == "v_scale" ||
		locSettings.Grading.ShowGradesOnCircuit

	// Bucket circuit colors
	colorCounts := make(map[string]int)
	// Bucket YDS grades into ranges
	ydsBuckets := map[string]*GradeGroup{
		"5.8-under": {Label: "5.8 & under", Value: "5.8-under"},
		"5.9":       {Label: "5.9", Value: "5.9"},
		"5.10":      {Label: "5.10", Value: "5.10"},
		"5.11":      {Label: "5.11", Value: "5.11"},
		"5.12-up":   {Label: "5.12+", Value: "5.12-up"},
	}
	// V-scale buckets
	vBuckets := map[string]*GradeGroup{
		"vb-v1": {Label: "VB-V1", Value: "vb-v1"},
		"v2-v3": {Label: "V2-V3", Value: "v2-v3"},
		"v4-v5": {Label: "V4-V5", Value: "v4-v5"},
		"v6-v7": {Label: "V6-V7", Value: "v6-v7"},
		"v8-up": {Label: "V8+", Value: "v8-up"},
	}

	for _, gc := range dist {
		switch gc.GradingSystem {
		case "v_scale":
			if showVScale {
				switch {
				case gc.Grade == "VB" || gc.Grade == "V0" || gc.Grade == "V1":
					vBuckets["vb-v1"].Count += gc.Count
				case gc.Grade == "V2" || gc.Grade == "V3":
					vBuckets["v2-v3"].Count += gc.Count
				case gc.Grade == "V4" || gc.Grade == "V5":
					vBuckets["v4-v5"].Count += gc.Count
				case gc.Grade == "V6" || gc.Grade == "V7":
					vBuckets["v6-v7"].Count += gc.Count
				default:
					vBuckets["v8-up"].Count += gc.Count
				}
			}
		case "circuit":
			colorCounts[gc.Grade] += gc.Count
		case "yds":
			bucket := ydsGradeBucket(gc.Grade)
			if b, ok := ydsBuckets[bucket]; ok {
				b.Count += gc.Count
			}
		}
	}

	var groups []GradeGroup

	// Circuit color groups first (in gym's configured order)
	for _, name := range circuitOrder {
		if count, ok := colorCounts[name]; ok && count > 0 {
			hex := circuitHex[name]
			groups = append(groups, GradeGroup{
				Label:   titleCase(name),
				Value:   "circuit:" + name,
				Count:   count,
				Color:   hex,
				IsColor: true,
			})
		}
	}

	// V-scale groups (boulders) — only when V-grades are visible
	if showVScale {
		for _, key := range []string{"vb-v1", "v2-v3", "v4-v5", "v6-v7", "v8-up"} {
			if vBuckets[key].Count > 0 {
				groups = append(groups, *vBuckets[key])
			}
		}
	}

	// YDS groups (ropes)
	for _, key := range []string{"5.8-under", "5.9", "5.10", "5.11", "5.12-up"} {
		if ydsBuckets[key].Count > 0 {
			groups = append(groups, *ydsBuckets[key])
		}
	}

	return groups
}

// ── Utility Helpers ──────────────────────────────────────────

// realIP extracts the client IP, preferring the value set by Chi's RealIP
// middleware (X-Forwarded-For / X-Real-Ip), and stripping the port suffix
// from RemoteAddr as a fallback.
func realIP(r *http.Request) string {
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	ip = strings.TrimPrefix(ip, "[")
	ip = strings.TrimSuffix(ip, "]")
	return ip
}

// truncateUA caps the user-agent string to a reasonable length to prevent
// storage abuse from maliciously long values.
func truncateUA(ua string) string {
	const maxUALength = 512
	if len(ua) > maxUALength {
		return ua[:maxUALength]
	}
	return ua
}

// ── Template FuncMap Helpers ─────────────────────────────────

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%dh ago", h)
	case d < 7*24*time.Hour:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "yesterday"
		}
		return fmt.Sprintf("%dd ago", days)
	default:
		return t.Format("Jan 2")
	}
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func seq(start, end int) []int {
	s := make([]int, 0, end-start+1)
	for i := start; i <= end; i++ {
		s = append(s, i)
	}
	return s
}

func derefFloat64(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}

func derefInt(n *int) int {
	if n == nil {
		return 0
	}
	return *n
}

func strPtr(s string) *string { return &s }

// ── Seasonal / Event Template Helpers ────────────────────────

// isSeasonal returns true for quest types that are time-limited.
func isSeasonal(qt string) bool {
	return qt == "seasonal" || qt == "event"
}

// daysUntil returns the number of whole days until the given time.
// Returns 0 for nil or past times.
func daysUntil(t *time.Time) int {
	if t == nil {
		return 0
	}
	days := int(time.Until(*t).Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}

// isExpiringSoon returns true if the time is in the future but within 7 days.
func isExpiringSoon(t *time.Time) bool {
	if t == nil {
		return false
	}
	remaining := time.Until(*t)
	return remaining > 0 && remaining < 7*24*time.Hour
}

// hasStarted returns true if the time is nil (no start constraint) or in the past.
func hasStarted(t *time.Time) bool {
	if t == nil {
		return true
	}
	return time.Now().After(*t)
}

// formatDate formats a time pointer as "Jan 2, 2006", or "" if nil.
func formatDate(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("Jan 2, 2006")
}
