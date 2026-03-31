package repository

import (
	"context"
	"testing"

	"github.com/shotwell-paddle/routewerk/internal/model"
)

// ── helpers to create prerequisite data ─────────────────────────────

func seedOrgAndLocation(t *testing.T, repo *SettingsRepo, ctx context.Context) (orgID, locationID string) {
	t.Helper()

	// Create an org
	err := repo.db.QueryRow(ctx,
		`INSERT INTO organizations (name, slug) VALUES ($1, $2) RETURNING id`,
		"Test Gym Co", "test-gym-co",
	).Scan(&orgID)
	if err != nil {
		t.Fatalf("create test org: %v", err)
	}

	// Create a location
	err = repo.db.QueryRow(ctx,
		`INSERT INTO locations (org_id, name, slug, timezone) VALUES ($1, $2, $3, $4) RETURNING id`,
		orgID, "Test Location", "test-location", "America/Denver",
	).Scan(&locationID)
	if err != nil {
		t.Fatalf("create test location: %v", err)
	}

	return orgID, locationID
}

func seedUser(t *testing.T, repo *SettingsRepo, ctx context.Context) string {
	t.Helper()
	var userID string
	err := repo.db.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, display_name) VALUES ($1, $2, $3) RETURNING id`,
		"settings-test@example.com", "$2a$10$fakehash", "Settings Tester",
	).Scan(&userID)
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	return userID
}

// ── Location Settings ──────────────────────────────────────────────

func TestSettingsRepo_LocationSettings_DefaultsWhenEmpty(t *testing.T) {
	pool := testDB(t)
	repo := NewSettingsRepo(pool)
	ctx := context.Background()

	_, locationID := seedOrgAndLocation(t, repo, ctx)

	settings, err := repo.GetLocationSettings(ctx, locationID)
	if err != nil {
		t.Fatalf("GetLocationSettings: %v", err)
	}

	defaults := model.DefaultLocationSettings()
	if settings.Grading.BoulderMethod != defaults.Grading.BoulderMethod {
		t.Errorf("BoulderMethod = %q, want %q", settings.Grading.BoulderMethod, defaults.Grading.BoulderMethod)
	}
	if settings.Grading.RouteGradeFormat != defaults.Grading.RouteGradeFormat {
		t.Errorf("RouteGradeFormat = %q, want %q", settings.Grading.RouteGradeFormat, defaults.Grading.RouteGradeFormat)
	}
}

func TestSettingsRepo_LocationSettings_UpdateAndRead(t *testing.T) {
	pool := testDB(t)
	repo := NewSettingsRepo(pool)
	ctx := context.Background()

	_, locationID := seedOrgAndLocation(t, repo, ctx)

	custom := model.DefaultLocationSettings()
	custom.Grading.BoulderMethod = "circuit"
	custom.Grading.RouteGradeFormat = "letter"
	custom.Display.DefaultStripAgeDays = 60

	if err := repo.UpdateLocationSettings(ctx, locationID, custom); err != nil {
		t.Fatalf("UpdateLocationSettings: %v", err)
	}

	got, err := repo.GetLocationSettings(ctx, locationID)
	if err != nil {
		t.Fatalf("GetLocationSettings after update: %v", err)
	}
	if got.Grading.BoulderMethod != "circuit" {
		t.Errorf("BoulderMethod = %q, want %q", got.Grading.BoulderMethod, "circuit")
	}
	if got.Grading.RouteGradeFormat != "letter" {
		t.Errorf("RouteGradeFormat = %q, want %q", got.Grading.RouteGradeFormat, "letter")
	}
	if got.Display.DefaultStripAgeDays != 60 {
		t.Errorf("DefaultStripAgeDays = %d, want 60", got.Display.DefaultStripAgeDays)
	}
}

func TestSettingsRepo_LocationSettings_NonexistentLocation(t *testing.T) {
	pool := testDB(t)
	repo := NewSettingsRepo(pool)
	ctx := context.Background()

	_, err := repo.GetLocationSettings(ctx, "nonexistent-id")
	if err == nil {
		t.Error("GetLocationSettings should return error for nonexistent location")
	}
}

// ── Organization Settings ──────────────────────────────────────────

func TestSettingsRepo_OrgSettings_DefaultsWhenEmpty(t *testing.T) {
	pool := testDB(t)
	repo := NewSettingsRepo(pool)
	ctx := context.Background()

	orgID, _ := seedOrgAndLocation(t, repo, ctx)

	settings, err := repo.GetOrgSettings(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgSettings: %v", err)
	}

	defaults := model.DefaultOrgSettings()
	if settings.Permissions.HeadSetterCanEditGrading != defaults.Permissions.HeadSetterCanEditGrading {
		t.Error("default org permissions should match")
	}
}

func TestSettingsRepo_OrgSettings_UpdateAndRead(t *testing.T) {
	pool := testDB(t)
	repo := NewSettingsRepo(pool)
	ctx := context.Background()

	orgID, _ := seedOrgAndLocation(t, repo, ctx)

	custom := model.DefaultOrgSettings()
	custom.Permissions.HeadSetterCanEditGrading = false
	custom.Defaults.BoulderMethod = "circuit"

	if err := repo.UpdateOrgSettings(ctx, orgID, custom); err != nil {
		t.Fatalf("UpdateOrgSettings: %v", err)
	}

	got, err := repo.GetOrgSettings(ctx, orgID)
	if err != nil {
		t.Fatalf("GetOrgSettings after update: %v", err)
	}
	if got.Permissions.HeadSetterCanEditGrading {
		t.Error("HeadSetterCanEditGrading should be false after update")
	}
	if got.Defaults.BoulderMethod != "circuit" {
		t.Errorf("BoulderMethod = %q, want %q", got.Defaults.BoulderMethod, "circuit")
	}
}

// ── User Settings ──────────────────────────────────────────────────

func TestSettingsRepo_UserSettings_DefaultsWhenEmpty(t *testing.T) {
	pool := testDB(t)
	repo := NewSettingsRepo(pool)
	ctx := context.Background()

	userID := seedUser(t, repo, ctx)

	settings, err := repo.GetUserSettings(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserSettings: %v", err)
	}

	if !settings.Privacy.ShowProfile {
		t.Error("ShowProfile should default to true")
	}
}

func TestSettingsRepo_UserSettings_UpdateAndRead(t *testing.T) {
	pool := testDB(t)
	repo := NewSettingsRepo(pool)
	ctx := context.Background()

	userID := seedUser(t, repo, ctx)

	custom := model.UserSettings{
		Privacy: model.PrivacySettings{
			ShowProfile:       false,
			ShowTickList:      false,
			ShowStats:         true,
			ShowOnLeaderboard: false,
		},
	}

	if err := repo.UpdateUserSettings(ctx, userID, custom); err != nil {
		t.Fatalf("UpdateUserSettings: %v", err)
	}

	got, err := repo.GetUserSettings(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserSettings after update: %v", err)
	}
	if got.Privacy.ShowProfile {
		t.Error("ShowProfile should be false after update")
	}
	if got.Privacy.ShowTickList {
		t.Error("ShowTickList should be false after update")
	}
	if !got.Privacy.ShowStats {
		t.Error("ShowStats should be true after update")
	}
}
