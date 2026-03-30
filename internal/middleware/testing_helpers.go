package middleware

import (
	"context"

	"github.com/shotwell-paddle/routewerk/internal/model"
)

// ── Test Helpers ────────────────────────────────────────────
// These exported helpers allow other packages to set context values in tests.
// They mirror how RequireSession populates context internally.

// SetWebUser stores a user in context (for testing web handlers).
func SetWebUser(ctx context.Context, user *model.User) context.Context {
	return context.WithValue(ctx, WebUserKey, user)
}

// SetWebRole stores the effective role in context (for testing web handlers).
func SetWebRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, WebRoleKey, role)
}

// SetWebRealRole stores the actual role in context (for testing view-as).
func SetWebRealRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, WebRealRoleKey, role)
}

// SetWebLocationID stores the active location ID in context (for testing).
func SetWebLocationID(ctx context.Context, locationID string) context.Context {
	return context.WithValue(ctx, WebLocationKey, &locationID)
}

// SetWebSession stores a web session in context (for testing).
func SetWebSession(ctx context.Context, session *model.WebSession) context.Context {
	return context.WithValue(ctx, WebSessionKey, session)
}
