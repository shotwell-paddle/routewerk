package rbac

import "testing"

func TestRankValue(t *testing.T) {
	tests := []struct {
		role string
		want int
	}{
		{RoleClimber, 1},
		{RoleSetter, 2},
		{RoleHeadSetter, 3},
		{RoleGymManager, 4},
		{RoleOrgAdmin, 5},
		{"unknown", 0},
	}
	for _, tc := range tests {
		if got := RankValue(tc.role); got != tc.want {
			t.Errorf("RankValue(%q) = %d, want %d", tc.role, got, tc.want)
		}
	}
}

func TestIsAtLeast(t *testing.T) {
	tests := []struct {
		user, required string
		want           bool
	}{
		{RoleOrgAdmin, RoleClimber, true},
		{RoleSetter, RoleSetter, true},
		{RoleClimber, RoleSetter, false},
		{RoleHeadSetter, RoleGymManager, false},
		{"unknown", RoleClimber, false},
	}
	for _, tc := range tests {
		if got := IsAtLeast(tc.user, tc.required); got != tc.want {
			t.Errorf("IsAtLeast(%q, %q) = %v, want %v", tc.user, tc.required, got, tc.want)
		}
	}
}

func TestHasAnyRole(t *testing.T) {
	if !HasAnyRole(RoleSetter, RoleSetter, RoleHeadSetter) {
		t.Error("setter should match setter requirement")
	}
	if !HasAnyRole(RoleOrgAdmin, RoleSetter) {
		t.Error("org_admin should satisfy setter requirement")
	}
	if HasAnyRole(RoleClimber, RoleSetter) {
		t.Error("climber should not satisfy setter requirement")
	}
	if HasAnyRole("invalid") {
		t.Error("unknown role should not match anything")
	}
}

func TestValidRole(t *testing.T) {
	if !ValidRole(RoleClimber) {
		t.Error("climber should be valid")
	}
	if ValidRole("superadmin") {
		t.Error("superadmin should not be valid")
	}
}

func TestCan(t *testing.T) {
	tests := []struct {
		role string
		perm Permission
		want bool
	}{
		// Climber can view routes but not create
		{RoleClimber, PermRouteView, true},
		{RoleClimber, PermRouteCreate, false},

		// Setter can create routes but not bulk archive
		{RoleSetter, PermRouteCreate, true},
		{RoleSetter, PermRouteBulkArchive, false},

		// Head setter can bulk archive and manage sessions
		{RoleHeadSetter, PermRouteBulkArchive, true},
		{RoleHeadSetter, PermSessionCreate, true},
		{RoleHeadSetter, PermOrgEdit, false},

		// Org admin can do everything
		{RoleOrgAdmin, PermOrgEdit, true},
		{RoleOrgAdmin, PermRouteView, true},
		{RoleOrgAdmin, PermSessionAssign, true},

		// Unknown permission
		{RoleOrgAdmin, "unknown.perm", false},
	}
	for _, tc := range tests {
		if got := Can(tc.role, tc.perm); got != tc.want {
			t.Errorf("Can(%q, %q) = %v, want %v", tc.role, tc.perm, got, tc.want)
		}
	}
}

func TestAllPermissions(t *testing.T) {
	climberPerms := AllPermissions(RoleClimber)
	adminPerms := AllPermissions(RoleOrgAdmin)

	if len(climberPerms) >= len(adminPerms) {
		t.Errorf("climber (%d perms) should have fewer than admin (%d perms)",
			len(climberPerms), len(adminPerms))
	}

	if len(adminPerms) != len(minRole) {
		t.Errorf("org_admin should have all %d permissions, got %d",
			len(minRole), len(adminPerms))
	}
}

func TestRoleHierarchy(t *testing.T) {
	// Each role should have at least as many permissions as the role below it.
	// Note: gym_manager and head_setter currently have the same permissions
	// because gym_manager doesn't have unique permissions yet — it just
	// outranks head_setter in the role hierarchy.
	roles := []string{RoleClimber, RoleSetter, RoleHeadSetter, RoleGymManager, RoleOrgAdmin}
	for i := 1; i < len(roles); i++ {
		lower := len(AllPermissions(roles[i-1]))
		higher := len(AllPermissions(roles[i]))
		if higher < lower {
			t.Errorf("%s (%d perms) should have >= %s (%d perms)",
				roles[i], higher, roles[i-1], lower)
		}
	}

	// Org admin must have strictly more than head setter
	admin := len(AllPermissions(RoleOrgAdmin))
	headSetter := len(AllPermissions(RoleHeadSetter))
	if admin <= headSetter {
		t.Errorf("org_admin (%d) should have more perms than head_setter (%d)", admin, headSetter)
	}
}
