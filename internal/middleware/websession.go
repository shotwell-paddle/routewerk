package middleware

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/rbac"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

const (
	// SessionCookieName is the cookie used for web session auth.
	SessionCookieName = "_rw_session"

	// sessionTokenBytes is the raw token length before hex encoding.
	sessionTokenBytes = 32

	// touchInterval avoids hammering the DB with last_seen updates on every
	// single request. We only touch if > 5 minutes since last touch.
	touchInterval = 5 * time.Minute
)

// Context keys for web session data.
const (
	WebSessionKey  contextKey = "web_session"
	WebUserKey     contextKey = "web_user"
	WebLocationKey contextKey = "web_location"
	WebRoleKey     contextKey = "web_role"
	WebRealRoleKey contextKey = "web_real_role" // actual role before view-as override
)

// ViewAsCookieName is the cookie used for the "view as" role override.
// Exported so the web handler can set/clear it.
const ViewAsCookieName = "_rw_view_as"

// validViewAsRoles are roles a user can downgrade to via view-as.
var validViewAsRoles = map[string]bool{
	"climber":     true,
	"setter":      true,
	"head_setter": true,
	"gym_manager": true,
}

// SessionManager handles cookie-based session lifecycle for the web frontend.
type SessionManager struct {
	sessions *repository.WebSessionRepo
	users    *repository.UserRepo
	secure   bool // Secure flag on cookie (true in production)
}

// NewSessionManager creates a SessionManager.
func NewSessionManager(sessions *repository.WebSessionRepo, users *repository.UserRepo, isDev bool) *SessionManager {
	return &SessionManager{
		sessions: sessions,
		users:    users,
		secure:   !isDev,
	}
}

// GenerateSessionToken creates a cryptographically random token and its SHA-256 hash.
// The raw token goes in the cookie; the hash is stored in the database.
func GenerateSessionToken() (token string, hash string, err error) {
	b := make([]byte, sessionTokenBytes)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	token = hex.EncodeToString(b)
	h := sha256.Sum256([]byte(token))
	hash = hex.EncodeToString(h[:])
	return token, hash, nil
}

// HashSessionToken computes the SHA-256 hash of a raw session token.
// Used when looking up a session from a cookie value.
func HashSessionToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// SetSessionCookie writes the session cookie to the response.
func (sm *SessionManager) SetSessionCookie(w http.ResponseWriter, token string, maxAge time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(maxAge.Seconds()),
		HttpOnly: true,
		Secure:   sm.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearSessionCookie expires the session cookie.
func (sm *SessionManager) ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   sm.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// RequireSession is middleware that redirects unauthenticated users to /login.
// On success it populates the context with the session, user, and membership info.
func (sm *SessionManager) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(SessionCookieName)
		if err != nil || cookie.Value == "" {
			sm.redirectToLogin(w, r)
			return
		}

		tokenHash := HashSessionToken(cookie.Value)

		session, err := sm.sessions.GetByTokenHash(r.Context(), tokenHash)
		if err != nil {
			slog.Error("session lookup failed", "error", err)
			sm.redirectToLogin(w, r)
			return
		}
		if session == nil {
			// Invalid or expired session — clear the stale cookie
			sm.ClearSessionCookie(w)
			sm.redirectToLogin(w, r)
			return
		}

		// Touch last_seen_at periodically (not on every request)
		if time.Since(session.LastSeenAt) > touchInterval {
			go func(id string) {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				if err := sm.sessions.TouchLastSeen(ctx, id); err != nil {
					slog.Error("failed to touch session", "session_id", id, "error", err)
				}
			}(session.ID)
		}

		// Load the user
		user, err := sm.users.GetByID(r.Context(), session.UserID)
		if err != nil || user == nil {
			slog.Error("session user not found", "user_id", session.UserID, "error", err)
			sm.ClearSessionCookie(w)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// Load the user's highest role for the session's location (or any membership)
		memberships, err := sm.users.GetMemberships(r.Context(), user.ID)
		if err != nil {
			slog.Error("failed to load memberships", "user_id", user.ID, "error", err)
		}

		role := bestRole(memberships, session.LocationID)

		// Check for view-as role override cookie. This allows admins to
		// experience the app as a lower-privileged role for testing/demos.
		effectiveRole := applyViewAsOverride(r, role)

		// Inject into context
		ctx := r.Context()
		ctx = context.WithValue(ctx, WebSessionKey, session)
		ctx = context.WithValue(ctx, WebUserKey, user)
		ctx = context.WithValue(ctx, UserIDKey, user.ID)     // reuse existing key
		ctx = context.WithValue(ctx, EmailKey, user.Email)    // reuse existing key
		ctx = context.WithValue(ctx, WebLocationKey, session.LocationID)
		ctx = context.WithValue(ctx, WebRoleKey, effectiveRole)
		ctx = context.WithValue(ctx, WebRealRoleKey, role) // always store actual role

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// redirectToLogin sends the user to /login. For HTMX partial requests it uses
// the HX-Redirect header so HTMX does a full-page navigation instead of
// swapping the login page HTML into the content container.
func (sm *SessionManager) redirectToLogin(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/login")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// OptionalSession is like RequireSession but does not redirect — it just
// populates context if a valid session exists. Useful for public pages that
// show different content for logged-in vs anonymous users.
func (sm *SessionManager) OptionalSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(SessionCookieName)
		if err != nil || cookie.Value == "" {
			next.ServeHTTP(w, r)
			return
		}

		tokenHash := HashSessionToken(cookie.Value)
		session, err := sm.sessions.GetByTokenHash(r.Context(), tokenHash)
		if err != nil || session == nil {
			next.ServeHTTP(w, r)
			return
		}

		user, err := sm.users.GetByID(r.Context(), session.UserID)
		if err != nil || user == nil {
			next.ServeHTTP(w, r)
			return
		}

		memberships, _ := sm.users.GetMemberships(r.Context(), user.ID)
		role := bestRole(memberships, session.LocationID)
		effectiveRole := applyViewAsOverride(r, role)

		ctx := r.Context()
		ctx = context.WithValue(ctx, WebSessionKey, session)
		ctx = context.WithValue(ctx, WebUserKey, user)
		ctx = context.WithValue(ctx, UserIDKey, user.ID)
		ctx = context.WithValue(ctx, EmailKey, user.Email)
		ctx = context.WithValue(ctx, WebLocationKey, session.LocationID)
		ctx = context.WithValue(ctx, WebRoleKey, effectiveRole)
		ctx = context.WithValue(ctx, WebRealRoleKey, role)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireSetterSession is middleware that gates routes to setter-level roles
// (setter, head_setter, gym_manager, org_admin). Must be applied after
// RequireSession so that WebRoleKey is already in context.
func RequireSetterSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role := GetWebRole(r.Context())
		if !rbac.IsAtLeast(role, rbac.RoleSetter) {
			if r.Header.Get("HX-Request") == "true" {
				w.Header().Set("HX-Redirect", "/routes")
				w.WriteHeader(http.StatusForbidden)
				return
			}
			http.Redirect(w, r, "/routes", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAppAdmin is middleware that gates routes to users with the is_app_admin
// flag set on their user record. Must be applied after RequireSession so that
// WebUserKey is already in context.
func RequireAppAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := GetWebUser(r.Context())
		if user == nil || !user.IsAppAdmin {
			if r.Header.Get("HX-Request") == "true" {
				w.Header().Set("HX-Redirect", "/routes")
				w.WriteHeader(http.StatusForbidden)
				return
			}
			http.Redirect(w, r, "/routes", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// GetWebUser extracts the authenticated user from context.
func GetWebUser(ctx context.Context) *model.User {
	v, _ := ctx.Value(WebUserKey).(*model.User)
	return v
}

// GetWebSession extracts the web session from context.
func GetWebSession(ctx context.Context) *model.WebSession {
	v, _ := ctx.Value(WebSessionKey).(*model.WebSession)
	return v
}

// GetWebLocationID extracts the active location ID from context.
func GetWebLocationID(ctx context.Context) string {
	v, _ := ctx.Value(WebLocationKey).(*string)
	if v == nil {
		return ""
	}
	return *v
}

// GetWebRole extracts the user's resolved role from context.
func GetWebRole(ctx context.Context) string {
	v, _ := ctx.Value(WebRoleKey).(string)
	return v
}

// GetWebRealRole extracts the user's actual role (before view-as override) from context.
func GetWebRealRole(ctx context.Context) string {
	v, _ := ctx.Value(WebRealRoleKey).(string)
	if v == "" {
		// Fallback: if no real role stored, the effective role IS the real role
		return GetWebRole(ctx)
	}
	return v
}

// applyViewAsOverride checks for a view-as cookie and downgrades the role
// if the cookie is valid and the target role is below the user's actual role.
func applyViewAsOverride(r *http.Request, realRole string) string {
	cookie, err := r.Cookie(ViewAsCookieName)
	if err != nil || cookie.Value == "" {
		return realRole
	}
	target := cookie.Value
	if !validViewAsRoles[target] {
		return realRole
	}
	// Only allow downgrade: target rank must be strictly less than real rank
	if RoleRankValue(target) < RoleRankValue(realRole) {
		return target
	}
	return realRole
}

// bestRole finds the user's highest role scoped to the given location.
// If locationID is nil (no location selected), it falls back to the highest
// role across all memberships. Otherwise, only memberships at that location
// (or org-wide memberships with no specific location) are considered.
// Uses the roleRank map defined in authz.go.
func bestRole(memberships []model.UserMembership, locationID *string) string {
	best := "climber"
	bestRank := 0

	for _, m := range memberships {
		// If we have a location context, only consider memberships at that
		// location or org-wide memberships (location_id is null).
		if locationID != nil && m.LocationID != nil && *m.LocationID != *locationID {
			continue
		}

		rank := rbac.RankValue(m.Role)
		if rank == 0 {
			continue
		}
		if rank > bestRank {
			best = m.Role
			bestRank = rank
		}
	}
	return best
}
