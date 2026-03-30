package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shotwell-paddle/routewerk/internal/auth"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

const testJWTSecret = "test-secret-key-for-middleware-tests"

// ── JWT Authenticate ────────────────────────────────────────

func TestAuthenticate_ValidToken(t *testing.T) {
	token, _, err := auth.GenerateAccessToken("user-123", "test@example.com", testJWTSecret, 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	var gotUserID, gotEmail string
	handler := Authenticate(testJWTSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID = GetUserID(r.Context())
		gotEmail = GetEmail(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if gotUserID != "user-123" {
		t.Errorf("UserID = %q, want %q", gotUserID, "user-123")
	}
	if gotEmail != "test@example.com" {
		t.Errorf("Email = %q, want %q", gotEmail, "test@example.com")
	}
}

func TestAuthenticate_MissingHeader(t *testing.T) {
	handler := Authenticate(testJWTSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestAuthenticate_InvalidFormat(t *testing.T) {
	handler := Authenticate(testJWTSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	for _, header := range []string{"Basic abc123", "just-a-token", "Bearer"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
		req.Header.Set("Authorization", header)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("header %q: expected 401, got %d", header, rec.Code)
		}
	}
}

func TestAuthenticate_ExpiredToken(t *testing.T) {
	// Generate a token that's already expired
	token, _, err := auth.GenerateAccessToken("user-123", "test@example.com", testJWTSecret, -1*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	handler := Authenticate(testJWTSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for expired token")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for expired token, got %d", rec.Code)
	}
}

func TestAuthenticate_WrongSecret(t *testing.T) {
	token, _, err := auth.GenerateAccessToken("user-123", "test@example.com", "correct-secret", 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	handler := Authenticate("wrong-secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for wrong secret")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong secret, got %d", rec.Code)
	}
}

func TestAuthenticate_CaseInsensitiveBearer(t *testing.T) {
	token, _, err := auth.GenerateAccessToken("user-123", "test@example.com", testJWTSecret, 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	handler := Authenticate(testJWTSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// RFC 7235 says scheme comparison is case-insensitive
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("lowercase 'bearer' should be accepted, got %d", rec.Code)
	}
}

// ── AuthenticateAllowExpired ────────────────────────────────

func TestAuthenticateAllowExpired_AcceptsExpiredToken(t *testing.T) {
	token, _, err := auth.GenerateAccessToken("user-456", "refresh@example.com", testJWTSecret, -5*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	var gotUserID string
	handler := AuthenticateAllowExpired(testJWTSecret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID = GetUserID(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for expired token in AllowExpired, got %d", rec.Code)
	}
	if gotUserID != "user-456" {
		t.Errorf("UserID = %q, want %q", gotUserID, "user-456")
	}
}

func TestAuthenticateAllowExpired_RejectsWrongSignature(t *testing.T) {
	token, _, err := auth.GenerateAccessToken("user-456", "refresh@example.com", "correct-secret", -5*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	handler := AuthenticateAllowExpired("wrong-secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for wrong signature")
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

// ── Context getters ─────────────────────────────────────────

func TestGetUserID_EmptyContext(t *testing.T) {
	ctx := context.Background()
	if got := GetUserID(ctx); got != "" {
		t.Errorf("GetUserID on empty context = %q, want empty", got)
	}
}

func TestGetEmail_EmptyContext(t *testing.T) {
	ctx := context.Background()
	if got := GetEmail(ctx); got != "" {
		t.Errorf("GetEmail on empty context = %q, want empty", got)
	}
}

func TestGetWebUser_EmptyContext(t *testing.T) {
	ctx := context.Background()
	if got := GetWebUser(ctx); got != nil {
		t.Errorf("GetWebUser on empty context should be nil")
	}
}

func TestGetWebLocationID_EmptyContext(t *testing.T) {
	ctx := context.Background()
	if got := GetWebLocationID(ctx); got != "" {
		t.Errorf("GetWebLocationID on empty context = %q, want empty", got)
	}
}

func TestGetWebRole_EmptyContext(t *testing.T) {
	ctx := context.Background()
	if got := GetWebRole(ctx); got != "" {
		t.Errorf("GetWebRole on empty context = %q, want empty", got)
	}
}

func TestGetMembership_EmptyContext(t *testing.T) {
	ctx := context.Background()
	if got := GetMembership(ctx); got != nil {
		t.Errorf("GetMembership on empty context should be nil")
	}
}

// ── Role hierarchy (hasRole) ────────────────────────────────

func TestHasRole_HierarchyRespected(t *testing.T) {
	tests := []struct {
		userRole string
		required []string
		want     bool
	}{
		{"climber", []string{"climber"}, true},
		{"setter", []string{"climber"}, true},    // setter > climber
		{"head_setter", []string{"setter"}, true}, // head_setter > setter
		{"org_admin", []string{"climber"}, true},  // org_admin > everything
		{"climber", []string{"setter"}, false},    // climber < setter
		{"setter", []string{"head_setter"}, false},
		{"climber", []string{"org_admin"}, false},
		{"unknown_role", []string{"climber"}, false},
	}

	for _, tc := range tests {
		got := hasRole(tc.userRole, tc.required)
		if got != tc.want {
			t.Errorf("hasRole(%q, %v) = %v, want %v", tc.userRole, tc.required, got, tc.want)
		}
	}
}

func TestHasRole_MultipleRequiredRoles(t *testing.T) {
	// If required includes setter AND head_setter, setter should qualify
	// (we use the lowest required rank as threshold)
	if !hasRole("setter", []string{"setter", "head_setter"}) {
		t.Error("setter should match when setter is in required list")
	}
	if !hasRole("head_setter", []string{"setter", "head_setter"}) {
		t.Error("head_setter should match when setter is in required list")
	}
	if hasRole("climber", []string{"setter", "head_setter"}) {
		t.Error("climber should NOT match setter or head_setter")
	}
}

// ── bestRole ────────────────────────────────────────────────

func TestBestRole_PicksHighest(t *testing.T) {
	memberships := []model.UserMembership{
		{Role: "climber"},
		{Role: "head_setter"},
		{Role: "setter"},
	}
	got := bestRole(memberships, nil)
	if got != "head_setter" {
		t.Errorf("bestRole = %q, want %q", got, "head_setter")
	}
}

func TestBestRole_EmptyMemberships(t *testing.T) {
	got := bestRole(nil, nil)
	if got != "climber" {
		t.Errorf("bestRole with no memberships = %q, want %q", got, "climber")
	}
}

func TestBestRole_UnknownRoleSkipped(t *testing.T) {
	memberships := []model.UserMembership{
		{Role: "unknown"},
		{Role: "setter"},
	}
	got := bestRole(memberships, nil)
	if got != "setter" {
		t.Errorf("bestRole should skip unknown roles, got %q", got)
	}
}

// ── Session token helpers ───────────────────────────────────

func TestGenerateSessionToken_UniqueAndCorrectLength(t *testing.T) {
	token1, hash1, err := GenerateSessionToken()
	if err != nil {
		t.Fatalf("GenerateSessionToken error: %v", err)
	}
	token2, hash2, err := GenerateSessionToken()
	if err != nil {
		t.Fatalf("GenerateSessionToken error: %v", err)
	}

	// Tokens should be 64 hex chars (32 bytes)
	if len(token1) != 64 {
		t.Errorf("token length = %d, want 64", len(token1))
	}
	// Hashes should be 64 hex chars (SHA-256 = 32 bytes)
	if len(hash1) != 64 {
		t.Errorf("hash length = %d, want 64", len(hash1))
	}

	// Tokens should be unique
	if token1 == token2 {
		t.Error("two generated tokens should not be equal")
	}
	if hash1 == hash2 {
		t.Error("two generated hashes should not be equal")
	}

	// Hash should match re-hashing the token
	if HashSessionToken(token1) != hash1 {
		t.Error("HashSessionToken(token) should match the hash from GenerateSessionToken")
	}
}

func TestHashSessionToken_Deterministic(t *testing.T) {
	token := "test-session-token-value"
	h1 := HashSessionToken(token)
	h2 := HashSessionToken(token)
	if h1 != h2 {
		t.Error("HashSessionToken should be deterministic")
	}
	if len(h1) != 64 {
		t.Errorf("hash length = %d, want 64", len(h1))
	}
}

func TestHashSessionToken_DifferentTokensDifferentHashes(t *testing.T) {
	h1 := HashSessionToken("token-a")
	h2 := HashSessionToken("token-b")
	if h1 == h2 {
		t.Error("different tokens should produce different hashes")
	}
}

// ── SetSessionCookie / ClearSessionCookie ───────────────────

func TestSetSessionCookie_DevMode(t *testing.T) {
	sm := NewSessionManager(nil, nil, true) // isDev=true → secure=false
	rec := httptest.NewRecorder()
	sm.SetSessionCookie(rec, "my-token", 24*time.Hour)

	cookies := rec.Result().Cookies()
	var found *http.Cookie
	for _, c := range cookies {
		if c.Name == SessionCookieName {
			found = c
		}
	}
	if found == nil {
		t.Fatal("session cookie not set")
	}
	if found.Value != "my-token" {
		t.Errorf("cookie value = %q, want %q", found.Value, "my-token")
	}
	if found.Secure {
		t.Error("cookie should not be Secure in dev mode")
	}
	if !found.HttpOnly {
		t.Error("session cookie should be HttpOnly")
	}
	if found.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v, want Lax", found.SameSite)
	}
}

func TestSetSessionCookie_ProductionMode(t *testing.T) {
	sm := NewSessionManager(nil, nil, false) // isDev=false → secure=true
	rec := httptest.NewRecorder()
	sm.SetSessionCookie(rec, "my-token", 24*time.Hour)

	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionCookieName && !c.Secure {
			t.Error("cookie should have Secure flag in production")
		}
	}
}

func TestClearSessionCookie(t *testing.T) {
	sm := NewSessionManager(nil, nil, true)
	rec := httptest.NewRecorder()
	sm.ClearSessionCookie(rec)

	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionCookieName {
			if c.MaxAge != -1 {
				t.Errorf("MaxAge = %d, want -1 (expire)", c.MaxAge)
			}
			if c.Value != "" {
				t.Errorf("Value = %q, want empty", c.Value)
			}
		}
	}
}
