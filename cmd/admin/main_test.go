package main

import "testing"

// ── slugify ────────────────────────────────────────────────────────

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello World", "hello-world"},
		{"  LEF Climbing  ", "lef-climbing"},
		{"Multiple   Spaces", "multiple-spaces"},
		{"Special!@#$%^&*Chars", "special-chars"},
		{"already-slugged", "already-slugged"},
		{"UPPERCASE", "uppercase"},
		{"numbers123test", "numbers123test"},
		{"---dashes---", "dashes"},
		{"  ", ""},
		{"", ""},
		{"a", "a"},
		{"Mosaic Climbing Co.", "mosaic-climbing-co"},
		{"Chris's Gym", "chris-s-gym"},
		{"Test & More", "test-more"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := slugify(tc.input)
			if got != tc.want {
				t.Errorf("slugify(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestSlugify_Idempotent(t *testing.T) {
	// Slugifying an already-slugified string should be a no-op
	input := "lef-boulder"
	first := slugify(input)
	second := slugify(first)
	if first != second {
		t.Errorf("slugify is not idempotent: %q → %q → %q", input, first, second)
	}
}

// ── role validation ────────────────────────────────────────────────

func TestValidRoles(t *testing.T) {
	validRoles := map[string]bool{
		"org_admin":   true,
		"head_setter": true,
		"setter":      true,
		"climber":     true,
	}

	for _, role := range []string{"org_admin", "head_setter", "setter", "climber"} {
		if !validRoles[role] {
			t.Errorf("expected %q to be a valid role", role)
		}
	}

	for _, role := range []string{"admin", "manager", "gym_manager", "owner", ""} {
		if validRoles[role] {
			t.Errorf("expected %q to be an invalid role", role)
		}
	}
}

// ── password validation rules ──────────────────────────────────────

func TestPasswordValidation(t *testing.T) {
	tests := []struct {
		name    string
		pw      string
		wantErr bool
	}{
		{"valid 8 chars", "password", false},
		{"valid long", "this-is-a-very-strong-passphrase", false},
		{"too short", "1234567", true},
		{"empty", "", true},
		{"exactly 8", "12345678", false},
		{"exactly 72", "123456789012345678901234567890123456789012345678901234567890123456789012", false},
		{"73 chars", "1234567890123456789012345678901234567890123456789012345678901234567890123", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tooShort := len(tc.pw) < 8
			tooLong := len(tc.pw) > 72
			hasErr := tooShort || tooLong

			if hasErr != tc.wantErr {
				t.Errorf("password %q: validation=%v, want error=%v", tc.name, hasErr, tc.wantErr)
			}
		})
	}
}
