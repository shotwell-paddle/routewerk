package handler

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

// TeamHandler powers the SPA team-management surface (Phase 2.7).
//
// The HTMX side has had similar functionality at /settings/team since
// the early HTMX days; this handler exposes the same operations as JSON
// so the SPA at /app/team can drive them without reaching for cookie-auth
// HTMX endpoints.
type TeamHandler struct {
	users *repository.UserRepo
}

func NewTeamHandler(users *repository.UserRepo) *TeamHandler {
	return &TeamHandler{users: users}
}

// teamMember is the response shape — matches repository.LocationMember
// plus the membership_id field that the SPA uses as a stable key.
type teamMember struct {
	MembershipID string `json:"membership_id"`
	UserID       string `json:"user_id"`
	DisplayName  string `json:"display_name"`
	Email        string `json:"email"`
	Role         string `json:"role"`
}

// List — GET /locations/{locationID}/team. Returns the location's team:
// every user with a membership at the location (or org-wide membership at
// the location's org). Caller must be head_setter+ at the location
// (enforced by router middleware).
func (h *TeamHandler) List(w http.ResponseWriter, r *http.Request) {
	locationID := chi.URLParam(r, "locationID")
	q := r.URL.Query()

	limit := 200 // generous; the HTMX page paginates at 50 with search
	if v := q.Get("limit"); v != "" {
		var n int
		_, err := fmt.Sscanf(v, "%d", &n)
		if err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}

	res, err := h.users.SearchMembersByLocation(r.Context(), locationID, repository.MemberSearchParams{
		Query:      q.Get("q"),
		RoleFilter: q.Get("role"),
		Limit:      limit,
		Offset:     0,
	})
	if err != nil {
		Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := make([]teamMember, len(res.Members))
	for i, m := range res.Members {
		out[i] = teamMember{
			MembershipID: m.MembershipID,
			UserID:       m.UserID,
			DisplayName:  m.DisplayName,
			Email:        m.Email,
			Role:         m.Role,
		}
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"members":     out,
		"total_count": res.TotalCount,
	})
}

type updateMembershipRequest struct {
	Role string `json:"role"`
}

// UpdateMembership — PATCH /memberships/{membershipID}. Changes a member's
// role. The route is mounted under an authorizer that requires gym_manager+
// at the membership's org (the only authoritative scope for cross-location
// admins); we additionally enforce here that head_setter is the highest
// role a head_setter can assign — gym_manager and above can assign anything
// up to gym_manager.
func (h *TeamHandler) UpdateMembership(w http.ResponseWriter, r *http.Request) {
	membershipID := chi.URLParam(r, "membershipID")
	callerRole := callerRoleForMembership(r, h.users, membershipID)

	var req updateMembershipRequest
	if err := Decode(r, &req); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	allowed := allowedRolesForGrantor(callerRole)
	if !allowed[req.Role] {
		Error(w, http.StatusForbidden, "you cannot assign that role")
		return
	}

	if err := h.users.UpdateMemberRole(r.Context(), membershipID, req.Role); err != nil {
		Error(w, http.StatusInternalServerError, "update failed")
		return
	}

	m, err := h.users.GetMembershipByID(r.Context(), membershipID)
	if err != nil || m == nil {
		Error(w, http.StatusNotFound, "membership not found")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{"membership": m})
}

// RemoveMembership — DELETE /memberships/{membershipID}. Soft-deletes the
// membership. Same role gating as UpdateMembership.
func (h *TeamHandler) RemoveMembership(w http.ResponseWriter, r *http.Request) {
	membershipID := chi.URLParam(r, "membershipID")
	callerRole := callerRoleForMembership(r, h.users, membershipID)
	if middleware.RoleRankValue(callerRole) < 4 {
		Error(w, http.StatusForbidden, "gym_manager or above required")
		return
	}

	if err := h.users.RemoveMembership(r.Context(), membershipID); err != nil {
		Error(w, http.StatusInternalServerError, "remove failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// callerRoleForMembership returns the caller's effective role *at the org
// owning the target membership*. Used to gate role changes / removal.
func callerRoleForMembership(r *http.Request, users *repository.UserRepo, membershipID string) string {
	m, err := users.GetMembershipByID(r.Context(), membershipID)
	if err != nil || m == nil {
		return ""
	}
	callerID := middleware.GetUserID(r.Context())
	memberships, err := users.GetMemberships(r.Context(), callerID)
	if err != nil {
		return ""
	}
	// Highest-rank role the caller has anywhere in this org.
	best := ""
	bestRank := 0
	for _, mm := range memberships {
		if mm.OrgID != m.OrgID {
			continue
		}
		if rank := middleware.RoleRankValue(mm.Role); rank > bestRank {
			best = mm.Role
			bestRank = rank
		}
	}
	return best
}

// allowedRolesForGrantor returns the role set a caller of the given role
// is allowed to assign. Mirrors the HTMX rules in
// internal/handler/web/org_settings.go.
func allowedRolesForGrantor(callerRole string) map[string]bool {
	rank := middleware.RoleRankValue(callerRole)
	if rank >= 5 {
		return map[string]bool{"climber": true, "setter": true, "head_setter": true, "gym_manager": true, "org_admin": true}
	}
	if rank >= 4 {
		return map[string]bool{"climber": true, "setter": true, "head_setter": true, "gym_manager": true}
	}
	if rank >= 3 {
		return map[string]bool{"climber": true, "setter": true}
	}
	return map[string]bool{}
}

