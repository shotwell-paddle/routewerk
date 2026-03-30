package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shotwell-paddle/routewerk/internal/model"
)

// ── RoleRankValue ───────────────────────────────────────────

func TestRoleRankValue(t *testing.T) {
	tests := []struct {
		role string
		rank int
	}{
		{RoleClimber, 1},
		{RoleSetter, 2},
		{RoleHeadSetter, 3},
		{RoleGymManager, 4},
		{RoleOrgAdmin, 5},
		{"unknown", 0},
		{"", 0},
	}

	for _, tc := range tests {
		got := RoleRankValue(tc.role)
		if got != tc.rank {
			t.Errorf("RoleRankValue(%q) = %d, want %d", tc.role, got, tc.rank)
		}
	}
}

func TestRoleRankValue_StrictHierarchy(t *testing.T) {
	roles := []string{RoleClimber, RoleSetter, RoleHeadSetter, RoleGymManager, RoleOrgAdmin}
	for i := 1; i < len(roles); i++ {
		if RoleRankValue(roles[i]) <= RoleRankValue(roles[i-1]) {
			t.Errorf("expected %q (%d) > %q (%d)", roles[i], RoleRankValue(roles[i]), roles[i-1], RoleRankValue(roles[i-1]))
		}
	}
}

// ── Role constants ──────────────────────────────────────────

func TestRoleConstants_Values(t *testing.T) {
	if RoleClimber != "climber" {
		t.Errorf("RoleClimber = %q, want %q", RoleClimber, "climber")
	}
	if RoleSetter != "setter" {
		t.Errorf("RoleSetter = %q, want %q", RoleSetter, "setter")
	}
	if RoleHeadSetter != "head_setter" {
		t.Errorf("RoleHeadSetter = %q, want %q", RoleHeadSetter, "head_setter")
	}
	if RoleGymManager != "gym_manager" {
		t.Errorf("RoleGymManager = %q, want %q", RoleGymManager, "gym_manager")
	}
	if RoleOrgAdmin != "org_admin" {
		t.Errorf("RoleOrgAdmin = %q, want %q", RoleOrgAdmin, "org_admin")
	}
}

// ── hasRole comprehensive ───────────────────────────────────

func TestHasRole_AllRoleCombinations(t *testing.T) {
	roles := []string{RoleClimber, RoleSetter, RoleHeadSetter, RoleGymManager, RoleOrgAdmin}

	for _, userRole := range roles {
		for _, requiredRole := range roles {
			want := RoleRankValue(userRole) >= RoleRankValue(requiredRole)
			got := hasRole(userRole, []string{requiredRole})
			if got != want {
				t.Errorf("hasRole(%q, [%q]) = %v, want %v", userRole, requiredRole, got, want)
			}
		}
	}
}

func TestHasRole_EmptyRequired(t *testing.T) {
	// No required roles means nobody can access
	if hasRole("org_admin", []string{}) {
		t.Error("hasRole with empty required should return false")
	}
}

func TestHasRole_UnknownUserRole(t *testing.T) {
	if hasRole("super_admin", []string{"climber"}) {
		t.Error("unknown user role should return false")
	}
}

func TestHasRole_UnknownRequiredRole(t *testing.T) {
	// Unknown required role should be skipped
	if hasRole("org_admin", []string{"super_admin"}) {
		t.Error("unknown required role should not match")
	}
}

func TestHasRole_MixedValidAndInvalidRequired(t *testing.T) {
	// If one valid required role matches, should return true
	if !hasRole("setter", []string{"unknown_role", "setter"}) {
		t.Error("should match the valid required role")
	}
}

// ── bestRole ────────────────────────────────────────────────

func TestBestRole_WithLocationFilter(t *testing.T) {
	locID := "loc-123"
	otherLoc := "loc-456"

	memberships := []model.UserMembership{
		{Role: "org_admin", LocationID: &otherLoc},
		{Role: "setter", LocationID: &locID},
		{Role: "climber", LocationID: &locID},
	}

	got := bestRole(memberships, &locID)
	if got != "setter" {
		t.Errorf("bestRole with location filter = %q, want %q", got, "setter")
	}
}

func TestBestRole_OrgScopedMembership(t *testing.T) {
	// Org-scoped membership (locationID = nil) should match any location
	locID := "loc-123"
	memberships := []model.UserMembership{
		{Role: "gym_manager", LocationID: nil}, // org-scoped
		{Role: "setter", LocationID: &locID},
	}

	got := bestRole(memberships, &locID)
	if got != "gym_manager" {
		t.Errorf("bestRole should include org-scoped memberships, got %q", got)
	}
}

func TestBestRole_NilLocationFilter(t *testing.T) {
	locID := "loc-123"
	memberships := []model.UserMembership{
		{Role: "head_setter", LocationID: &locID},
		{Role: "setter", LocationID: &locID},
	}

	got := bestRole(memberships, nil)
	if got != "head_setter" {
		t.Errorf("bestRole with nil filter = %q, want %q", got, "head_setter")
	}
}

// ── RequireSetterSession ────────────────────────────────────

func TestRequireSetterSession_AllowsSetter(t *testing.T) {
	handler := RequireSetterSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	ctx := context.WithValue(req.Context(), WebRoleKey, "setter")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("setter should be allowed, got %d", rec.Code)
	}
}

func TestRequireSetterSession_AllowsHeadSetter(t *testing.T) {
	handler := RequireSetterSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	ctx := context.WithValue(req.Context(), WebRoleKey, "head_setter")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("head_setter should be allowed, got %d", rec.Code)
	}
}

func TestRequireSetterSession_AllowsOrgAdmin(t *testing.T) {
	handler := RequireSetterSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	ctx := context.WithValue(req.Context(), WebRoleKey, "org_admin")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("org_admin should be allowed, got %d", rec.Code)
	}
}

func TestRequireSetterSession_BlocksClimber(t *testing.T) {
	handler := RequireSetterSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for climber")
	}))

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	ctx := context.WithValue(req.Context(), WebRoleKey, "climber")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Non-HTMX requests get redirected (303 See Other), not 403
	if rec.Code != http.StatusSeeOther {
		t.Errorf("climber should be redirected, got %d", rec.Code)
	}
}

func TestRequireSetterSession_BlocksClimber_HTMX(t *testing.T) {
	handler := RequireSetterSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for climber")
	}))

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req.Header.Set("HX-Request", "true")
	ctx := context.WithValue(req.Context(), WebRoleKey, "climber")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("HTMX climber request should get 403, got %d", rec.Code)
	}
}

func TestRequireSetterSession_BlocksEmptyRole(t *testing.T) {
	handler := RequireSetterSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called with empty role")
	}))

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Non-HTMX requests get redirected (303 See Other)
	if rec.Code != http.StatusSeeOther {
		t.Errorf("empty role should be redirected, got %d", rec.Code)
	}
}

// ── Membership context ──────────────────────────────────────

func TestGetMembership_Roundtrip(t *testing.T) {
	m := &Membership{
		OrgID: "org-abc",
		Role:  "setter",
	}
	ctx := context.WithValue(context.Background(), MembershipKey, m)

	got := GetMembership(ctx)
	if got == nil {
		t.Fatal("GetMembership should return non-nil")
	}
	if got.OrgID != "org-abc" {
		t.Errorf("OrgID = %q, want %q", got.OrgID, "org-abc")
	}
	if got.Role != "setter" {
		t.Errorf("Role = %q, want %q", got.Role, "setter")
	}
}

// ── Testing helpers roundtrip ───────────────────────────────

func TestSetWebUser_Roundtrip(t *testing.T) {
	user := &model.User{ID: "user-test", DisplayName: "Test User"}
	ctx := SetWebUser(context.Background(), user)

	got := GetWebUser(ctx)
	if got == nil || got.ID != "user-test" {
		t.Error("SetWebUser/GetWebUser roundtrip failed")
	}
}

func TestSetWebRole_Roundtrip(t *testing.T) {
	ctx := SetWebRole(context.Background(), "setter")
	got := GetWebRole(ctx)
	if got != "setter" {
		t.Errorf("SetWebRole/GetWebRole = %q, want %q", got, "setter")
	}
}

func TestSetWebRealRole_Roundtrip(t *testing.T) {
	ctx := SetWebRealRole(context.Background(), "org_admin")
	got := GetWebRealRole(ctx)
	if got != "org_admin" {
		t.Errorf("SetWebRealRole/GetWebRealRole = %q, want %q", got, "org_admin")
	}
}

func TestSetWebLocationID_Roundtrip(t *testing.T) {
	ctx := SetWebLocationID(context.Background(), "loc-123")
	got := GetWebLocationID(ctx)
	if got != "loc-123" {
		t.Errorf("SetWebLocationID/GetWebLocationID = %q, want %q", got, "loc-123")
	}
}

func TestSetWebSession_Roundtrip(t *testing.T) {
	sess := &model.WebSession{ID: "sess-test", UserID: "user-test"}
	ctx := SetWebSession(context.Background(), sess)

	got := GetWebSession(ctx)
	if got == nil || got.ID != "sess-test" {
		t.Error("SetWebSession/GetWebSession roundtrip failed")
	}
}
