package repository

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shotwell-paddle/routewerk/internal/model"
)

func seedAscentFixture(t *testing.T, pool *pgxpool.Pool, ctx context.Context) (locationID, wallID, routeID, userID string) {
	t.Helper()
	var orgID string
	pool.QueryRow(ctx,
		`INSERT INTO organizations (name, slug) VALUES ($1, $2) RETURNING id`,
		"Ascent Org", "ascent-org",
	).Scan(&orgID)
	pool.QueryRow(ctx,
		`INSERT INTO locations (org_id, name, slug, timezone) VALUES ($1, $2, $3, $4) RETURNING id`,
		orgID, "Ascent Loc", "ascent-loc", "UTC",
	).Scan(&locationID)
	pool.QueryRow(ctx,
		`INSERT INTO walls (location_id, name, wall_type, sort_order) VALUES ($1, $2, $3, $4) RETURNING id`,
		locationID, "Test Wall", "boulder", 1,
	).Scan(&wallID)
	pool.QueryRow(ctx,
		`INSERT INTO routes (location_id, wall_id, route_type, status, grading_system, grade, color, date_set)
		 VALUES ($1, $2, 'boulder', 'active', 'v_scale', 'V5', '#FF0000', CURRENT_DATE) RETURNING id`,
		locationID, wallID,
	).Scan(&routeID)
	pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, display_name) VALUES ($1, $2, $3) RETURNING id`,
		"climber@test.com", "$2a$10$fakehash", "Test Climber",
	).Scan(&userID)
	return
}

func TestAscentRepo_CreateAndGetByID(t *testing.T) {
	pool := testDB(t)
	repo := NewAscentRepo(pool)
	ctx := context.Background()
	_, _, routeID, userID := seedAscentFixture(t, pool, ctx)

	a := &model.Ascent{
		UserID:     userID,
		RouteID:    routeID,
		AscentType: "send",
		Attempts:   3,
		ClimbedAt:  time.Now(),
	}
	if err := repo.Create(ctx, a); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if a.ID == "" {
		t.Fatal("Create should populate ID")
	}

	got, err := repo.GetByID(ctx, a.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("GetByID returned nil")
	}
	if got.AscentType != "send" {
		t.Errorf("AscentType = %q, want %q", got.AscentType, "send")
	}
	if got.Attempts != 3 {
		t.Errorf("Attempts = %d, want 3", got.Attempts)
	}
}

func TestAscentRepo_Update(t *testing.T) {
	pool := testDB(t)
	repo := NewAscentRepo(pool)
	ctx := context.Background()
	_, _, routeID, userID := seedAscentFixture(t, pool, ctx)

	a := &model.Ascent{
		UserID: userID, RouteID: routeID, AscentType: "attempt", Attempts: 1, ClimbedAt: time.Now(),
	}
	repo.Create(ctx, a)

	notes := "Fell on the crux"
	a.AscentType = "send"
	a.Attempts = 5
	a.Notes = &notes
	if err := repo.Update(ctx, a); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, _ := repo.GetByID(ctx, a.ID)
	if got.AscentType != "send" {
		t.Errorf("AscentType = %q, want %q", got.AscentType, "send")
	}
	if got.Attempts != 5 {
		t.Errorf("Attempts = %d, want 5", got.Attempts)
	}
}

func TestAscentRepo_Delete_OwnerOnly(t *testing.T) {
	pool := testDB(t)
	repo := NewAscentRepo(pool)
	ctx := context.Background()
	_, _, routeID, userID := seedAscentFixture(t, pool, ctx)

	a := &model.Ascent{
		UserID: userID, RouteID: routeID, AscentType: "flash", Attempts: 1, ClimbedAt: time.Now(),
	}
	repo.Create(ctx, a)

	// Wrong user can't delete
	err := repo.Delete(ctx, a.ID, "wrong-user-id")
	if err == nil {
		t.Error("Delete should fail for non-owner")
	}

	// Owner can delete
	if err := repo.Delete(ctx, a.ID, userID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, _ := repo.GetByID(ctx, a.ID)
	if got != nil {
		t.Error("Deleted ascent should not be returned")
	}
}

func TestAscentRepo_RouteAscentStatus(t *testing.T) {
	pool := testDB(t)
	repo := NewAscentRepo(pool)
	ctx := context.Background()
	_, _, routeID, userID := seedAscentFixture(t, pool, ctx)

	// No ascents yet
	hasAny, hasCompleted, err := repo.RouteAscentStatus(ctx, userID, routeID)
	if err != nil {
		t.Fatalf("RouteAscentStatus: %v", err)
	}
	if hasAny || hasCompleted {
		t.Error("Should have no ascents initially")
	}

	// Log an attempt
	repo.Create(ctx, &model.Ascent{
		UserID: userID, RouteID: routeID, AscentType: "attempt", Attempts: 1, ClimbedAt: time.Now(),
	})

	hasAny, hasCompleted, _ = repo.RouteAscentStatus(ctx, userID, routeID)
	if !hasAny {
		t.Error("Should have ascent after attempt")
	}
	if hasCompleted {
		t.Error("Attempt should not count as completed")
	}

	// Log a send
	repo.Create(ctx, &model.Ascent{
		UserID: userID, RouteID: routeID, AscentType: "send", Attempts: 3, ClimbedAt: time.Now(),
	})

	hasAny, hasCompleted, _ = repo.RouteAscentStatus(ctx, userID, routeID)
	if !hasAny || !hasCompleted {
		t.Error("Should have both any and completed after send")
	}
}

func TestAscentRepo_UserStats(t *testing.T) {
	pool := testDB(t)
	repo := NewAscentRepo(pool)
	ctx := context.Background()
	_, _, routeID, userID := seedAscentFixture(t, pool, ctx)

	// Create a second route for variety
	var route2ID string
	pool.QueryRow(ctx,
		`INSERT INTO routes (location_id, wall_id, route_type, status, grading_system, grade, color, date_set)
		 VALUES ((SELECT location_id FROM routes WHERE id = $1),
		         (SELECT wall_id FROM routes WHERE id = $1),
		         'boulder', 'active', 'v_scale', 'V3', '#00FF00', CURRENT_DATE) RETURNING id`,
		routeID,
	).Scan(&route2ID)

	// Log ascents
	repo.Create(ctx, &model.Ascent{UserID: userID, RouteID: routeID, AscentType: "flash", Attempts: 1, ClimbedAt: time.Now()})
	repo.Create(ctx, &model.Ascent{UserID: userID, RouteID: route2ID, AscentType: "send", Attempts: 5, ClimbedAt: time.Now()})

	stats, err := repo.UserStats(ctx, userID)
	if err != nil {
		t.Fatalf("UserStats: %v", err)
	}
	if stats.TotalSends != 2 { // flash counts as send
		t.Errorf("TotalSends = %d, want 2", stats.TotalSends)
	}
	if stats.TotalFlashes != 1 {
		t.Errorf("TotalFlashes = %d, want 1", stats.TotalFlashes)
	}
	if stats.UniqueRoutes != 2 {
		t.Errorf("UniqueRoutes = %d, want 2", stats.UniqueRoutes)
	}
	if len(stats.GradePyramid) < 2 {
		t.Errorf("GradePyramid entries = %d, want >= 2", len(stats.GradePyramid))
	}
}
