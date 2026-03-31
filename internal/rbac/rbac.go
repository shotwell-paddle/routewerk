// Package rbac provides a centralized permission matrix for role-based
// access control. Instead of scattering role checks across middleware and
// handlers, all permission decisions flow through this package.
//
// Role hierarchy (lowest → highest):
//
//	climber → setter → head_setter → gym_manager → org_admin
//
// Higher roles inherit all permissions of lower roles.
package rbac

// ── Roles ───────────────────────────────────────────────────────

const (
	RoleClimber    = "climber"
	RoleSetter     = "setter"
	RoleHeadSetter = "head_setter"
	RoleGymManager = "gym_manager"
	RoleOrgAdmin   = "org_admin"
)

// Rank maps roles to a numeric value for >= comparisons.
var Rank = map[string]int{
	RoleClimber:    1,
	RoleSetter:     2,
	RoleHeadSetter: 3,
	RoleGymManager: 4,
	RoleOrgAdmin:   5,
}

// RankValue returns the numeric rank for a role string, or 0 if unknown.
func RankValue(role string) int {
	return Rank[role]
}

// IsAtLeast returns true if userRole is at least as privileged as requiredRole.
func IsAtLeast(userRole, requiredRole string) bool {
	return Rank[userRole] >= Rank[requiredRole]
}

// HasAnyRole returns true if userRole matches any of the required roles
// (or a higher-ranked role). Uses the lowest required role as threshold.
func HasAnyRole(userRole string, required ...string) bool {
	userRank, ok := Rank[userRole]
	if !ok {
		return false
	}
	for _, r := range required {
		if rank, ok := Rank[r]; ok && userRank >= rank {
			return true
		}
	}
	return false
}

// ValidRole returns true if the role string is recognized.
func ValidRole(role string) bool {
	_, ok := Rank[role]
	return ok
}

// ── Permission Matrix ───────────────────────────────────────────

// Permission identifies an action in the system.
type Permission string

const (
	// Route management
	PermRouteView          Permission = "route.view"
	PermRouteCreate        Permission = "route.create"
	PermRouteEdit          Permission = "route.edit"
	PermRouteChangeStatus  Permission = "route.change_status"
	PermRouteBulkArchive   Permission = "route.bulk_archive"

	// Wall management
	PermWallView   Permission = "wall.view"
	PermWallCreate Permission = "wall.create"
	PermWallEdit   Permission = "wall.edit"
	PermWallDelete Permission = "wall.delete"

	// Setting sessions
	PermSessionView   Permission = "session.view"
	PermSessionCreate Permission = "session.create"
	PermSessionEdit   Permission = "session.edit"
	PermSessionAssign Permission = "session.assign"

	// Climber actions
	PermAscentLog  Permission = "ascent.log"
	PermRouteRate  Permission = "route.rate"
	PermPhotoUpload Permission = "photo.upload"

	// Gym settings
	PermSettingsView Permission = "settings.view"
	PermSettingsEdit Permission = "settings.edit"

	// Organization
	PermOrgView       Permission = "org.view"
	PermOrgEdit       Permission = "org.edit"
	PermLocationCreate Permission = "location.create"

	// Analytics
	PermAnalyticsView Permission = "analytics.view"

	// Team management
	PermTeamView Permission = "team.view"
	PermTeamEdit Permission = "team.edit"

	// Labor
	PermLaborLog  Permission = "labor.log"
	PermLaborView Permission = "labor.view"

	// Tags
	PermTagView   Permission = "tag.view"
	PermTagManage Permission = "tag.manage"
)

// minRole maps each permission to the minimum role required.
var minRole = map[Permission]string{
	// Any authenticated user (climber+)
	PermRouteView:   RoleClimber,
	PermWallView:    RoleClimber,
	PermAscentLog:   RoleClimber,
	PermRouteRate:   RoleClimber,
	PermPhotoUpload: RoleClimber,
	PermOrgView:     RoleClimber,
	PermTagView:     RoleClimber,

	// Setter+
	PermRouteCreate:       RoleSetter,
	PermRouteEdit:         RoleSetter,
	PermRouteChangeStatus: RoleSetter,
	PermWallCreate:        RoleSetter,
	PermWallEdit:          RoleSetter,
	PermSessionView:       RoleSetter,
	PermSettingsView:      RoleSetter,
	PermLaborLog:          RoleSetter,
	PermTagManage:         RoleSetter,

	// Head setter+
	PermRouteBulkArchive: RoleHeadSetter,
	PermWallDelete:       RoleHeadSetter,
	PermSessionCreate:    RoleHeadSetter,
	PermSessionEdit:      RoleHeadSetter,
	PermSessionAssign:    RoleHeadSetter,
	PermSettingsEdit:     RoleHeadSetter,
	PermAnalyticsView:    RoleHeadSetter,
	PermTeamView:         RoleHeadSetter,
	PermTeamEdit:         RoleHeadSetter,
	PermLaborView:        RoleHeadSetter,

	// Org admin
	PermOrgEdit:         RoleOrgAdmin,
	PermLocationCreate:  RoleOrgAdmin,
}

// Can returns true if the given role has the specified permission.
func Can(role string, perm Permission) bool {
	required, ok := minRole[perm]
	if !ok {
		return false // unknown permission = deny
	}
	return IsAtLeast(role, required)
}

// AllPermissions returns all permissions available to a role.
func AllPermissions(role string) []Permission {
	var perms []Permission
	for perm, req := range minRole {
		if IsAtLeast(role, req) {
			perms = append(perms, perm)
		}
	}
	return perms
}
