package middleware

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ============================================================
// Context keys for authorization data
// ============================================================

const (
	MembershipKey  contextKey = "membership"
	MemberRoleKey  contextKey = "member_role"
	MemberOrgIDKey contextKey = "member_org_id"
)

// Membership represents the resolved user-org relationship for the current request.
type Membership struct {
	OrgID      string
	LocationID *string // nil for org-wide membership
	Role       string
}

// GetMembership returns the resolved membership from context (set by authz middleware).
func GetMembership(ctx context.Context) *Membership {
	v, _ := ctx.Value(MembershipKey).(*Membership)
	return v
}

// Role constants avoid magic strings throughout the codebase.
const (
	RoleClimber    = "climber"
	RoleSetter     = "setter"
	RoleHeadSetter = "head_setter"
	RoleGymManager = "gym_manager"
	RoleOrgAdmin   = "org_admin"
)

// roleRank maps roles to a numeric rank for >= comparisons.
// Higher rank = more privileges.
var roleRank = map[string]int{
	RoleClimber:    1,
	RoleSetter:     2,
	RoleHeadSetter: 3,
	RoleGymManager: 4,
	RoleOrgAdmin:   5,
}

// RoleRankValue returns the numeric rank for a role string.
// Exported for use by the web handler's view-as-role feature.
func RoleRankValue(role string) int {
	return roleRank[role]
}

// hasRole checks whether the user's role is at least as privileged as one of the required roles.
// We use the lowest-ranked required role as the threshold — if you pass {"setter", "head_setter"},
// any role >= setter qualifies.
func hasRole(userRole string, required []string) bool {
	userRank, ok := roleRank[userRole]
	if !ok {
		return false
	}
	for _, r := range required {
		if rank, ok := roleRank[r]; ok && userRank >= rank {
			return true
		}
	}
	return false
}

// ============================================================
// Authorizer holds the DB pool for membership lookups.
// ============================================================

// Authorizer provides Chi middleware that enforces org/location membership
// and role-based access control. It runs a single query per request to resolve
// the user's membership, then stores it in the request context.
type Authorizer struct {
	db *pgxpool.Pool
}

// NewAuthorizer creates an Authorizer with the given database pool.
func NewAuthorizer(db *pgxpool.Pool) *Authorizer {
	return &Authorizer{db: db}
}

// ============================================================
// Org-scoped middleware
// ============================================================

// RequireOrgMember ensures the authenticated user has any membership in the org
// identified by the {orgID} URL parameter. The resolved Membership is stored
// in context for downstream handlers.
func (a *Authorizer) RequireOrgMember(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		membership, ok := a.resolveOrgMembership(w, r)
		if !ok {
			return
		}
		ctx := context.WithValue(r.Context(), MembershipKey, membership)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireOrgRole ensures the user is a member of the org AND has one of the
// specified roles (or a higher-ranked role).
func (a *Authorizer) RequireOrgRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			membership, ok := a.resolveOrgMembership(w, r)
			if !ok {
				return
			}
			if !hasRole(membership.Role, roles) {
				jsonError(w, http.StatusForbidden, "insufficient permissions")
				return
			}
			ctx := context.WithValue(r.Context(), MembershipKey, membership)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// resolveOrgMembership looks up the user's membership for the org in the URL.
// Returns (nil, false) and writes an HTTP error if the user is not a member.
func (a *Authorizer) resolveOrgMembership(w http.ResponseWriter, r *http.Request) (*Membership, bool) {
	userID := GetUserID(r.Context())
	if userID == "" {
		jsonError(w, http.StatusUnauthorized, "authentication required")
		return nil, false
	}

	orgID := chi.URLParam(r, "orgID")
	if orgID == "" {
		jsonError(w, http.StatusBadRequest, "missing org ID")
		return nil, false
	}

	query := `
		SELECT org_id, location_id, role
		FROM user_memberships
		WHERE user_id = $1 AND org_id = $2 AND deleted_at IS NULL
		ORDER BY
			CASE role
				WHEN 'org_admin'   THEN 5
				WHEN 'gym_manager' THEN 4
				WHEN 'head_setter' THEN 3
				WHEN 'setter'      THEN 2
				WHEN 'climber'     THEN 1
				ELSE 0
			END DESC
		LIMIT 1`

	var m Membership
	err := a.db.QueryRow(r.Context(), query, userID, orgID).Scan(
		&m.OrgID, &m.LocationID, &m.Role,
	)
	if err != nil {
		// pgx returns ErrNoRows when no membership exists — treat as forbidden.
		// We intentionally say "not found" rather than "forbidden" to avoid
		// leaking whether the org exists (IDOR protection).
		jsonError(w, http.StatusNotFound, "organization not found")
		return nil, false
	}
	return &m, true
}

// ============================================================
// Location-scoped middleware
// ============================================================

// RequireLocationMember ensures the user has a membership in the org that owns
// the location identified by {locationID}. This runs a single query that joins
// locations → user_memberships.
func (a *Authorizer) RequireLocationMember(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		membership, ok := a.resolveLocationMembership(w, r)
		if !ok {
			return
		}
		ctx := context.WithValue(r.Context(), MembershipKey, membership)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireLocationRole ensures the user has a membership in the location's org
// AND one of the specified roles (or higher).
func (a *Authorizer) RequireLocationRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			membership, ok := a.resolveLocationMembership(w, r)
			if !ok {
				return
			}
			if !hasRole(membership.Role, roles) {
				jsonError(w, http.StatusForbidden, "insufficient permissions")
				return
			}
			ctx := context.WithValue(r.Context(), MembershipKey, membership)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// resolveLocationMembership resolves {locationID} → org_id, then checks the
// user's membership in that org. Single query, no round-trips.
func (a *Authorizer) resolveLocationMembership(w http.ResponseWriter, r *http.Request) (*Membership, bool) {
	userID := GetUserID(r.Context())
	if userID == "" {
		jsonError(w, http.StatusUnauthorized, "authentication required")
		return nil, false
	}

	locationID := chi.URLParam(r, "locationID")
	if locationID == "" {
		jsonError(w, http.StatusBadRequest, "missing location ID")
		return nil, false
	}

	// Join locations → user_memberships in a single query.
	// This verifies: (a) the location exists, (b) the user is a member of
	// the org that owns it. We pick the highest-privilege membership.
	query := `
		SELECT um.org_id, um.location_id, um.role
		FROM locations l
		JOIN user_memberships um
			ON um.org_id = l.org_id
			AND um.user_id = $1
			AND um.deleted_at IS NULL
		WHERE l.id = $2 AND l.deleted_at IS NULL
		ORDER BY
			CASE um.role
				WHEN 'org_admin'   THEN 5
				WHEN 'gym_manager' THEN 4
				WHEN 'head_setter' THEN 3
				WHEN 'setter'      THEN 2
				WHEN 'climber'     THEN 1
				ELSE 0
			END DESC
		LIMIT 1`

	var m Membership
	err := a.db.QueryRow(r.Context(), query, userID, locationID).Scan(
		&m.OrgID, &m.LocationID, &m.Role,
	)
	if err != nil {
		jsonError(w, http.StatusNotFound, "location not found")
		return nil, false
	}
	return &m, true
}

// ============================================================
// JSON error helper (avoids importing handler package)
// ============================================================

func jsonError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message}) //nolint:errcheck
}
