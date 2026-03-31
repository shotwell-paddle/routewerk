package repository

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

// testFixture creates an org, location, wall, and setter for route tests.
type testFixture struct {
	OrgID      string
	LocationID string
	WallID     string
	Wall2ID    string
	SetterID   string
}

func seedRouteFixture(t *testing.T, pool *pgxpool.Pool, ctx context.Context) testFixture {
	t.Helper()
	var f testFixture

	pool.QueryRow(ctx,
		`INSERT INTO organizations (name, slug) VALUES ($1, $2) RETURNING id`,
		"Route Test Org", "route-org",
	).Scan(&f.OrgID)

	pool.QueryRow(ctx,
		`INSERT INTO locations (org_id, name, slug, timezone) VALUES ($1, $2, $3, $4) RETURNING id`,
		f.OrgID, "Route Test Loc", "route-loc", "America/Denver",
	).Scan(&f.LocationID)

	pool.QueryRow(ctx,
		`INSERT INTO walls (location_id, name, wall_type, sort_order) VALUES ($1, $2, $3, $4) RETURNING id`,
		f.LocationID, "Main Wall", "boulder", 1,
	).Scan(&f.WallID)

	pool.QueryRow(ctx,
		`INSERT INTO walls (location_id, name, wall_type, sort_order) VALUES ($1, $2, $3, $4) RETURNING id`,
		f.LocationID, "Side Wall", "route", 2,
	).Scan(&f.Wall2ID)

	pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, display_name) VALUES ($1, $2, $3) RETURNING id`,
		"setter@route-test.com", "$2a$10$fakehash", "Test Setter",
	).Scan(&f.SetterID)

	return f
}

func TestRouteRepo_CreateAndGetByID(t *testing.T) {
	pool := testDB(t)
	repo := NewRouteRepo(pool)
	ctx := context.Background()
	f := seedRouteFixture(t, pool, ctx)

	today := time.Now().Truncate(24 * time.Hour)
	rt := &model.Route{
		LocationID:    f.LocationID,
		WallID:        f.WallID,
		SetterID:      &f.SetterID,
		RouteType:     "boulder",
		Status:        "active",
		GradingSystem: "v_scale",
		Grade:         "V4",
		Color:         "#FF5733",
		DateSet:       today,
	}
	if err := repo.Create(ctx, rt); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if rt.ID == "" {
		t.Fatal("Create should populate ID")
	}
	if rt.AscentCount != 0 {
		t.Errorf("AscentCount = %d, want 0 for new route", rt.AscentCount)
	}

	got, err := repo.GetByID(ctx, rt.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("GetByID returned nil")
	}
	if got.Grade != "V4" {
		t.Errorf("Grade = %q, want %q", got.Grade, "V4")
	}
	if got.Color != "#FF5733" {
		t.Errorf("Color = %q, want %q", got.Color, "#FF5733")
	}
	if got.Status != "active" {
		t.Errorf("Status = %q, want %q", got.Status, "active")
	}
}

func TestRouteRepo_GetByID_WithTags(t *testing.T) {
	pool := testDB(t)
	repo := NewRouteRepo(pool)
	ctx := context.Background()
	f := seedRouteFixture(t, pool, ctx)

	rt := &model.Route{
		LocationID: f.LocationID, WallID: f.WallID, RouteType: "boulder",
		Status: "active", GradingSystem: "v_scale", Grade: "V3", Color: "#000",
		DateSet: time.Now().Truncate(24 * time.Hour),
	}
	repo.Create(ctx, rt)

	// Create a tag and link it
	var tagID string
	pool.QueryRow(ctx,
		`INSERT INTO tags (org_id, category, name, color) VALUES ($1, $2, $3, $4) RETURNING id`,
		f.OrgID, "style", "Crimpy", "#FFD700",
	).Scan(&tagID)

	pool.QueryRow(ctx,
		`INSERT INTO route_tags (route_id, tag_id) VALUES ($1, $2) RETURNING route_id`,
		rt.ID, tagID,
	)

	got, err := repo.GetByID(ctx, rt.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if len(got.Tags) != 1 {
		t.Fatalf("Tags count = %d, want 1", len(got.Tags))
	}
	if got.Tags[0].Name != "Crimpy" {
		t.Errorf("Tag name = %q, want %q", got.Tags[0].Name, "Crimpy")
	}
}

func TestRouteRepo_List_Filtering(t *testing.T) {
	pool := testDB(t)
	repo := NewRouteRepo(pool)
	ctx := context.Background()
	f := seedRouteFixture(t, pool, ctx)
	today := time.Now().Truncate(24 * time.Hour)

	// Create routes with different attributes
	repo.Create(ctx, &model.Route{
		LocationID: f.LocationID, WallID: f.WallID, RouteType: "boulder",
		Status: "active", GradingSystem: "v_scale", Grade: "V3", Color: "#000", DateSet: today,
	})
	repo.Create(ctx, &model.Route{
		LocationID: f.LocationID, WallID: f.WallID, RouteType: "boulder",
		Status: "active", GradingSystem: "v_scale", Grade: "V6", Color: "#FFF", DateSet: today,
	})
	repo.Create(ctx, &model.Route{
		LocationID: f.LocationID, WallID: f.Wall2ID, RouteType: "route",
		Status: "active", GradingSystem: "yds", Grade: "5.10a", Color: "#F00", DateSet: today,
	})
	repo.Create(ctx, &model.Route{
		LocationID: f.LocationID, WallID: f.WallID, RouteType: "boulder",
		Status: "archived", GradingSystem: "v_scale", Grade: "V4", Color: "#0F0", DateSet: today,
	})

	// All active at location
	routes, total, err := repo.List(ctx, RouteFilter{LocationID: f.LocationID, Status: "active"})
	if err != nil {
		t.Fatalf("List active: %v", err)
	}
	if total != 3 {
		t.Errorf("Total active = %d, want 3", total)
	}
	if len(routes) != 3 {
		t.Errorf("Routes returned = %d, want 3", len(routes))
	}

	// Filter by wall
	routes, total, _ = repo.List(ctx, RouteFilter{LocationID: f.LocationID, WallID: f.WallID, Status: "active"})
	if total != 2 {
		t.Errorf("Total on main wall = %d, want 2", total)
	}

	// Filter by route type
	routes, total, _ = repo.List(ctx, RouteFilter{LocationID: f.LocationID, RouteType: "route"})
	if total != 1 {
		t.Errorf("Total routes (not boulders) = %d, want 1", total)
	}
	if routes[0].Grade != "5.10a" {
		t.Errorf("Route grade = %q, want %q", routes[0].Grade, "5.10a")
	}

	// Pagination
	routes, total, _ = repo.List(ctx, RouteFilter{LocationID: f.LocationID, Limit: 2, Offset: 0})
	if total != 4 {
		t.Errorf("Total all = %d, want 4", total)
	}
	if len(routes) != 2 {
		t.Errorf("Page size = %d, want 2", len(routes))
	}
}

func TestRouteRepo_BulkArchive(t *testing.T) {
	pool := testDB(t)
	repo := NewRouteRepo(pool)
	ctx := context.Background()
	f := seedRouteFixture(t, pool, ctx)
	today := time.Now().Truncate(24 * time.Hour)

	r1 := &model.Route{
		LocationID: f.LocationID, WallID: f.WallID, RouteType: "boulder",
		Status: "active", GradingSystem: "v_scale", Grade: "V2", Color: "#000", DateSet: today,
	}
	r2 := &model.Route{
		LocationID: f.LocationID, WallID: f.WallID, RouteType: "boulder",
		Status: "active", GradingSystem: "v_scale", Grade: "V5", Color: "#FFF", DateSet: today,
	}
	repo.Create(ctx, r1)
	repo.Create(ctx, r2)

	archived, err := repo.BulkArchive(ctx, []string{r1.ID, r2.ID})
	if err != nil {
		t.Fatalf("BulkArchive: %v", err)
	}
	if archived != 2 {
		t.Errorf("Archived = %d, want 2", archived)
	}

	// Verify they're archived
	got, _ := repo.GetByID(ctx, r1.ID)
	if got.Status != "archived" {
		t.Errorf("Status = %q, want archived", got.Status)
	}
	if got.DateStripped == nil {
		t.Error("DateStripped should be set after archive")
	}
}

func TestRouteRepo_BulkArchiveByWall(t *testing.T) {
	pool := testDB(t)
	repo := NewRouteRepo(pool)
	ctx := context.Background()
	f := seedRouteFixture(t, pool, ctx)
	today := time.Now().Truncate(24 * time.Hour)

	// Two active on main wall, one on side wall
	repo.Create(ctx, &model.Route{
		LocationID: f.LocationID, WallID: f.WallID, RouteType: "boulder",
		Status: "active", GradingSystem: "v_scale", Grade: "V1", Color: "#000", DateSet: today,
	})
	repo.Create(ctx, &model.Route{
		LocationID: f.LocationID, WallID: f.WallID, RouteType: "boulder",
		Status: "active", GradingSystem: "v_scale", Grade: "V3", Color: "#000", DateSet: today,
	})
	repo.Create(ctx, &model.Route{
		LocationID: f.LocationID, WallID: f.Wall2ID, RouteType: "route",
		Status: "active", GradingSystem: "yds", Grade: "5.9", Color: "#000", DateSet: today,
	})

	archived, err := repo.BulkArchiveByWall(ctx, f.WallID)
	if err != nil {
		t.Fatalf("BulkArchiveByWall: %v", err)
	}
	if archived != 2 {
		t.Errorf("Archived = %d, want 2 (only main wall)", archived)
	}

	// Side wall route should still be active
	routes, total, _ := repo.List(ctx, RouteFilter{LocationID: f.LocationID, WallID: f.Wall2ID, Status: "active"})
	if total != 1 {
		t.Errorf("Side wall active = %d, want 1", total)
	}
	_ = routes
}

func TestRouteRepo_SetTags(t *testing.T) {
	pool := testDB(t)
	repo := NewRouteRepo(pool)
	ctx := context.Background()
	f := seedRouteFixture(t, pool, ctx)
	today := time.Now().Truncate(24 * time.Hour)

	rt := &model.Route{
		LocationID: f.LocationID, WallID: f.WallID, RouteType: "boulder",
		Status: "active", GradingSystem: "v_scale", Grade: "V5", Color: "#000", DateSet: today,
	}
	repo.Create(ctx, rt)

	// Create two tags
	var tag1, tag2 string
	pool.QueryRow(ctx,
		`INSERT INTO tags (org_id, category, name) VALUES ($1, 'style', 'Slab') RETURNING id`, f.OrgID,
	).Scan(&tag1)
	pool.QueryRow(ctx,
		`INSERT INTO tags (org_id, category, name) VALUES ($1, 'style', 'Overhung') RETURNING id`, f.OrgID,
	).Scan(&tag2)

	// Set both tags
	if err := repo.SetTags(ctx, rt.ID, []string{tag1, tag2}); err != nil {
		t.Fatalf("SetTags: %v", err)
	}

	tags, err := repo.GetTags(ctx, rt.ID)
	if err != nil {
		t.Fatalf("GetTags: %v", err)
	}
	if len(tags) != 2 {
		t.Errorf("Tags count = %d, want 2", len(tags))
	}

	// Replace with single tag
	if err := repo.SetTags(ctx, rt.ID, []string{tag1}); err != nil {
		t.Fatalf("SetTags (replace): %v", err)
	}
	tags, _ = repo.GetTags(ctx, rt.ID)
	if len(tags) != 1 {
		t.Errorf("Tags count after replace = %d, want 1", len(tags))
	}
}

func TestRouteRepo_ListWithDetails(t *testing.T) {
	pool := testDB(t)
	repo := NewRouteRepo(pool)
	ctx := context.Background()
	f := seedRouteFixture(t, pool, ctx)
	today := time.Now().Truncate(24 * time.Hour)

	repo.Create(ctx, &model.Route{
		LocationID: f.LocationID, WallID: f.WallID, SetterID: &f.SetterID,
		RouteType: "boulder", Status: "active", GradingSystem: "v_scale",
		Grade: "V4", Color: "#000", DateSet: today,
	})

	routes, total, err := repo.ListWithDetails(ctx, RouteFilter{LocationID: f.LocationID})
	if err != nil {
		t.Fatalf("ListWithDetails: %v", err)
	}
	if total != 1 {
		t.Fatalf("Total = %d, want 1", total)
	}
	if routes[0].WallName != "Main Wall" {
		t.Errorf("WallName = %q, want %q", routes[0].WallName, "Main Wall")
	}
	if routes[0].SetterName != "Test Setter" {
		t.Errorf("SetterName = %q, want %q", routes[0].SetterName, "Test Setter")
	}
}
