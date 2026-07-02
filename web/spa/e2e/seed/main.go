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
//	E2E_DATABASE_URL=postgres://.../routewerk_e2e go run ./web/spa/e2e/seed
//
// The env var is deliberately E2E_DATABASE_URL, not DATABASE_URL: dev
// shells routinely export DATABASE_URL for `make run` (and admin shells
// may carry a production DSN in that exact variable), and this program
// writes rows. As a second guard the database name in the DSN must
// contain "e2e".
//
// Idempotent: if the fixture user already exists AND the fixture is
// complete (setter membership + wall present), it exits 0 without
// touching anything. A partial fixture (e.g. a prior run died midway)
// is an error — drop and recreate the scratch DB.
package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

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
	dbURL := os.Getenv("E2E_DATABASE_URL")
	if dbURL == "" {
		log.Fatal("seed: E2E_DATABASE_URL is required (deliberately not DATABASE_URL — this program writes rows and must never point at a dev/prod database)")
	}
	requireScratchDB(dbURL)

	ctx := context.Background()
	// isDev=true skips TLS enforcement — correct for local/CI scratch
	// targets, which are the only thing this program may connect to.
	db, err := database.Connect(dbURL, true)
	if err != nil {
		log.Fatalf("seed: connect: %v", err)
	}
	defer db.Close()

	userRepo := repository.NewUserRepo(db)
	orgRepo := repository.NewOrgRepo(db)
	locationRepo := repository.NewLocationRepo(db)
	wallRepo := repository.NewWallRepo(db)
	settingsRepo := repository.NewSettingsRepo(db)

	// Idempotency guard. The user row is created second-to-last (see the
	// creation order below), so its existence means a prior run got at
	// least that far — but not necessarily to the end. Verify the parts
	// the smoke test depends on (setter membership at a location + the
	// wall) and refuse to proceed on a partial fixture rather than let
	// Playwright fail with an opaque timeout.
	existing, err := userRepo.GetByEmail(ctx, seedEmail)
	if err != nil {
		log.Fatalf("seed: lookup user: %v", err)
	}
	if existing != nil {
		if fixtureComplete(ctx, userRepo, wallRepo, existing.ID) {
			fmt.Printf("seed: fixture already present (%s), nothing to do\n", seedEmail)
			return
		}
		log.Fatalf("seed: partial fixture detected (user %s exists but membership/wall are missing) — drop and recreate the scratch DB (DROP DATABASE routewerk_e2e) and re-run", seedEmail)
	}

	// Creation order: org → location → settings → wall → user →
	// membership. The user comes second-to-last so the guard above
	// ("user exists ⇒ nearly everything exists") holds as tightly as a
	// non-transactional seeder can make it.
	org := &model.Organization{Name: seedOrgName, Slug: seedOrgSlug}
	if err := orgRepo.Create(ctx, org); err != nil {
		log.Fatalf("seed: create org: %v", err)
	}

	loc := &model.Location{OrgID: org.ID, Name: seedGymName, Slug: seedGymSlug, Timezone: "UTC"}
	if err := locationRepo.Create(ctx, loc); err != nil {
		log.Fatalf("seed: create location: %v", err)
	}

	// Write the default settings explicitly so the fixture owns its hold-
	// color palette — the smoke test clicks the "Blue" swatch and must not
	// depend on what the server happens to default to for a settings-less
	// location.
	if err := settingsRepo.UpdateLocationSettings(ctx, loc.ID, model.DefaultLocationSettings()); err != nil {
		log.Fatalf("seed: write location settings: %v", err)
	}

	wall := &model.Wall{LocationID: loc.ID, Name: seedWallName, WallType: "boulder", SortOrder: 1}
	if err := wallRepo.Create(ctx, wall); err != nil {
		log.Fatalf("seed: create wall: %v", err)
	}

	hash, err := auth.HashPassword(seedPassword)
	if err != nil {
		log.Fatalf("seed: hash password: %v", err)
	}
	user := &model.User{Email: seedEmail, PasswordHash: hash, DisplayName: seedDisplayName}
	if err := userRepo.Create(ctx, user); err != nil {
		log.Fatalf("seed: create user: %v", err)
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

	fmt.Printf("seed: created %s (setter at %q, wall %q)\n", seedEmail, loc.Name, wall.Name)
}

// fixtureComplete reports whether the parts of the fixture the smoke test
// depends on exist for the given user: a location-scoped membership, and
// the named wall at that location.
func fixtureComplete(ctx context.Context, userRepo *repository.UserRepo, wallRepo *repository.WallRepo, userID string) bool {
	memberships, err := userRepo.GetMemberships(ctx, userID)
	if err != nil {
		log.Fatalf("seed: list memberships: %v", err)
	}
	for _, m := range memberships {
		if m.LocationID == nil {
			continue
		}
		walls, err := wallRepo.ListByLocation(ctx, *m.LocationID)
		if err != nil {
			log.Fatalf("seed: list walls: %v", err)
		}
		for _, w := range walls {
			if w.Name == seedWallName {
				return true
			}
		}
	}
	return false
}

// requireScratchDB parses the DSN and refuses to run unless the database
// name contains "e2e" — belt-and-brace against a dev/prod DSN finding its
// way into E2E_DATABASE_URL.
func requireScratchDB(dsn string) {
	u, err := url.Parse(dsn)
	if err != nil {
		log.Fatalf("seed: cannot parse E2E_DATABASE_URL: %v", err)
	}
	dbName := strings.TrimPrefix(u.Path, "/")
	if !strings.Contains(strings.ToLower(dbName), "e2e") {
		log.Fatalf("seed: refusing to run against database %q — the database name must contain \"e2e\" (use a scratch DB like routewerk_e2e)", dbName)
	}
}
