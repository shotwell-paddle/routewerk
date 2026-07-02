//go:build integration

package repository

import (
	"context"
	"testing"
)

// TestAnalyticsRepo_OrgOverview seeds a deliberately fan-out-prone shape —
// multiple ascents per route alongside multiple overdue routes at the same
// location — to pin down that the pre-aggregated rewrite keeps the exact
// semantics of the old triple-join query.
func TestAnalyticsRepo_OrgOverview(t *testing.T) {
	pool := testDB(t)
	repo := NewAnalyticsRepo(pool)
	ctx := context.Background()

	var orgID string
	pool.QueryRow(ctx,
		`INSERT INTO organizations (name, slug) VALUES ($1, $2) RETURNING id`,
		"Overview Org", "overview-org",
	).Scan(&orgID)

	var locA, locB string
	pool.QueryRow(ctx,
		`INSERT INTO locations (org_id, name, slug, timezone) VALUES ($1, $2, $3, $4) RETURNING id`,
		orgID, "Alpha Gym", "alpha-gym", "UTC",
	).Scan(&locA)
	pool.QueryRow(ctx,
		`INSERT INTO locations (org_id, name, slug, timezone) VALUES ($1, $2, $3, $4) RETURNING id`,
		orgID, "Beta Gym", "beta-gym", "UTC",
	).Scan(&locB)

	var wallID string
	pool.QueryRow(ctx,
		`INSERT INTO walls (location_id, name, wall_type, sort_order) VALUES ($1, $2, $3, $4) RETURNING id`,
		locA, "Overview Wall", "boulder", 1,
	).Scan(&wallID)

	// Alpha Gym: two active routes (both overdue), one archived, one
	// soft-deleted. Expect active_routes=2, overdue_strips=2.
	newRoute := func(status string, overdue, deleted bool) string {
		t.Helper()
		strip := "NULL"
		if overdue {
			strip = "CURRENT_DATE - 1"
		}
		deletedAt := "NULL"
		if deleted {
			deletedAt = "NOW()"
		}
		var id string
		if err := pool.QueryRow(ctx,
			`INSERT INTO routes (location_id, wall_id, route_type, status, grading_system, grade, color, date_set, projected_strip_date, deleted_at)
			 VALUES ($1, $2, 'boulder', $3, 'v_scale', 'V3', '#000', CURRENT_DATE - 10, `+strip+`, `+deletedAt+`) RETURNING id`,
			locA, wallID, status,
		).Scan(&id); err != nil {
			t.Fatalf("seed route: %v", err)
		}
		return id
	}
	active1 := newRoute("active", true, false)
	active2 := newRoute("active", true, false)
	archived := newRoute("archived", false, false)
	// Soft-deleted: excluded from active_routes and overdue_strips, but its
	// ascents still count toward active_climbers_30d (the pre-rewrite query
	// joined ascents through ALL routes at the location, deleted included).
	deleted := newRoute("active", true, true)

	newUser := func(email string) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx,
			`INSERT INTO users (email, password_hash, display_name) VALUES ($1, $2, $3) RETURNING id`,
			email, "$2a$10$fakehash", "Overview Climber",
		).Scan(&id); err != nil {
			t.Fatalf("seed user: %v", err)
		}
		return id
	}
	climber1 := newUser("c1@overview-test.com")
	climber2 := newUser("c2@overview-test.com")
	climber3 := newUser("c3@overview-test.com")
	climber4 := newUser("c4@overview-test.com")

	logAscent := func(userID, routeID, ascentType, age string) {
		t.Helper()
		if _, err := pool.Exec(ctx,
			`INSERT INTO ascents (user_id, route_id, ascent_type, climbed_at)
			 VALUES ($1, $2, $3, NOW() - $4::interval)`,
			userID, routeID, ascentType, age,
		); err != nil {
			t.Fatalf("seed ascent: %v", err)
		}
	}
	// climber1: three recent ascents across two routes — must count once.
	// (Only one send per user+route is allowed, hence the attempt.)
	logAscent(climber1, active1, "send", "1 day")
	logAscent(climber1, active1, "attempt", "2 days")
	logAscent(climber1, active2, "send", "3 days")
	// climber2: recent ascent on an archived route — still an active climber
	// (the old query joined ascents through ALL routes at the location).
	logAscent(climber2, archived, "send", "4 days")
	// climber3: outside the 30-day window — must not count.
	logAscent(climber3, active1, "send", "40 days")
	// climber4: recent ascent on the soft-deleted route — still an active
	// climber, mirroring the old query's unfiltered routes join. A "cleanup"
	// that adds deleted_at IS NULL to the climbers subquery must fail here.
	logAscent(climber4, deleted, "send", "5 days")

	overview, err := repo.OrgOverview(ctx, orgID)
	if err != nil {
		t.Fatalf("OrgOverview: %v", err)
	}
	if len(overview) != 2 {
		t.Fatalf("OrgOverview returned %d locations, want 2", len(overview))
	}

	tests := []struct {
		name              string
		got               LocationOverview
		wantName          string
		wantActiveRoutes  int
		wantClimbers      int
		wantOverdueStrips int
	}{
		{"busy location", overview[0], "Alpha Gym", 2, 3, 2},
		{"empty location", overview[1], "Beta Gym", 0, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got.LocationName != tt.wantName {
				t.Errorf("LocationName = %q, want %q", tt.got.LocationName, tt.wantName)
			}
			if tt.got.ActiveRoutes != tt.wantActiveRoutes {
				t.Errorf("ActiveRoutes = %d, want %d", tt.got.ActiveRoutes, tt.wantActiveRoutes)
			}
			if tt.got.ActiveClimbers30d != tt.wantClimbers {
				t.Errorf("ActiveClimbers30d = %d, want %d", tt.got.ActiveClimbers30d, tt.wantClimbers)
			}
			if tt.got.OverdueStrips != tt.wantOverdueStrips {
				t.Errorf("OverdueStrips = %d, want %d", tt.got.OverdueStrips, tt.wantOverdueStrips)
			}
		})
	}
}
