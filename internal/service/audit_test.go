package service

import "testing"

// ── Audit Constants ────────────────────────────────────────────────
// These tests verify that audit action constants are defined and follow
// the expected naming convention. This catches typos and ensures
// consistency across the codebase.

func TestAuditConstants_NonEmpty(t *testing.T) {
	constants := map[string]string{
		"AuditOrgUpdate":         AuditOrgUpdate,
		"AuditLocationCreate":    AuditLocationCreate,
		"AuditLocationUpdate":    AuditLocationUpdate,
		"AuditWallCreate":        AuditWallCreate,
		"AuditWallUpdate":        AuditWallUpdate,
		"AuditWallDelete":        AuditWallDelete,
		"AuditRouteCreate":       AuditRouteCreate,
		"AuditRouteUpdate":       AuditRouteUpdate,
		"AuditRouteStatusChange": AuditRouteStatusChange,
		"AuditRouteBulkArchive":  AuditRouteBulkArchive,
		"AuditSessionCreate":     AuditSessionCreate,
		"AuditSessionUpdate":     AuditSessionUpdate,
		"AuditSessionAssign":     AuditSessionAssign,
		"AuditTagCreate":         AuditTagCreate,
		"AuditTagDelete":         AuditTagDelete,
		"AuditMemberAdd":         AuditMemberAdd,
		"AuditMemberRemove":      AuditMemberRemove,
		"AuditMemberRoleChange":  AuditMemberRoleChange,
		"AuditLoginSuccess":      AuditLoginSuccess,
		"AuditLoginFailed":       AuditLoginFailed,
		"AuditAccountLocked":     AuditAccountLocked,
	}

	for name, val := range constants {
		if val == "" {
			t.Errorf("%s is empty", name)
		}
	}
}

func TestAuditConstants_DotNotation(t *testing.T) {
	// All audit actions should follow "resource.action" format
	constants := []string{
		AuditOrgUpdate,
		AuditLocationCreate,
		AuditLocationUpdate,
		AuditWallCreate,
		AuditWallUpdate,
		AuditWallDelete,
		AuditRouteCreate,
		AuditRouteUpdate,
		AuditRouteStatusChange,
		AuditRouteBulkArchive,
		AuditSessionCreate,
		AuditSessionUpdate,
		AuditSessionAssign,
		AuditTagCreate,
		AuditTagDelete,
		AuditMemberAdd,
		AuditMemberRemove,
		AuditMemberRoleChange,
		AuditLoginSuccess,
		AuditLoginFailed,
		AuditAccountLocked,
	}

	for _, c := range constants {
		dotCount := 0
		for _, ch := range c {
			if ch == '.' {
				dotCount++
			}
		}
		if dotCount != 1 {
			t.Errorf("audit constant %q should have exactly one dot", c)
		}
	}
}

func TestAuditConstants_NoDuplicates(t *testing.T) {
	constants := []string{
		AuditOrgUpdate,
		AuditLocationCreate,
		AuditLocationUpdate,
		AuditWallCreate,
		AuditWallUpdate,
		AuditWallDelete,
		AuditRouteCreate,
		AuditRouteUpdate,
		AuditRouteStatusChange,
		AuditRouteBulkArchive,
		AuditSessionCreate,
		AuditSessionUpdate,
		AuditSessionAssign,
		AuditTagCreate,
		AuditTagDelete,
		AuditMemberAdd,
		AuditMemberRemove,
		AuditMemberRoleChange,
		AuditLoginSuccess,
		AuditLoginFailed,
		AuditAccountLocked,
	}

	seen := make(map[string]bool)
	for _, c := range constants {
		if seen[c] {
			t.Errorf("duplicate audit constant %q", c)
		}
		seen[c] = true
	}
}

// ── Auth Error Constants ───────────────────────────────────────────

func TestAuthErrors_Distinct(t *testing.T) {
	errs := []error{
		ErrEmailTaken,
		ErrInvalidCredentials,
		ErrInvalidRefresh,
		ErrUserNotFound,
		ErrAccountLocked,
	}

	seen := make(map[string]bool)
	for _, e := range errs {
		msg := e.Error()
		if msg == "" {
			t.Error("auth error has empty message")
		}
		if seen[msg] {
			t.Errorf("duplicate auth error message: %q", msg)
		}
		seen[msg] = true
	}
}
