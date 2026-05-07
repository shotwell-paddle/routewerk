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

// AuthenticateCookieOrJWT accepts EITHER a valid web session cookie OR a
// valid JWT bearer token. Used by API endpoints that the SvelteKit SPA
// (cookie-authed, same-origin) and the Flutter mobile app (JWT-authed)
// both need to call. The plain Authenticate above stays JWT-only for
// endpoints exclusively used by the mobile API.
//
// Cookie is checked first because the SPA always sends one with
// credentials: 'same-origin'; if the cookie is missing or invalid we
// fall back to the bearer header so mobile callers still work.
//
// The context is populated with the same UserIDKey + EmailKey the
// Authenticate middleware uses, plus (in the cookie path) the full
// WebSession + WebUser the existing web handlers expect.
//
// View-as scope: the _rw_view_as cookie is intentionally NOT honored
// here. The downgraded role only takes effect inside SessionManager.
// RequireSession (the HTMX path), where it gates page rendering, and
// inside the SPA layout via /me/view-as which hides higher-privilege
// affordances. The JSON API enforces the user's REAL role on every
// endpoint — view-as is a UI/UX preview, not a privilege drop. A
// head_setter clicking "view as climber" still retains setter+ rights
// on /api/v1/* (which makes sense since they had those rights before
// clicking). If you want true privilege drop, build a separate
// "act as" mechanism that issues a downgraded session cookie.
func AuthenticateCookieOrJWT(sm *SessionManager, jwtSecret string, enforceAudience bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 1) Cookie path — only if a non-empty session cookie is present.
			if cookie, err := r.Cookie(SessionCookieName); err == nil && cookie.Value != "" {
				tokenHash := HashSessionToken(cookie.Value)
				ac, err := sm.sessions.GetAuthContextByTokenHash(r.Context(), tokenHash)
				if err == nil && ac != nil {
					ctx := r.Context()
					ctx = context.WithValue(ctx, WebSessionKey, ac.Session)
					ctx = context.WithValue(ctx, WebUserKey, ac.User)
					ctx = context.WithValue(ctx, UserIDKey, ac.User.ID)
					ctx = context.WithValue(ctx, EmailKey, ac.User.Email)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
				// Cookie present but invalid / expired / soft-deleted user.
				// Fall through to the bearer path — a mobile caller may
				// also be sending a JWT in the same request (unlikely but
				// not actively wrong).
			}

			// 2) Bearer-token path — same as Authenticate above.
			header := r.Header.Get("Authorization")
			if header == "" {
				http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
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
