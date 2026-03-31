package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

func strPtr(s string) *string { return &s }

func seedWebSessionUser(t *testing.T, pool *pgxpool.Pool, ctx context.Context) string {
	t.Helper()
	var id string
	err := pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, display_name) VALUES ($1, $2, $3) RETURNING id`,
		"session@test.com", "$2a$10$fakehash", "Session User",
	).Scan(&id)
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	return id
}

func newTestSession(userID, tokenHash string) *model.WebSession {
	return &model.WebSession{
		UserID:    userID,
		TokenHash: tokenHash,
		IPAddress: strPtr("10.0.0.1"),
		UserAgent: strPtr("Test"),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
}

func TestWebSessionRepo_CreateAndGetByTokenHash(t *testing.T) {
	pool := testDB(t)
	repo := NewWebSessionRepo(pool)
	ctx := context.Background()
	userID := seedWebSessionUser(t, pool, ctx)

	s := &model.WebSession{
		UserID:    userID,
		TokenHash: "abc123hash",
		IPAddress: strPtr("192.168.1.1"),
		UserAgent: strPtr("Mozilla/5.0 Test"),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	if err := repo.Create(ctx, s); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if s.ID == "" {
		t.Fatal("Create should populate ID")
	}
	if s.CreatedAt.IsZero() {
		t.Error("Create should populate CreatedAt")
	}

	got, err := repo.GetByTokenHash(ctx, "abc123hash")
	if err != nil {
		t.Fatalf("GetByTokenHash: %v", err)
	}
	if got == nil {
		t.Fatal("GetByTokenHash returned nil")
	}
	if got.UserID != userID {
		t.Errorf("UserID = %q, want %q", got.UserID, userID)
	}
	if got.IPAddress == nil || *got.IPAddress != "192.168.1.1" {
		t.Errorf("IPAddress = %v, want %q", got.IPAddress, "192.168.1.1")
	}
}

func TestWebSessionRepo_GetByTokenHash_Expired(t *testing.T) {
	pool := testDB(t)
	repo := NewWebSessionRepo(pool)
	ctx := context.Background()
	userID := seedWebSessionUser(t, pool, ctx)

	s := &model.WebSession{
		UserID:    userID,
		TokenHash: "expired-hash",
		IPAddress: strPtr("10.0.0.1"),
		UserAgent: strPtr("Test"),
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	repo.Create(ctx, s)

	got, err := repo.GetByTokenHash(ctx, "expired-hash")
	if err != nil {
		t.Fatalf("GetByTokenHash: %v", err)
	}
	if got != nil {
		t.Error("Expired session should not be returned")
	}
}

func TestWebSessionRepo_Delete(t *testing.T) {
	pool := testDB(t)
	repo := NewWebSessionRepo(pool)
	ctx := context.Background()
	userID := seedWebSessionUser(t, pool, ctx)

	s := newTestSession(userID, "to-delete")
	repo.Create(ctx, s)

	if err := repo.Delete(ctx, s.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, _ := repo.GetByTokenHash(ctx, "to-delete")
	if got != nil {
		t.Error("Deleted session should not be returned")
	}
}

func TestWebSessionRepo_DeleteAllForUser(t *testing.T) {
	pool := testDB(t)
	repo := NewWebSessionRepo(pool)
	ctx := context.Background()
	userID := seedWebSessionUser(t, pool, ctx)

	for _, hash := range []string{"hash-a", "hash-b", "hash-c"} {
		repo.Create(ctx, newTestSession(userID, hash))
	}

	count, _ := repo.CountForUser(ctx, userID)
	if count != 3 {
		t.Fatalf("CountForUser = %d, want 3", count)
	}

	if err := repo.DeleteAllForUser(ctx, userID); err != nil {
		t.Fatalf("DeleteAllForUser: %v", err)
	}

	count, _ = repo.CountForUser(ctx, userID)
	if count != 0 {
		t.Errorf("CountForUser after delete all = %d, want 0", count)
	}
}

func TestWebSessionRepo_DeleteExpired(t *testing.T) {
	pool := testDB(t)
	repo := NewWebSessionRepo(pool)
	ctx := context.Background()
	userID := seedWebSessionUser(t, pool, ctx)

	// One expired, one valid
	expired := newTestSession(userID, "expired-1")
	expired.ExpiresAt = time.Now().Add(-1 * time.Hour)
	repo.Create(ctx, expired)
	repo.Create(ctx, newTestSession(userID, "valid-1"))

	deleted, err := repo.DeleteExpired(ctx)
	if err != nil {
		t.Fatalf("DeleteExpired: %v", err)
	}
	if deleted != 1 {
		t.Errorf("Deleted = %d, want 1", deleted)
	}

	count, _ := repo.CountForUser(ctx, userID)
	if count != 1 {
		t.Errorf("Remaining sessions = %d, want 1", count)
	}
}

func TestWebSessionRepo_UpdateLocation(t *testing.T) {
	pool := testDB(t)
	repo := NewWebSessionRepo(pool)
	ctx := context.Background()
	userID := seedWebSessionUser(t, pool, ctx)

	var orgID, locID string
	pool.QueryRow(ctx,
		`INSERT INTO organizations (name, slug) VALUES ($1, $2) RETURNING id`,
		"Loc Switch Org", "loc-switch",
	).Scan(&orgID)
	pool.QueryRow(ctx,
		`INSERT INTO locations (org_id, name, slug, timezone) VALUES ($1, $2, $3, $4) RETURNING id`,
		orgID, "Target Loc", "target", "UTC",
	).Scan(&locID)

	s := newTestSession(userID, "loc-switch-hash")
	repo.Create(ctx, s)

	if err := repo.UpdateLocation(ctx, s.ID, locID); err != nil {
		t.Fatalf("UpdateLocation: %v", err)
	}

	got, _ := repo.GetByTokenHash(ctx, "loc-switch-hash")
	if got.LocationID == nil || *got.LocationID != locID {
		t.Errorf("LocationID = %v, want %q", got.LocationID, locID)
	}
}

func TestWebSessionRepo_ListForUser(t *testing.T) {
	pool := testDB(t)
	repo := NewWebSessionRepo(pool)
	ctx := context.Background()
	userID := seedWebSessionUser(t, pool, ctx)

	s1 := newTestSession(userID, "list-a")
	s1.UserAgent = strPtr("Chrome")
	s2 := newTestSession(userID, "list-b")
	s2.UserAgent = strPtr("Firefox")
	repo.Create(ctx, s1)
	repo.Create(ctx, s2)

	sessions, err := repo.ListForUser(ctx, userID)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("ListForUser returned %d, want 2", len(sessions))
	}
}

func TestWebSessionRepo_EnforceLimit(t *testing.T) {
	pool := testDB(t)
	repo := NewWebSessionRepo(pool)
	ctx := context.Background()
	userID := seedWebSessionUser(t, pool, ctx)

	for i := 0; i < 12; i++ {
		repo.Create(ctx, newTestSession(userID, fmt.Sprintf("enforce-%d", i)))
	}

	if err := repo.EnforceLimit(ctx, userID); err != nil {
		t.Fatalf("EnforceLimit: %v", err)
	}

	count, _ := repo.CountForUser(ctx, userID)
	if count != 10 {
		t.Errorf("Sessions after enforce = %d, want 10 (max)", count)
	}
}

func TestWebSessionRepo_RevokeAllForUser(t *testing.T) {
	pool := testDB(t)
	repo := NewWebSessionRepo(pool)
	ctx := context.Background()
	userID := seedWebSessionUser(t, pool, ctx)

	// Create 3 sessions
	for _, hash := range []string{"revoke-a", "revoke-b", "revoke-c"} {
		repo.Create(ctx, newTestSession(userID, hash))
	}

	revoked, err := repo.RevokeAllForUser(ctx, userID)
	if err != nil {
		t.Fatalf("RevokeAllForUser: %v", err)
	}
	if revoked != 3 {
		t.Errorf("Revoked = %d, want 3", revoked)
	}

	// Sessions should no longer be findable
	count, _ := repo.CountForUser(ctx, userID)
	if count != 0 {
		t.Errorf("Active sessions after revoke = %d, want 0", count)
	}

	// But they should still exist in the DB (soft revocation)
	var dbCount int
	pool.QueryRow(ctx, `SELECT COUNT(*) FROM web_sessions WHERE user_id = $1`, userID).Scan(&dbCount)
	if dbCount != 3 {
		t.Errorf("DB session rows = %d, want 3 (soft revoked, not deleted)", dbCount)
	}

	// Revoking again should affect 0
	revoked, _ = repo.RevokeAllForUser(ctx, userID)
	if revoked != 0 {
		t.Errorf("Second revoke = %d, want 0 (already revoked)", revoked)
	}
}

func TestWebSessionRepo_RevokeAllForUserExcept(t *testing.T) {
	pool := testDB(t)
	repo := NewWebSessionRepo(pool)
	ctx := context.Background()
	userID := seedWebSessionUser(t, pool, ctx)

	s1 := newTestSession(userID, "keep-this")
	s2 := newTestSession(userID, "revoke-this-1")
	s3 := newTestSession(userID, "revoke-this-2")
	repo.Create(ctx, s1)
	repo.Create(ctx, s2)
	repo.Create(ctx, s3)

	revoked, err := repo.RevokeAllForUserExcept(ctx, userID, s1.ID)
	if err != nil {
		t.Fatalf("RevokeAllForUserExcept: %v", err)
	}
	if revoked != 2 {
		t.Errorf("Revoked = %d, want 2", revoked)
	}

	// The kept session should still be valid
	got, _ := repo.GetByTokenHash(ctx, "keep-this")
	if got == nil {
		t.Error("Kept session should still be accessible")
	}

	// The revoked ones should not
	got, _ = repo.GetByTokenHash(ctx, "revoke-this-1")
	if got != nil {
		t.Error("Revoked session should not be accessible")
	}
}

func TestWebSessionRepo_TouchLastSeen(t *testing.T) {
	pool := testDB(t)
	repo := NewWebSessionRepo(pool)
	ctx := context.Background()
	userID := seedWebSessionUser(t, pool, ctx)

	s := newTestSession(userID, "touch-test")
	repo.Create(ctx, s)
	originalLastSeen := s.LastSeenAt

	time.Sleep(10 * time.Millisecond)

	if err := repo.TouchLastSeen(ctx, s.ID); err != nil {
		t.Fatalf("TouchLastSeen: %v", err)
	}

	got, _ := repo.GetByTokenHash(ctx, "touch-test")
	if !got.LastSeenAt.After(originalLastSeen) {
		t.Error("LastSeenAt should be updated after touch")
	}
}
