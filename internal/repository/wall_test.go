package repository

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

func seedOrgAndLocationForWall(t *testing.T, pool *pgxpool.Pool, ctx context.Context) (orgID, locationID string) {
	t.Helper()
	pool.QueryRow(ctx,
		`INSERT INTO organizations (name, slug) VALUES ($1, $2) RETURNING id`,
		"Wall Test Org", "wall-org",
	).Scan(&orgID)

	pool.QueryRow(ctx,
		`INSERT INTO locations (org_id, name, slug, timezone) VALUES ($1, $2, $3, $4) RETURNING id`,
		orgID, "Wall Test Loc", "wall-loc", "UTC",
	).Scan(&locationID)
	return
}

func TestWallRepo_CreateAndGetByID(t *testing.T) {
	pool := testDB(t)
	repo := NewWallRepo(pool)
	ctx := context.Background()
	_, locID := seedOrgAndLocationForWall(t, pool, ctx)

	w := &model.Wall{
		LocationID: locID,
		Name:       "The Cave",
		WallType:   "boulder",
		SortOrder:  1,
	}
	if err := repo.Create(ctx, w); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if w.ID == "" {
		t.Fatal("Create should populate ID")
	}

	got, err := repo.GetByID(ctx, w.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("GetByID returned nil")
	}
	if got.Name != "The Cave" {
		t.Errorf("Name = %q, want %q", got.Name, "The Cave")
	}
	if got.WallType != "boulder" {
		t.Errorf("WallType = %q, want %q", got.WallType, "boulder")
	}
}

func TestWallRepo_ListByLocation_SortOrder(t *testing.T) {
	pool := testDB(t)
	repo := NewWallRepo(pool)
	ctx := context.Background()
	_, locID := seedOrgAndLocationForWall(t, pool, ctx)

	// Create walls out of sort order
	repo.Create(ctx, &model.Wall{LocationID: locID, Name: "Wall C", WallType: "route", SortOrder: 3})
	repo.Create(ctx, &model.Wall{LocationID: locID, Name: "Wall A", WallType: "boulder", SortOrder: 1})
	repo.Create(ctx, &model.Wall{LocationID: locID, Name: "Wall B", WallType: "boulder", SortOrder: 2})

	walls, err := repo.ListByLocation(ctx, locID)
	if err != nil {
		t.Fatalf("ListByLocation: %v", err)
	}
	if len(walls) != 3 {
		t.Fatalf("ListByLocation returned %d, want 3", len(walls))
	}
	// Should be sorted by sort_order
	if walls[0].Name != "Wall A" || walls[1].Name != "Wall B" || walls[2].Name != "Wall C" {
		t.Errorf("Sort order wrong: %q, %q, %q", walls[0].Name, walls[1].Name, walls[2].Name)
	}
}

func TestWallRepo_ListWithCounts(t *testing.T) {
	pool := testDB(t)
	wallRepo := NewWallRepo(pool)
	ctx := context.Background()
	_, locID := seedOrgAndLocationForWall(t, pool, ctx)

	w := &model.Wall{LocationID: locID, Name: "Counted Wall", WallType: "boulder", SortOrder: 1}
	wallRepo.Create(ctx, w)

	// Add a route to the wall
	pool.QueryRow(ctx,
		`INSERT INTO routes (location_id, wall_id, route_type, status, grading_system, grade, color, date_set)
		 VALUES ($1, $2, 'boulder', 'active', 'v_scale', 'V4', '#FF0000', CURRENT_DATE) RETURNING id`,
		locID, w.ID,
	)

	walls, err := wallRepo.ListWithCounts(ctx, locID)
	if err != nil {
		t.Fatalf("ListWithCounts: %v", err)
	}
	if len(walls) != 1 {
		t.Fatalf("ListWithCounts returned %d, want 1", len(walls))
	}
	if walls[0].ActiveRoutes != 1 {
		t.Errorf("ActiveRoutes = %d, want 1", walls[0].ActiveRoutes)
	}
	if walls[0].TotalRoutes != 1 {
		t.Errorf("TotalRoutes = %d, want 1", walls[0].TotalRoutes)
	}
}

func TestWallRepo_SoftDelete(t *testing.T) {
	pool := testDB(t)
	repo := NewWallRepo(pool)
	ctx := context.Background()
	_, locID := seedOrgAndLocationForWall(t, pool, ctx)

	w := &model.Wall{LocationID: locID, Name: "Doomed Wall", WallType: "boulder", SortOrder: 1}
	repo.Create(ctx, w)

	if err := repo.Delete(ctx, w.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, err := repo.GetByID(ctx, w.ID)
	if err != nil {
		t.Fatalf("GetByID after delete: %v", err)
	}
	if got != nil {
		t.Error("Soft-deleted wall should not be returned by GetByID")
	}
}

func TestWallRepo_Update(t *testing.T) {
	pool := testDB(t)
	repo := NewWallRepo(pool)
	ctx := context.Background()
	_, locID := seedOrgAndLocationForWall(t, pool, ctx)

	w := &model.Wall{LocationID: locID, Name: "Original", WallType: "boulder", SortOrder: 1}
	repo.Create(ctx, w)

	w.Name = "Renamed Wall"
	w.SortOrder = 5
	if err := repo.Update(ctx, w); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, _ := repo.GetByID(ctx, w.ID)
	if got.Name != "Renamed Wall" {
		t.Errorf("Name = %q, want %q", got.Name, "Renamed Wall")
	}
	if got.SortOrder != 5 {
		t.Errorf("SortOrder = %d, want 5", got.SortOrder)
	}
}
