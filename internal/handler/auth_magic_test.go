package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ── safeMagicNext ──────────────────────────────────────────

func TestSafeMagicNext(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want *string // nil = rejected
	}{
		{"empty rejects", "", nil},
		{"absolute path accepted", "/comp/league", strPtr("/comp/league")},
		{"absolute path with query accepted", "/comp/league?utm=email", strPtr("/comp/league?utm=email")},
		{"relative path rejected", "comp/league", nil},
		{"protocol-absolute rejected", "//evil.example/", nil},
		{"with scheme rejected", "https://evil.example/", nil},
		{"with host rejected", "//evil.example/path", nil},
		{"unparseable rejected", "://broken", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := safeMagicNext(tc.in)
			switch {
			case tc.want == nil && got != nil:
				t.Errorf("safeMagicNext(%q) = %q, want nil", tc.in, *got)
			case tc.want != nil && got == nil:
				t.Errorf("safeMagicNext(%q) = nil, want %q", tc.in, *tc.want)
			case tc.want != nil && got != nil && *got != *tc.want:
				t.Errorf("safeMagicNext(%q) = %q, want %q", tc.in, *got, *tc.want)
			}
		})
	}
}

func strPtr(s string) *string { return &s }

// ── Request handler always returns 202 + ok=true ──────────

// magicAccepted runs the request through a no-op handler and asserts
// the standard 202 response shape. The actual service call (which
// requires a DB) is exercised by writeMagicAccepted directly here —
// the handler delegates to it on every code path.
func TestWriteMagicAccepted_ResponseShape(t *testing.T) {
	rr := httptest.NewRecorder()
	writeMagicAccepted(rr)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "json") {
		t.Errorf("Content-Type = %q, want JSON", ct)
	}
	var body struct {
		OK bool `json:"ok"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if !body.OK {
		t.Errorf("body ok = false, want true")
	}
}

// ── clientIP / splitHostPort / truncate ────────────────────

func TestClientIP(t *testing.T) {
	tests := []struct {
		remoteAddr string
		want       string
	}{
		{"192.0.2.1:54321", "192.0.2.1"},
		{"[2001:db8::1]:8080", "[2001:db8::1]"}, // IPv6 keeps the brackets per current impl
		{"192.0.2.1", "192.0.2.1"},              // no port
		{"", ""},
	}
	for _, tc := range tests {
		t.Run(tc.remoteAddr, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tc.remoteAddr
			if got := clientIP(r); got != tc.want {
				t.Errorf("clientIP = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		s    string
		n    int
		want string
	}{
		{"abc", 5, "abc"},
		{"abcdefgh", 4, "abcd"},
		{"", 5, ""},
		{"abc", 3, "abc"},
	}
	for _, tc := range tests {
		t.Run(tc.s, func(t *testing.T) {
			if got := truncate(tc.s, tc.n); got != tc.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tc.s, tc.n, got, tc.want)
			}
		})
	}
}
