package repository

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// testDB returns a *pgxpool.Pool connected to a test database.
// It skips the test if DATABASE_URL is not set (no test DB available).
// Each test gets a fresh schema via a unique search_path, so tests
// don't interfere with each other.
//
// Usage:
//
//	func TestSomething(t *testing.T) {
//	    pool := testDB(t)
//	    repo := NewRouteRepo(pool)
//	    // ... test repo methods
//	}
func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dbURL := os.Getenv("ROUTEWERK_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("ROUTEWERK_TEST_DATABASE_URL not set — skipping integration test")
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect to test DB: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping test DB: %v", err)
	}

	// Create an isolated schema for this test to avoid interference.
	schema := fmt.Sprintf("test_%s_%d", sanitizeTestName(t.Name()), os.Getpid())
	_, err = pool.Exec(ctx, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schema))
	if err != nil {
		pool.Close()
		t.Fatalf("drop schema: %v", err)
	}
	_, err = pool.Exec(ctx, fmt.Sprintf("CREATE SCHEMA %s", schema))
	if err != nil {
		pool.Close()
		t.Fatalf("create schema: %v", err)
	}

	// Set the search_path so all queries use this schema
	_, err = pool.Exec(ctx, fmt.Sprintf("SET search_path TO %s, public", schema))
	if err != nil {
		pool.Close()
		t.Fatalf("set search_path: %v", err)
	}

	// Run migrations
	if err := runMigrations(ctx, pool, schema); err != nil {
		pool.Close()
		t.Fatalf("run migrations: %v", err)
	}

	t.Cleanup(func() {
		pool.Exec(context.Background(), fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schema))
		pool.Close()
	})

	return pool
}

// runMigrations applies all .up.sql files in order.
func runMigrations(ctx context.Context, pool *pgxpool.Pool, schema string) error {
	migrationsDir := findMigrationsDir()
	if migrationsDir == "" {
		return fmt.Errorf("migrations directory not found")
	}

	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	// Collect and sort .up.sql files
	var upFiles []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".up.sql") {
			upFiles = append(upFiles, filepath.Join(migrationsDir, e.Name()))
		}
	}
	sort.Strings(upFiles)

	for _, f := range upFiles {
		sql, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("read %s: %w", filepath.Base(f), err)
		}

		// Execute the migration SQL
		_, err = pool.Exec(ctx, string(sql))
		if err != nil {
			return fmt.Errorf("execute %s: %w", filepath.Base(f), err)
		}
	}

	return nil
}

// findMigrationsDir walks up from the test file to find the migrations directory.
func findMigrationsDir() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}

	dir := filepath.Dir(filename)
	// Walk up to find internal/database/migrations
	for i := 0; i < 5; i++ {
		candidate := filepath.Join(dir, "database", "migrations")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		candidate = filepath.Join(dir, "internal", "database", "migrations")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		dir = filepath.Dir(dir)
	}
	return ""
}

func sanitizeTestName(name string) string {
	// Replace non-alphanumeric characters with underscores for schema names
	result := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, name)
	// Truncate to a reasonable length for schema name (max 63 chars in PG)
	if len(result) > 40 {
		result = result[:40]
	}
	return strings.ToLower(result)
}
