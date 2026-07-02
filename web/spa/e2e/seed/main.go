// Command seed provisions the minimal fixture for the Playwright smoke
// test (web/spa/e2e/smoke.spec.ts): one user with a setter membership at
// one location, plus one boulder wall to hang a route on.
//
// Test-only tooling — NOT part of the admin CLI surface. It reuses the
// production repositories and auth.HashPassword so the seeded credentials
// go through the exact bcrypt path the login form checks.
//
// Usage (against a SCRATCH database that migrations have already run on —
// the API server migrates on startup, so boot it first):
//
//	DATABASE_URL=postgres://... go run ./web/spa/e2e/seed
//
// Idempotent: if the fixture user already exists it exits 0 without
// touching anything, so re-runs against a persistent local scratch DB
// are safe.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/shotwell-paddle/routewerk/internal/auth"
	"github.com/shotwell-paddle/routewerk/internal/database"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

// Keep in sync with the constants at the top of web/spa/e2e/smoke.spec.ts.
const (
	seedEmail       = "e2e-setter@routewerk.test"
	seedPassword    = "e2e-smoke-password"
	seedDisplayName = "E2E Setter"
	seedOrgName     = "E2E Climbing"
	seedOrgSlug     = "e2e-climbing"
	seedGymName     = "E2E Gym"
	seedGymSlug     = "e2e-gym"
	seedWallName    = "E2E Wall"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("seed: DATABASE_URL is required")
	}

	ctx := context.Background()
	db, err := database.Connect(dbURL, true)
	if err != nil {
		log.Fatalf("seed: connect: %v", err)
	}
	defer db.Close()

	userRepo := repository.NewUserRepo(db)
	orgRepo := repository.NewOrgRepo(db)
	locationRepo := repository.NewLocationRepo(db)
	wallRepo := repository.NewWallRepo(db)

	// Idempotency guard: the user is created last-but-one, but checked
	// first — if they exist, a prior run completed far enough to log in.
	existing, err := userRepo.GetByEmail(ctx, seedEmail)
	if err != nil {
		log.Fatalf("seed: lookup user: %v", err)
	}
	if existing != nil {
		fmt.Printf("seed: fixture already present (%s), nothing to do\n", seedEmail)
		return
	}

	hash, err := auth.HashPassword(seedPassword)
	if err != nil {
		log.Fatalf("seed: hash password: %v", err)
	}
	user := &model.User{Email: seedEmail, PasswordHash: hash, DisplayName: seedDisplayName}
	if err := userRepo.Create(ctx, user); err != nil {
		log.Fatalf("seed: create user: %v", err)
	}

	org := &model.Organization{Name: seedOrgName, Slug: seedOrgSlug}
	if err := orgRepo.Create(ctx, org); err != nil {
		log.Fatalf("seed: create org: %v", err)
	}

	loc := &model.Location{OrgID: org.ID, Name: seedGymName, Slug: seedGymSlug, Timezone: "UTC"}
	if err := locationRepo.Create(ctx, loc); err != nil {
		log.Fatalf("seed: create location: %v", err)
	}

	// Location-scoped setter: rank 2 — can create routes, sees the staff
	// dashboard. Location-scoped (not org-wide) so the post-login default-
	// location pick and the SPA's location fallback both resolve to the gym.
	membership := &model.UserMembership{
		UserID:     user.ID,
		OrgID:      org.ID,
		LocationID: &loc.ID,
		Role:       "setter",
	}
	if err := orgRepo.AddMember(ctx, membership); err != nil {
		log.Fatalf("seed: add membership: %v", err)
	}

	wall := &model.Wall{LocationID: loc.ID, Name: seedWallName, WallType: "boulder", SortOrder: 1}
	if err := wallRepo.Create(ctx, wall); err != nil {
		log.Fatalf("seed: create wall: %v", err)
	}

	fmt.Printf("seed: created %s (setter at %q, wall %q)\n", seedEmail, loc.Name, wall.Name)
}
