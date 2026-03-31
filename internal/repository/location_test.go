package repository

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

func seedOrg(t *testing.T, pool *pgxpool.Pool, ctx context.Context) string {
	t.Helper()
	var id string
	err := pool.QueryRow(ctx,
		`INSERT INTO organizations (name, slug) VALUES ($1, $2) RETURNING id`,
		"Test Org", "test-org",
	).Scan(&id)
	if err != nil {
		t.Fatalf("create test org: %v", err)
	}
	return id
}

func TestLocationRepo_CreateAndGetByID(t *testing.T) {
	pool := testDB(t)
	repo := NewLocationRepo(pool)
	ctx := context.Background()
	orgID := seedOrg(t, pool, ctx)

	l := &model.Location{
		OrgID:    orgID,
		Name:     "Boulder Barn",
		Slug:     "boulder-barn",
		Timezone: "America/Denver",
	}
	if err := repo.Create(ctx, l); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if l.ID == "" {
		t.Fatal("Create should populate ID")
	}

	got, err := repo.GetByID(ctx, l.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("GetByID returned nil")
	}
	if got.Name != "Boulder Barn" {
		t.Errorf("Name = %q, want %q", got.Name, "Boulder Barn")
	}
	if got.OrgID != orgID {
		t.Errorf("OrgID = %q, want %q", got.OrgID, orgID)
	}
	if got.Timezone != "America/Denver" {
		t.Errorf("Timezone = %q, want %q", got.Timezone, "America/Denver")
	}
}

func TestLocationRepo_ListByOrg(t *testing.T) {
	pool := testDB(t)
	repo := NewLocationRepo(pool)
	ctx := context.Background()
	orgID := seedOrg(t, pool, ctx)

	// Create two locations — should be returned alphabetically
	repo.Create(ctx, &model.Location{OrgID: orgID, Name: "Zephyr Wall", Slug: "zephyr", Timezone: "UTC"})
	repo.Create(ctx, &model.Location{OrgID: orgID, Name: "Alpha Boulders", Slug: "alpha", Timezone: "UTC"})

	locations, err := repo.ListByOrg(ctx, orgID)
	if err != nil {
		t.Fatalf("ListByOrg: %v", err)
	}
	if len(locations) != 2 {
		t.Fatalf("ListByOrg returned %d, want 2", len(locations))
	}
	// Alphabetical order
	if locations[0].Name != "Alpha Boulders" {
		t.Errorf("First location = %q, want %q", locations[0].Name, "Alpha Boulders")
	}
	if locations[1].Name != "Zephyr Wall" {
		t.Errorf("Second location = %q, want %q", locations[1].Name, "Zephyr Wall")
	}
}

func TestLocationRepo_SearchPublic(t *testing.T) {
	pool := testDB(t)
	repo := NewLocationRepo(pool)
	ctx := context.Background()
	orgID := seedOrg(t, pool, ctx)

	addr := "123 Main St"
	repo.Create(ctx, &model.Location{OrgID: orgID, Name: "Granite Gym", Slug: "granite", Timezone: "UTC", Address: &addr})
	repo.Create(ctx, &model.Location{OrgID: orgID, Name: "Sandstone Studio", Slug: "sandstone", Timezone: "UTC"})

	// Partial match
	results, err := repo.SearchPublic(ctx, "gran", 10)
	if err != nil {
		t.Fatalf("SearchPublic: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("SearchPublic 'gran' returned %d, want 1", len(results))
	}
	if results[0].Name != "Granite Gym" {
		t.Errorf("Name = %q, want %q", results[0].Name, "Granite Gym")
	}
	if results[0].OrgName != "Test Org" {
		t.Errorf("OrgName = %q, want %q", results[0].OrgName, "Test Org")
	}

	// No match
	results, err = repo.SearchPublic(ctx, "nonexistent", 10)
	if err != nil {
		t.Fatalf("SearchPublic (no match): %v", err)
	}
	if len(results) != 0 {
		t.Errorf("SearchPublic no match returned %d, want 0", len(results))
	}
}

func TestLocationRepo_ListForUser(t *testing.T) {
	pool := testDB(t)
	locRepo := NewLocationRepo(pool)
	orgRepo := NewOrgRepo(pool)
	ctx := context.Background()

	// Create org, user, two locations
	o := &model.Organization{Name: "Multi Loc", Slug: "multi-loc"}
	orgRepo.Create(ctx, o)
	userID := createTestUser(t, pool, ctx, "multi@test.com", "Multi User")

	l1 := &model.Location{OrgID: o.ID, Name: "Location A", Slug: "loc-a", Timezone: "UTC"}
	l2 := &model.Location{OrgID: o.ID, Name: "Location B", Slug: "loc-b", Timezone: "UTC"}
	locRepo.Create(ctx, l1)
	locRepo.Create(ctx, l2)

	// Give user setter role at location A only
	orgRepo.AddMember(ctx, &model.UserMembership{UserID: userID, OrgID: o.ID, LocationID: &l1.ID, Role: "setter"})

	locations, err := locRepo.ListForUser(ctx, userID)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(locations) != 1 {
		t.Fatalf("ListForUser returned %d, want 1", len(locations))
	}
	if locations[0].ID != l1.ID {
		t.Errorf("Location ID = %q, want %q", locations[0].ID, l1.ID)
	}
	if locations[0].Role != "setter" {
		t.Errorf("Role = %q, want %q", locations[0].Role, "setter")
	}
}

func TestLocationRepo_ListForUser_OrgWide(t *testing.T) {
	pool := testDB(t)
	locRepo := NewLocationRepo(pool)
	orgRepo := NewOrgRepo(pool)
	ctx := context.Background()

	o := &model.Organization{Name: "Org Wide", Slug: "org-wide"}
	orgRepo.Create(ctx, o)
	userID := createTestUser(t, pool, ctx, "admin@test.com", "Admin")

	l1 := &model.Location{OrgID: o.ID, Name: "Loc 1", Slug: "loc-1", Timezone: "UTC"}
	l2 := &model.Location{OrgID: o.ID, Name: "Loc 2", Slug: "loc-2", Timezone: "UTC"}
	locRepo.Create(ctx, l1)
	locRepo.Create(ctx, l2)

	// Org-wide admin membership (location_id = nil) should see all locations
	orgRepo.AddMember(ctx, &model.UserMembership{UserID: userID, OrgID: o.ID, Role: "org_admin"})

	locations, err := locRepo.ListForUser(ctx, userID)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(locations) != 2 {
		t.Fatalf("Org-wide admin should see all %d locations, got %d", 2, len(locations))
	}
	for _, loc := range locations {
		if loc.Role != "org_admin" {
			t.Errorf("Role for %s = %q, want org_admin", loc.Name, loc.Role)
		}
	}
}

func TestLocationRepo_Update(t *testing.T) {
	pool := testDB(t)
	repo := NewLocationRepo(pool)
	ctx := context.Background()
	orgID := seedOrg(t, pool, ctx)

	l := &model.Location{OrgID: orgID, Name: "Original", Slug: "original", Timezone: "UTC"}
	repo.Create(ctx, l)

	l.Name = "Renamed Gym"
	l.Slug = "renamed-gym"
	if err := repo.Update(ctx, l); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.GetByID(ctx, l.ID)
	if err != nil {
		t.Fatalf("GetByID after update: %v", err)
	}
	if got.Name != "Renamed Gym" {
		t.Errorf("Name = %q, want %q", got.Name, "Renamed Gym")
	}
}

func TestLocationRepo_GetByCustomDomain(t *testing.T) {
	pool := testDB(t)
	repo := NewLocationRepo(pool)
	ctx := context.Background()
	orgID := seedOrg(t, pool, ctx)

	domain := "custom.climbing.com"
	l := &model.Location{OrgID: orgID, Name: "Custom", Slug: "custom", Timezone: "UTC", CustomDomain: &domain}
	repo.Create(ctx, l)

	got, err := repo.GetByCustomDomain(ctx, "custom.climbing.com")
	if err != nil {
		t.Fatalf("GetByCustomDomain: %v", err)
	}
	if got == nil || got.ID != l.ID {
		t.Fatalf("GetByCustomDomain returned wrong location")
	}

	// Not found
	got, err = repo.GetByCustomDomain(ctx, "nope.com")
	if err != nil {
		t.Fatalf("GetByCustomDomain (not found): %v", err)
	}
	if got != nil {
		t.Error("should return nil for nonexistent domain")
	}
}
