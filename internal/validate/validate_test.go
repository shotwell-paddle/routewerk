package validate

import "testing"

func TestHexColor(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"#fff", true},
		{"#FF00AA", true},
		{"#123abc", true},
		{"", false},
		{"fff", false},
		{"#gg0000", false},
		{"#12345", false},
		{"#1234567", false},
	}
	for _, tc := range tests {
		if got := HexColor(tc.input); got != tc.want {
			t.Errorf("HexColor(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestSafeColor(t *testing.T) {
	if got := SafeColor("#ff0000", "#999"); got != "#ff0000" {
		t.Errorf("SafeColor valid = %q", got)
	}
	if got := SafeColor("invalid", "#999"); got != "#999" {
		t.Errorf("SafeColor invalid = %q", got)
	}
}

func TestResourceID(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"abc123", true},
		{"a-b_c", true},
		{"", false},
		{"a b", false},
		{"a/b", false},
	}
	for _, tc := range tests {
		if got := ResourceID(tc.input); got != tc.want {
			t.Errorf("ResourceID(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestEmail(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"test@example.com", true},
		{"a@b.c", true},
		{"", false},
		{"nope", false},
		{"@missing.com", false},
	}
	for _, tc := range tests {
		if got := Email(tc.input); got != tc.want {
			t.Errorf("Email(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestRequired(t *testing.T) {
	if msg := Required("hello", "Name"); msg != "" {
		t.Errorf("Required non-empty = %q", msg)
	}
	if msg := Required("", "Name"); msg == "" {
		t.Error("Required empty should return error")
	}
	if msg := Required("  ", "Name"); msg == "" {
		t.Error("Required whitespace should return error")
	}
}

func TestMaxLength(t *testing.T) {
	if msg := MaxLength("hi", 5, "Name"); msg != "" {
		t.Errorf("MaxLength short = %q", msg)
	}
	if msg := MaxLength("toolong", 3, "Name"); msg == "" {
		t.Error("MaxLength over should return error")
	}
}

func TestMinLength(t *testing.T) {
	if msg := MinLength("hello", 3, "Name"); msg != "" {
		t.Errorf("MinLength long = %q", msg)
	}
	if msg := MinLength("hi", 5, "Name"); msg == "" {
		t.Error("MinLength short should return error")
	}
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"Hello World", "hello-world"},
		{"  Boulder Gym  ", "boulder-gym"},
		{"LEF Climbing (Downtown)", "lef-climbing-downtown"},
		{"café", "caf"},
		{"---test---", "test"},
		{"", ""},
	}
	for _, tc := range tests {
		if got := Slugify(tc.input); got != tc.want {
			t.Errorf("Slugify(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestRouteType(t *testing.T) {
	if !RouteType("boulder") {
		t.Error("boulder should be valid")
	}
	if !RouteType("") {
		t.Error("empty should be valid (no filter)")
	}
	if RouteType("invalid") {
		t.Error("invalid should not be valid")
	}
}
