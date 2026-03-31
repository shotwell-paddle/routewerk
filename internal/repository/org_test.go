package repository

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

// ── helpers ───────────────────────────────────────────────────────────

func createTestUser(t *testing.T, pool *pgxpool.Pool, ctx context.Context, email, name string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, display_name) VALUES ($1, $2, $3) RETURNING id`,
		email, "$2a$10$fakehash", name,
	).Scan(&id)
	if err != nil {
		t.Fatalf("create test user %s: %v", email, err)
	}
	return id
}

// ── Org CRUD ──────────────────────────────────────────────────────────

func TestOrgRepo_CreateAndGetByID(t *testing.T) {
	pool := testDB(t)
	repo := NewOrgRepo(pool)
	ctx := context.Background()

	o := &model.Organization{Name: "LEF Climbing", Slug: "lef-climbing"}
	if err := repo.Create(ctx, o); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if o.ID == "" {
		t.Fatal("Create should populate ID")
	}
	if o.CreatedAt.IsZero() {
		t.Error("Create should populate CreatedAt")
	}

	got, err := repo.GetByID(ctx, o.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("GetByID returned nil")
	}
	if got.Name != "LEF Climbing" {
		t.Errorf("Name = %q, want %q", got.Name, "LEF Climbing")
	}
	if got.Slug != "lef-climbing" {
		t.Errorf("Slug = %q, want %q", got.Slug, "lef-climbing")
	}
}

func TestOrgRepo_GetBySlug(t *testing.T) {
	pool := testDB(t)
	repo := NewOrgRepo(pool)
	ctx := context.Background()

	o := &model.Organization{Name: "Slug Test Gym", Slug: "slug-test"}
	if err := repo.Create(ctx, o); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetBySlug(ctx, "slug-test")
	if err != nil {
		t.Fatalf("GetBySlug: %v", err)
	}
	if got == nil || got.ID != o.ID {
		t.Fatalf("GetBySlug returned wrong org: got %v", got)
	}

	// Not found
	got, err = repo.GetBySlug(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("GetBySlug (not found): %v", err)
	}
	if got != nil {
		t.Error("GetBySlug should return nil for nonexistent slug")
	}
}

func TestOrgRepo_GetByID_NotFound(t *testing.T) {
	pool := testDB(t)
	repo := NewOrgRepo(pool)
	ctx := context.Background()

	got, err := repo.GetByID(ctx, "00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got != nil {
		t.Error("GetByID should return nil for nonexistent org")
	}
}

func TestOrgRepo_Count(t *testing.T) {
	pool := testDB(t)
	repo := NewOrgRepo(pool)
	ctx := context.Background()

	count, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 0 {
		t.Errorf("Count = %d, want 0 in fresh schema", count)
	}

	repo.Create(ctx, &model.Organization{Name: "Gym A", Slug: "gym-a"})
	repo.Create(ctx, &model.Organization{Name: "Gym B", Slug: "gym-b"})

	count, err = repo.Count(ctx)
	if err != nil {
		t.Fatalf("Count after inserts: %v", err)
	}
	if count != 2 {
		t.Errorf("Count = %d, want 2", count)
	}
}

func TestOrgRepo_Update(t *testing.T) {
	pool := testDB(t)
	repo := NewOrgRepo(pool)
	ctx := context.Background()

	o := &model.Organization{Name: "Original", Slug: "original"}
	if err := repo.Create(ctx, o); err != nil {
		t.Fatalf("Create: %v", err)
	}

	o.Name = "Updated Name"
	o.Slug = "updated-name"
	if err := repo.Update(ctx, o); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.GetByID(ctx, o.ID)
	if err != nil {
		t.Fatalf("GetByID after update: %v", err)
	}
	if got.Name != "Updated Name" {
		t.Errorf("Name = %q, want %q", got.Name, "Updated Name")
	}
}

// ── Membership ────────────────────────────────────────────────────────

func TestOrgRepo_AddMember_And_ListByUser(t *testing.T) {
	pool := testDB(t)
	orgRepo := NewOrgRepo(pool)
	ctx := context.Background()

	// Create org + user
	o := &model.Organization{Name: "Member Test", Slug: "member-test"}
	if err := orgRepo.Create(ctx, o); err != nil {
		t.Fatalf("Create org: %v", err)
	}
	userID := createTestUser(t, pool, ctx, "member@test.com", "Member")

	// Add membership
	m := &model.UserMembership{
		UserID: userID,
		OrgID:  o.ID,
		Role:   "setter",
	}
	if err := orgRepo.AddMember(ctx, m); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	if m.ID == "" {
		t.Error("AddMember should populate ID")
	}

	// ListByUser
	orgs, err := orgRepo.ListByUser(ctx, userID)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(orgs) != 1 {
		t.Fatalf("ListByUser returned %d orgs, want 1", len(orgs))
	}
	if orgs[0].ID != o.ID {
		t.Errorf("ListByUser[0].ID = %q, want %q", orgs[0].ID, o.ID)
	}
}

func TestOrgRepo_DuplicateSlug(t *testing.T) {
	pool := testDB(t)
	repo := NewOrgRepo(pool)
	ctx := context.Background()

	repo.Create(ctx, &model.Organization{Name: "First", Slug: "unique-slug"})
	err := repo.Create(ctx, &model.Organization{Name: "Second", Slug: "unique-slug"})
	if err == nil {
		t.Error("Create should fail for duplicate slug")
	}
}
