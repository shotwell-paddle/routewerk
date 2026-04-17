package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/shotwell-paddle/routewerk/internal/auth"
)

type contextKey string

const (
	UserIDKey contextKey = "user_id"
	EmailKey  contextKey = "email"
)

// Authenticate validates the JWT from the Authorization header and injects
// the user's ID and email into the request context. enforceAudience gates
// the audience claim check so the stricter rule can be enabled in staging
// before production (see Config.EnforceJWTAudience).
func Authenticate(jwtSecret string, enforceAudience bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if header == "" {
				http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
				return
			}

			parts := strings.SplitN(header, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
				http.Error(w, `{"error":"invalid authorization format"}`, http.StatusUnauthorized)
				return
			}

			claims, err := auth.ValidateAccessToken(parts[1], jwtSecret, enforceAudience)
			if err != nil {
				http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
			ctx = context.WithValue(ctx, EmailKey, claims.Email)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AuthenticateAllowExpired is like Authenticate but accepts expired tokens.
// It still validates the signature — only the expiry check is skipped.
// Used exclusively for the refresh token endpoint so users can refresh after
// their access token has expired (which is the whole point of refresh tokens).
func AuthenticateAllowExpired(jwtSecret string, enforceAudience bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if header == "" {
				http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
				return
			}

			parts := strings.SplitN(header, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
				http.Error(w, `{"error":"invalid authorization format"}`, http.StatusUnauthorized)
				return
			}

			claims, err := auth.ParseExpiredClaims(parts[1], jwtSecret, enforceAudience)
			if err != nil {
				http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
			ctx = context.WithValue(ctx, EmailKey, claims.Email)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetUserID extracts the authenticated user's ID from the request context.
func GetUserID(ctx context.Context) string {
	v, _ := ctx.Value(UserIDKey).(string)
	return v
}

// GetEmail extracts the authenticated user's email from the request context.
func GetEmail(ctx context.Context) string {
	v, _ := ctx.Value(EmailKey).(string)
	return v
}
