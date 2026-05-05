package service

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shotwell-paddle/routewerk/internal/model"
)

// ── Token gen/hash roundtrip ───────────────────────────────

func TestGenerateMagicToken_HasExpectedShape(t *testing.T) {
	token, hash, err := generateMagicToken()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	// Decoded plaintext is exactly 32 bytes.
	buf, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(buf) != MagicLinkTokenBytes {
		t.Errorf("decoded len = %d, want %d", len(buf), MagicLinkTokenBytes)
	}
	// SHA-256 produces 32 bytes.
	if len(hash) != 32 {
		t.Errorf("hash len = %d, want 32", len(hash))
	}
}

func TestGenerateMagicToken_DifferentEachCall(t *testing.T) {
	t1, h1, _ := generateMagicToken()
	t2, h2, _ := generateMagicToken()
	if t1 == t2 {
		t.Error("two consecutive tokens collided — randomness broken?")
	}
	if string(h1) == string(h2) {
		t.Error("two consecutive hashes collided")
	}
}

func TestHashMagicToken_RoundtripWithGenerate(t *testing.T) {
	token, expectedHash, err := generateMagicToken()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	gotHash, err := hashMagicToken(token)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if string(gotHash) != string(expectedHash) {
		t.Error("hash from token doesn't match hash returned by generate")
	}
}

func TestHashMagicToken_BadInputs(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"empty string", ""},
		{"non-base64", "not!valid!base64!at!all"},
		{"valid base64 but wrong length", base64.RawURLEncoding.EncodeToString([]byte("too-short"))},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := hashMagicToken(tc.in); err == nil {
				t.Errorf("want error for input %q, got nil", tc.in)
			}
		})
	}
}

// ── Email normalization ────────────────────────────────────

func TestNormalizeEmail(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"foo@bar.com", "foo@bar.com"},
		{"  Foo@BAR.com  ", "foo@bar.com"},
		{"", ""},
		{"   ", ""},
		{"User+Tag@Example.IO", "user+tag@example.io"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := normalizeEmail(tc.in); got != tc.want {
				t.Errorf("normalizeEmail(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// ── nilIfEmpty ─────────────────────────────────────────────

func TestNilIfEmpty(t *testing.T) {
	if got := nilIfEmpty(""); got != nil {
		t.Errorf("nilIfEmpty(\"\") = %v, want nil", got)
	}
	if got := nilIfEmpty("x"); got == nil || *got != "x" {
		t.Errorf("nilIfEmpty(%q) = %v, want pointer to %q", "x", got, "x")
	}
}

// ── Model behavior ─────────────────────────────────────────

func TestMagicLinkToken_IsExpired(t *testing.T) {
	now := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{"future expiry", now.Add(15 * time.Minute), false},
		{"exact now", now, false}, // After is strict; equality is not expired
		{"past expiry", now.Add(-1 * time.Second), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tok := model.MagicLinkToken{ExpiresAt: tc.expiresAt}
			if got := tok.IsExpired(now); got != tc.want {
				t.Errorf("IsExpired = %v, want %v", got, tc.want)
			}
		})
	}
}

// ── ErrMagicLinkInvalid is the only caller-facing error ────

func TestErrMagicLinkInvalid_IsExported(t *testing.T) {
	// Sanity: callers in the web handler check errors.Is against this
	// sentinel; if it gets renamed/unexported the verify handler breaks
	// silently. This test fails loudly when that happens.
	if !errors.Is(ErrMagicLinkInvalid, ErrMagicLinkInvalid) {
		t.Fatal("ErrMagicLinkInvalid is not comparable to itself — broken sentinel")
	}
	if !strings.Contains(ErrMagicLinkInvalid.Error(), "invalid") {
		t.Errorf("ErrMagicLinkInvalid message = %q, want it to mention 'invalid'", ErrMagicLinkInvalid.Error())
	}
}
