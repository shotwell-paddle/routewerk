// Package validate provides shared input validation utilities used across
// handlers and services. Centralizes rules that were previously duplicated
// in multiple handler files.
package validate

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// ── Regex patterns ──────────────────────────────────────────────

var (
	// HexColor matches #RGB or #RRGGBB (case-insensitive).
	hexColorRe = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}){1,2}$`)

	// UUIDV4 matches UUID v4 or similar slug IDs used in the app.
	resourceIDRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

	// emailRe is a simplified email check (not RFC 5322, but good enough
	// for form validation — the real check is the confirmation email).
	emailRe = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
)

// ── Color ───────────────────────────────────────────────────────

// HexColor returns true if s is a valid hex color (#RGB or #RRGGBB).
func HexColor(s string) bool {
	return hexColorRe.MatchString(s)
}

// SafeColor returns the color if valid, or a safe default.
func SafeColor(color, fallback string) string {
	if HexColor(color) {
		return color
	}
	return fallback
}

// ── Identifiers ─────────────────────────────────────────────────

// ResourceID returns true if s looks like a valid resource ID (UUID, slug, etc.).
func ResourceID(s string) bool {
	return resourceIDRe.MatchString(s)
}

// ── Email ───────────────────────────────────────────────────────

// Email returns true if s looks like a plausible email address.
func Email(s string) bool {
	return emailRe.MatchString(s)
}

// ── Strings ─────────────────────────────────────────────────────

// Required returns an error message if s is empty after trimming whitespace.
func Required(s, fieldName string) string {
	if strings.TrimSpace(s) == "" {
		return fieldName + " is required"
	}
	return ""
}

// MaxLength returns an error message if s exceeds max runes.
func MaxLength(s string, max int, fieldName string) string {
	if utf8.RuneCountInString(s) > max {
		return fieldName + " is too long"
	}
	return ""
}

// MinLength returns an error message if s is shorter than min runes.
func MinLength(s string, min int, fieldName string) string {
	if utf8.RuneCountInString(s) < min {
		return fieldName + " is too short"
	}
	return ""
}

// ── Slugs ───────────────────────────────────────────────────────

// Slugify creates a URL-safe slug from a string.
func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	result := make([]byte, 0, len(s))
	lastDash := false
	for _, b := range []byte(s) {
		if (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') {
			result = append(result, b)
			lastDash = false
		} else if !lastDash && len(result) > 0 {
			result = append(result, '-')
			lastDash = true
		}
	}
	return strings.Trim(string(result), "-")
}

// ── Route types ─────────────────────────────────────────────────

// ValidRouteTypes is the allow-list for route type filter parameters.
var ValidRouteTypes = map[string]bool{
	"":         true,
	"boulder":  true,
	"sport":    true,
	"top_rope": true,
}

// RouteType returns true if s is a valid route type.
func RouteType(s string) bool {
	return ValidRouteTypes[s]
}
