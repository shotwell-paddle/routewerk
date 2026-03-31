package repository

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestMigrations_UpDown verifies every migration can be applied (up) and
// rolled back (down) cleanly. This catches syntax errors in down migrations
// and ensures rollbacks don't leave orphaned objects.
func TestMigrations_UpDown(t *testing.T) {
	dbURL := os.Getenv("ROUTEWERK_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("ROUTEWERK_TEST_DATABASE_URL not set — skipping migration test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	schema := "test_migration_updown"
	pool.Exec(ctx, "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
	_, err = pool.Exec(ctx, "CREATE SCHEMA "+schema)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}
	defer pool.Exec(ctx, "DROP SCHEMA IF EXISTS "+schema+" CASCADE")

	_, err = pool.Exec(ctx, "SET search_path TO "+schema+", public")
	if err != nil {
		t.Fatalf("set search_path: %v", err)
	}

	migrationsDir := findMigrationsDir()
	if migrationsDir == "" {
		t.Fatal("migrations directory not found")
	}

	// Collect migration pairs
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}

	type migrationPair struct {
		version string
		upFile  string
		downFile string
	}

	pairMap := make(map[string]*migrationPair)
	for _, e := range entries {
		name := e.Name()
		version := strings.Split(name, "_")[0] // e.g., "000001"
		if _, ok := pairMap[version]; !ok {
			pairMap[version] = &migrationPair{version: version}
		}
		if strings.HasSuffix(name, ".up.sql") {
			pairMap[version].upFile = filepath.Join(migrationsDir, name)
		} else if strings.HasSuffix(name, ".down.sql") {
			pairMap[version].downFile = filepath.Join(migrationsDir, name)
		}
	}

	var pairs []*migrationPair
	for _, p := range pairMap {
		pairs = append(pairs, p)
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].version < pairs[j].version })

	// Verify every migration has both up and down files
	for _, p := range pairs {
		if p.upFile == "" {
			t.Errorf("migration %s: missing .up.sql", p.version)
		}
		if p.downFile == "" {
			t.Errorf("migration %s: missing .down.sql", p.version)
		}
	}

	// Apply all migrations up, one by one
	for _, p := range pairs {
		sql, err := os.ReadFile(p.upFile)
		if err != nil {
			t.Fatalf("read %s up: %v", p.version, err)
		}
		_, err = pool.Exec(ctx, string(sql))
		if err != nil {
			t.Fatalf("migration %s UP failed: %v", p.version, err)
		}
	}

	t.Log("All migrations applied UP successfully")

	// Roll back all migrations, in reverse order
	for i := len(pairs) - 1; i >= 0; i-- {
		p := pairs[i]
		sql, err := os.ReadFile(p.downFile)
		if err != nil {
			t.Fatalf("read %s down: %v", p.version, err)
		}
		_, err = pool.Exec(ctx, string(sql))
		if err != nil {
			t.Fatalf("migration %s DOWN failed: %v", p.version, err)
		}
	}

	t.Log("All migrations rolled back DOWN successfully")

	// Re-apply all migrations to verify clean round-trip
	for _, p := range pairs {
		sql, err := os.ReadFile(p.upFile)
		if err != nil {
			t.Fatalf("read %s up (re-apply): %v", p.version, err)
		}
		_, err = pool.Exec(ctx, string(sql))
		if err != nil {
			t.Fatalf("migration %s UP (re-apply) failed: %v", p.version, err)
		}
	}

	t.Log("All migrations re-applied UP after rollback — clean round-trip")
}

// TestMigrations_AllPaired ensures every .up.sql has a corresponding .down.sql.
// This test runs without a database.
func TestMigrations_AllPaired(t *testing.T) {
	migrationsDir := findMigrationsDir()
	if migrationsDir == "" {
		t.Fatal("migrations directory not found")
	}

	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}

	ups := make(map[string]bool)
	downs := make(map[string]bool)
	for _, e := range entries {
		name := e.Name()
		version := strings.Split(name, "_")[0]
		if strings.HasSuffix(name, ".up.sql") {
			ups[version] = true
		} else if strings.HasSuffix(name, ".down.sql") {
			downs[version] = true
		}
	}

	for v := range ups {
		if !downs[v] {
			t.Errorf("migration %s has .up.sql but no .down.sql", v)
		}
	}
	for v := range downs {
		if !ups[v] {
			t.Errorf("migration %s has .down.sql but no .up.sql", v)
		}
	}

	if len(ups) == 0 {
		t.Error("no migrations found")
	}
	t.Logf("Found %d migration pairs", len(ups))
}
