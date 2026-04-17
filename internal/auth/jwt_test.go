package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
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

	claims, err := ValidateAccessToken(token, testSecret, true)
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
	if claims.Subject != "user-2" {
		t.Errorf("Subject = %q, want %q", claims.Subject, "user-2")
	}
	if len(claims.Audience) == 0 || claims.Audience[0] != "routewerk-api" {
		t.Errorf("Audience = %v, want [routewerk-api]", claims.Audience)
	}
}

func TestValidateAccessToken_ExpiredFails(t *testing.T) {
	token, _, err := GenerateAccessToken("user-3", "expired@test.com", testSecret, -1*time.Minute)
	if err != nil {
		t.Fatalf("GenerateAccessToken error: %v", err)
	}

	_, err = ValidateAccessToken(token, testSecret, true)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestValidateAccessToken_WrongSecretFails(t *testing.T) {
	token, _, _ := GenerateAccessToken("user-4", "user@test.com", "correct-secret", 15*time.Minute)

	_, err := ValidateAccessToken(token, "wrong-secret", true)
	if err == nil {
		t.Fatal("expected error for wrong secret")
	}
}

func TestValidateAccessToken_GarbageTokenFails(t *testing.T) {
	_, err := ValidateAccessToken("not.a.real.jwt.token", testSecret, true)
	if err == nil {
		t.Fatal("expected error for garbage token")
	}
}

// ── ParseExpiredClaims ──────────────────────────────────────

func TestParseExpiredClaims_AcceptsExpired(t *testing.T) {
	token, _, _ := GenerateAccessToken("user-5", "refresh@test.com", testSecret, -10*time.Minute)

	claims, err := ParseExpiredClaims(token, testSecret, true)
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

	_, err := ParseExpiredClaims(token, "wrong-secret", true)
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
	const secret = "refresh-hmac-secret"
	token, _ := GenerateRefreshToken()
	hash := HashRefreshToken(token, secret)

	if !CheckRefreshToken(token, hash, secret) {
		t.Error("CheckRefreshToken should return true for correct token")
	}
	if CheckRefreshToken("wrong-token", hash, secret) {
		t.Error("CheckRefreshToken should return false for wrong token")
	}
	if CheckRefreshToken(token, hash, "other-secret") {
		t.Error("CheckRefreshToken should return false with wrong key")
	}
}

func TestHashRefreshToken_DeterministicAndKeyed(t *testing.T) {
	// Same input + same key ⇒ same hash (enables O(1) lookup).
	h1 := HashRefreshToken("abc", "key1")
	h2 := HashRefreshToken("abc", "key1")
	if h1 != h2 {
		t.Fatal("HMAC must be deterministic")
	}
	// Different key ⇒ different hash (keyed, not a plain digest).
	h3 := HashRefreshToken("abc", "key2")
	if h1 == h3 {
		t.Error("different keys must produce different hashes")
	}
	// Hex-encoded SHA-256 is 64 characters.
	if len(h1) != 64 {
		t.Errorf("hash length = %d, want 64", len(h1))
	}
}

func TestCheckRefreshTokenBcrypt_LegacyPath(t *testing.T) {
	// The dual-scheme migration relies on still being able to verify old
	// bcrypt rows written before the HMAC switchover.
	token, _ := GenerateRefreshToken()
	legacyHash, err := bcrypt.GenerateFromPassword([]byte(token), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("legacy bcrypt: %v", err)
	}
	if !CheckRefreshTokenBcrypt(token, string(legacyHash)) {
		t.Error("legacy bcrypt verifier must still accept valid tokens")
	}
	if CheckRefreshTokenBcrypt("wrong", string(legacyHash)) {
		t.Error("legacy bcrypt verifier must reject wrong tokens")
	}
}

// ── Edge cases ──────────────────────────────────────────────

func TestValidateAccessToken_EmptyString(t *testing.T) {
	_, err := ValidateAccessToken("", testSecret, true)
	if err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestParseExpiredClaims_EmptyString(t *testing.T) {
	_, err := ParseExpiredClaims("", testSecret, true)
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
	claims, err := ValidateAccessToken(token, "", true)
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

// ── Issuer / audience / alg enforcement ─────────────────────

// mintToken builds a signed token with the given registered claims, bypassing
// GenerateAccessToken so tests can forge bad issuers/audiences.
func mintToken(t *testing.T, reg jwt.RegisteredClaims) string {
	t.Helper()
	claims := Claims{UserID: "user-x", Email: "x@test.com", RegisteredClaims: reg}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return s
}

func TestValidateAccessToken_RejectsWrongIssuer(t *testing.T) {
	now := time.Now()
	tok := mintToken(t, jwt.RegisteredClaims{
		Subject:   "user-x",
		Audience:  jwt.ClaimStrings{"routewerk-api"},
		Issuer:    "evil-service",
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
	})
	if _, err := ValidateAccessToken(tok, testSecret, true); err == nil {
		t.Fatal("expected rejection for wrong issuer")
	}
	// Also fails when audience enforcement is off: issuer is always enforced.
	if _, err := ValidateAccessToken(tok, testSecret, false); err == nil {
		t.Fatal("expected rejection for wrong issuer even without audience enforcement")
	}
}

func TestValidateAccessToken_RejectsWrongAudience(t *testing.T) {
	now := time.Now()
	tok := mintToken(t, jwt.RegisteredClaims{
		Subject:   "user-x",
		Audience:  jwt.ClaimStrings{"other-api"},
		Issuer:    jwtIssuer,
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
	})
	if _, err := ValidateAccessToken(tok, testSecret, true); err == nil {
		t.Fatal("expected rejection for wrong audience when enforcement enabled")
	}
	// With enforcement off, same token should pass (backward-compat path).
	if _, err := ValidateAccessToken(tok, testSecret, false); err != nil {
		t.Fatalf("unexpected rejection when audience enforcement disabled: %v", err)
	}
}

func TestValidateAccessToken_RejectsAlgNone(t *testing.T) {
	// Hand-craft an alg=none token with valid claims. Use jwt.UnsafeAllowNoneSignatureType
	// as the signing key to produce a signature-less token.
	now := time.Now()
	claims := Claims{
		UserID: "user-x",
		Email:  "x@test.com",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-x",
			Audience:  jwt.ClaimStrings{jwtAudience},
			Issuer:    jwtIssuer,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	s, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign none: %v", err)
	}
	if _, err := ValidateAccessToken(s, testSecret, true); err == nil {
		t.Fatal("expected rejection for alg=none token")
	}
	if _, err := ParseExpiredClaims(s, testSecret, true); err == nil {
		t.Fatal("expected ParseExpiredClaims to reject alg=none token")
	}
}

func TestParseExpiredClaims_EnforcesIssuer(t *testing.T) {
	now := time.Now()
	tok := mintToken(t, jwt.RegisteredClaims{
		Subject:   "user-x",
		Audience:  jwt.ClaimStrings{jwtAudience},
		Issuer:    "evil-service",
		IssuedAt:  jwt.NewNumericDate(now.Add(-1 * time.Hour)),
		NotBefore: jwt.NewNumericDate(now.Add(-1 * time.Hour)),
		ExpiresAt: jwt.NewNumericDate(now.Add(-30 * time.Minute)),
	})
	if _, err := ParseExpiredClaims(tok, testSecret, false); err == nil {
		t.Fatal("expected rejection for wrong issuer in ParseExpiredClaims")
	}
}

func TestParseExpiredClaims_EnforcesAudienceWhenGated(t *testing.T) {
	now := time.Now()
	tok := mintToken(t, jwt.RegisteredClaims{
		Subject:   "user-x",
		Audience:  jwt.ClaimStrings{"other-api"},
		Issuer:    jwtIssuer,
		IssuedAt:  jwt.NewNumericDate(now.Add(-1 * time.Hour)),
		NotBefore: jwt.NewNumericDate(now.Add(-1 * time.Hour)),
		ExpiresAt: jwt.NewNumericDate(now.Add(-30 * time.Minute)),
	})
	if _, err := ParseExpiredClaims(tok, testSecret, true); err == nil {
		t.Fatal("expected rejection for wrong audience when enforcement enabled")
	}
	if _, err := ParseExpiredClaims(tok, testSecret, false); err != nil {
		t.Fatalf("should accept wrong audience when enforcement off: %v", err)
	}
}
