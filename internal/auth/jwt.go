package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// Issuer/audience constants for JWT validation. The audience claim
// distinguishes tokens minted for this API from tokens minted for any
// future service that might share JWT_SECRET (e.g. a separate admin
// surface). Always written on new tokens; enforcement on the read path
// is gated by Config.EnforceJWTAudience to allow a staged rollout.
const (
	jwtIssuer   = "routewerk"
	jwtAudience = "routewerk-api"
)

type Claims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// HashPassword hashes a plaintext password using bcrypt.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

// CheckPassword compares a plaintext password against a bcrypt hash.
func CheckPassword(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// GenerateAccessToken creates a signed JWT with the user's ID and email.
// The token includes Subject, Audience, NotBefore, and Issuer claims so
// validators can enforce them on the read path.
func GenerateAccessToken(userID, email, secret string, expiry time.Duration) (string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(expiry)

	claims := Claims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			Audience:  jwt.ClaimStrings{jwtAudience},
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    jwtIssuer,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign token: %w", err)
	}

	return signed, expiresAt, nil
}

// ValidateAccessToken parses and validates a JWT, returning the claims.
// Issuer and signing method (HS256 only) are always enforced. The audience
// claim is additionally enforced when enforceAudience is true; this is gated
// so we can stage the rollout while old tokens without the audience claim
// drain from the wild.
func ValidateAccessToken(tokenStr, secret string, enforceAudience bool) (*Claims, error) {
	opts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithIssuer(jwtIssuer),
		jwt.WithExpirationRequired(),
	}
	if enforceAudience {
		opts = append(opts, jwt.WithAudience(jwtAudience))
	}

	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	}, opts...)
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}

// ParseExpiredClaims extracts claims from a JWT that may have expired.
// The HMAC signature IS verified (the keyfunc always runs) and the signing
// method is restricted to HS256. Only the registered-claims time check
// (exp, nbf, iat) is skipped so that an expired access token can still
// identify the user for a refresh request. Issuer is checked manually
// post-parse because jwt.WithoutClaimsValidation also disables the issuer
// check. Audience is similarly checked manually when enforceAudience is true.
func ParseExpiredClaims(tokenStr, secret string, enforceAudience bool) (*Claims, error) {
	// Step 1: Parse with full signature verification but skip claims validation
	// so that an expired token doesn't cause an error.
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired(), jwt.WithoutClaimsValidation())
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	// Step 2: Verify the issuer matches to prevent cross-service token reuse.
	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}
	if claims.Issuer != jwtIssuer {
		return nil, fmt.Errorf("invalid token issuer")
	}
	if claims.UserID == "" {
		return nil, fmt.Errorf("missing user_id in token")
	}
	if enforceAudience {
		hasAud := false
		for _, a := range claims.Audience {
			if a == jwtAudience {
				hasAud = true
				break
			}
		}
		if !hasAud {
			return nil, fmt.Errorf("invalid token audience")
		}
	}

	return claims, nil
}

// GenerateRefreshToken creates a cryptographically random refresh token.
func GenerateRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate refresh token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// HashRefreshToken returns a keyed HMAC-SHA256 of the plaintext refresh
// token, hex-encoded. Because it's deterministic (same input ⇒ same output),
// the storage layer can look up a token by hash equality — O(1) indexed —
// instead of bcrypting against every active token for the user.
//
// The secret MUST be JWT_SECRET (or another high-entropy server secret). If
// an attacker steals the database but not the secret, the hashes remain
// useless: without the key they can't reproduce a hash from a candidate
// token, and constant-time comparison below prevents timing oracles.
func HashRefreshToken(token, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(token))
	return hex.EncodeToString(h.Sum(nil))
}

// CheckRefreshToken compares a plaintext refresh token against its stored
// HMAC hash using a constant-time comparison. Returns true on match.
func CheckRefreshToken(token, hash, secret string) bool {
	expected := HashRefreshToken(token, secret)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(hash)) == 1
}

// CheckRefreshTokenBcrypt compares a plaintext refresh token against a legacy
// bcrypt hash. Retained only for the dual-scheme migration window — once all
// bcrypt-scheme rows have aged out past REFRESH_TOKEN_EXPIRY, this and the
// fallback code path in service.AuthService.Refresh can be deleted.
func CheckRefreshTokenBcrypt(token, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(token)) == nil
}
