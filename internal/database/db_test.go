package database

import "testing"

func TestEnforceTLS(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "replaces sslmode=disable with require",
			input:    "postgres://user:pass@host:5432/db?sslmode=disable",
			expected: "postgres://user:pass@host:5432/db?sslmode=require",
		},
		{
			name:     "adds sslmode=require when missing",
			input:    "postgres://user:pass@host:5432/db",
			expected: "postgres://user:pass@host:5432/db?sslmode=require",
		},
		{
			name:     "appends with & when other params exist",
			input:    "postgres://user:pass@host:5432/db?application_name=routewerk",
			expected: "postgres://user:pass@host:5432/db?application_name=routewerk&sslmode=require",
		},
		{
			name:     "leaves sslmode=require untouched",
			input:    "postgres://user:pass@host:5432/db?sslmode=require",
			expected: "postgres://user:pass@host:5432/db?sslmode=require",
		},
		{
			name:     "leaves sslmode=verify-full untouched",
			input:    "postgres://user:pass@host:5432/db?sslmode=verify-full",
			expected: "postgres://user:pass@host:5432/db?sslmode=verify-full",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := enforceTLS(tt.input)
			if got != tt.expected {
				t.Errorf("enforceTLS(%q)\n  got:  %q\n  want: %q", tt.input, got, tt.expected)
			}
		})
	}
}
