package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestApplyViewAsOverride pins the privilege-downgrade rules: the view-as
// cookie may only LOWER the effective role, never raise it, and unknown
// or org_admin targets are ignored. This is privilege-boundary code — a
// regression here silently changes what staff can see while impersonating.
func TestApplyViewAsOverride(t *testing.T) {
	tests := []struct {
		name     string
		cookie   string // "" = no cookie
		realRole string
		want     string
	}{
		{"no cookie keeps real role", "", "head_setter", "head_setter"},
		{"empty cookie keeps real role", "", "gym_manager", "gym_manager"},
		{"downgrade head_setter to climber", "climber", "head_setter", "climber"},
		{"downgrade org_admin to setter", "setter", "org_admin", "setter"},
		{"downgrade gym_manager to head_setter", "head_setter", "gym_manager", "head_setter"},
		{"equal rank ignored", "head_setter", "head_setter", "head_setter"},
		{"upgrade attempt ignored", "gym_manager", "setter", "setter"},
		{"org_admin target never valid", "org_admin", "org_admin", "org_admin"},
		{"unknown role ignored", "superuser", "org_admin", "org_admin"},
		{"garbage value ignored", "'; DROP TABLE users;--", "head_setter", "head_setter"},
		{"climber cannot downgrade further", "climber", "climber", "climber"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.cookie != "" {
				r.AddCookie(&http.Cookie{Name: ViewAsCookieName, Value: tt.cookie})
			}
			got := applyViewAsOverride(r, tt.realRole)
			if got != tt.want {
				t.Errorf("applyViewAsOverride(cookie=%q, real=%q) = %q, want %q",
					tt.cookie, tt.realRole, got, tt.want)
			}
		})
	}
}
