package auth

import (
	"strings"
	"testing"
	"time"
)

const testSecret = "super-secret-jwt-key-for-tests"

// ── Password hashing ────────────────────────────────────────

func TestHashPassword_Roundtrip(t *testing.T) {
	hash, err := HashPassword("my-secure-password")
	if err != nil {
		t.Fatalf("HashPassword error: %v", err)
	}
	if hash == "" {
		t.Fatal("hash should not be empty")
	}
	if hash == "my-secure-password" {
		t.Fatal("hash should not equal plaintext")
	}
	if !CheckPassword("my-secure-password", hash) {
		t.Error("CheckPassword should return true for correct password")
	}
	if CheckPassword("wrong-password", hash) {
		t.Error("CheckPassword should return false for wrong password")
	}
}

func TestHashPassword_DifferentHashesForSamePassword(t *testing.T) {
	h1, _ := HashPassword("password")
	h2, _ := HashPassword("password")
	if h1 == h2 {
		t.Error("bcrypt should produce different hashes (different salts)")
	}
}

// ── Access tokens ───────────────────────────────────────────

func TestGenerateAccessToken_Valid(t *testing.T) {
	token, expiresAt, err := GenerateAccessToken("user-1", "user@test.com", testSecret, 15*time.Minute)
	if err != nil {
		t.Fatalf("GenerateAccessToken error: %v", err)
	}
	if token == "" {
		t.Fatal("token should not be empty")
	}
	if expiresAt.Before(time.Now()) {
		t.Error("expiresAt should be in the future")
	}
}

func TestValidateAccessToken_Roundtrip(t *testing.T) {
	token, _, err := GenerateAccessToken("user-2", "user2@test.com", testSecret, 15*time.Minute)
	if err != nil {
		t.Fatalf("GenerateAccessToken error: %v", err)
	}

	claims, err := ValidateAccessToken(token, testSecret)
	if err != nil {
		t.Fatalf("ValidateAccessToken error: %v", err)
	}
	if claims.UserID != "user-2" {
		t.Errorf("UserID = %q, want %q", claims.UserID, "user-2")
	}
	if claims.Email != "user2@test.com" {
		t.Errorf("Email = %q, want %q", claims.Email, "user2@test.com")
	}
	if claims.Issuer != "routewerk" {
		t.Errorf("Issuer = %q, want %q", claims.Issuer, "routewerk")
	}
}

func TestValidateAccessToken_ExpiredFails(t *testing.T) {
	token, _, err := GenerateAccessToken("user-3", "expired@test.com", testSecret, -1*time.Minute)
	if err != nil {
		t.Fatalf("GenerateAccessToken error: %v", err)
	}

	_, err = ValidateAccessToken(token, testSecret)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestValidateAccessToken_WrongSecretFails(t *testing.T) {
	token, _, _ := GenerateAccessToken("user-4", "user@test.com", "correct-secret", 15*time.Minute)

	_, err := ValidateAccessToken(token, "wrong-secret")
	if err == nil {
		t.Fatal("expected error for wrong secret")
	}
}

func TestValidateAccessToken_GarbageTokenFails(t *testing.T) {
	_, err := ValidateAccessToken("not.a.real.jwt.token", testSecret)
	if err == nil {
		t.Fatal("expected error for garbage token")
	}
}

// ── ParseExpiredClaims ──────────────────────────────────────

func TestParseExpiredClaims_AcceptsExpired(t *testing.T) {
	token, _, _ := GenerateAccessToken("user-5", "refresh@test.com", testSecret, -10*time.Minute)

	claims, err := ParseExpiredClaims(token, testSecret)
	if err != nil {
		t.Fatalf("ParseExpiredClaims should accept expired token: %v", err)
	}
	if claims.UserID != "user-5" {
		t.Errorf("UserID = %q, want %q", claims.UserID, "user-5")
	}
	if claims.Email != "refresh@test.com" {
		t.Errorf("Email = %q, want %q", claims.Email, "refresh@test.com")
	}
}

func TestParseExpiredClaims_RejectsWrongSignature(t *testing.T) {
	token, _, _ := GenerateAccessToken("user-5", "refresh@test.com", "correct-secret", -10*time.Minute)

	_, err := ParseExpiredClaims(token, "wrong-secret")
	if err == nil {
		t.Fatal("expected error for wrong signature")
	}
}

// ── Refresh tokens ──────────────────────────────────────────

func TestGenerateRefreshToken_UniqueAndSized(t *testing.T) {
	t1, err := GenerateRefreshToken()
	if err != nil {
		t.Fatalf("GenerateRefreshToken error: %v", err)
	}
	t2, err := GenerateRefreshToken()
	if err != nil {
		t.Fatalf("GenerateRefreshToken error: %v", err)
	}

	// 32 bytes hex = 64 chars
	if len(t1) != 64 {
		t.Errorf("refresh token length = %d, want 64", len(t1))
	}
	if t1 == t2 {
		t.Error("two refresh tokens should not be equal")
	}
}

func TestRefreshToken_HashAndCheck(t *testing.T) {
	token, _ := GenerateRefreshToken()
	hash := HashRefreshToken(token)

	if !CheckRefreshToken(token, hash) {
		t.Error("CheckRefreshToken should return true for correct token")
	}
	if CheckRefreshToken("wrong-token", hash) {
		t.Error("CheckRefreshToken should return false for wrong token")
	}
}

// ── Edge cases ──────────────────────────────────────────────

func TestValidateAccessToken_EmptyString(t *testing.T) {
	_, err := ValidateAccessToken("", testSecret)
	if err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestParseExpiredClaims_EmptyString(t *testing.T) {
	_, err := ParseExpiredClaims("", testSecret)
	if err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestGenerateAccessToken_EmptySecret(t *testing.T) {
	// Should still generate a valid token (HMAC with empty key is technically valid)
	token, _, err := GenerateAccessToken("user", "user@test.com", "", 15*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// But it should validate with the same empty secret
	claims, err := ValidateAccessToken(token, "")
	if err != nil {
		t.Fatalf("should validate with matching empty secret: %v", err)
	}
	if claims.UserID != "user" {
		t.Errorf("UserID = %q, want %q", claims.UserID, "user")
	}
}

func TestAccessToken_ContainsThreeParts(t *testing.T) {
	token, _, _ := GenerateAccessToken("user", "user@test.com", testSecret, 15*time.Minute)
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Errorf("JWT should have 3 dot-separated parts, got %d", len(parts))
	}
}
