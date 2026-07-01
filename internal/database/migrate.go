package database

import (
	"embed"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate runs all pending database migrations. It uses the embedded SQL
// files in the migrations/ directory and applies them in order.
// The databaseURL is a standard PostgreSQL connection string.
func Migrate(databaseURL string) error {
	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("create migration source: %w", err)
	}

	// golang-migrate's pgx5 driver expects a "pgx5://" scheme
	pgxURL := toPgx5URL(databaseURL)

	m, err := migrate.NewWithSourceInstance("iofs", source, pgxURL)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}
	defer m.Close()

	// A dirty state means a migration failed halfway. Recovering requires
	// forcing the version back so golang-migrate will re-attempt it, but
	// doing that automatically at startup is unattended recovery against a
	// half-applied schema — dangerous even though our migrations are written
	// to be idempotent (IF NOT EXISTS, IF EXISTS, etc.). It is therefore
	// gated behind ALLOW_DIRTY_MIGRATION_FORCE=true (default false): without
	// the opt-in we refuse to start so an operator can inspect the schema
	// and run `admin migrate-force N` deliberately.
	version, dirty, verr := m.Version()
	if verr == nil && dirty {
		prev := int(version) - 1
		if prev < 0 {
			prev = -1 // special value: no version applied
		}
		if !allowDirtyForce() {
			return fmt.Errorf(
				"database is dirty at migration version %d (a migration failed halfway); "+
					"refusing to auto-recover — inspect the schema, then run `admin migrate-force %d` "+
					"to retry it (or set ALLOW_DIRTY_MIGRATION_FORCE=true to opt in to automatic recovery)",
				version, prev)
		}
		slog.Warn("dirty migration detected, auto-recovering (ALLOW_DIRTY_MIGRATION_FORCE=true)",
			"dirty_version", version, "forcing_to", prev)
		if ferr := m.Force(prev); ferr != nil {
			return fmt.Errorf("auto-recover dirty migration %d: %w", version, ferr)
		}
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("run migrations: %w", err)
	}

	version, dirty, _ = m.Version()
	if dirty {
		return fmt.Errorf("migration version %d is dirty after retry — check the SQL", version)
	}
	slog.Info("migrations applied", "version", version)

	return nil
}

// MigrateDown rolls back the last applied migration.
func MigrateDown(databaseURL string) error {
	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("create migration source: %w", err)
	}

	pgxURL := toPgx5URL(databaseURL)

	m, err := migrate.NewWithSourceInstance("iofs", source, pgxURL)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}
	defer m.Close()

	if err := m.Steps(-1); err != nil {
		return fmt.Errorf("rollback migration: %w", err)
	}

	version, _, _ := m.Version()
	slog.Info("rolled back to", "version", version)
	return nil
}

// MigrateForce sets the migration version without running the migration and
// clears the dirty flag. Use this to recover from a failed migration.
func MigrateForce(databaseURL string, version int) error {
	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("create migration source: %w", err)
	}

	pgxURL := toPgx5URL(databaseURL)

	m, err := migrate.NewWithSourceInstance("iofs", source, pgxURL)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}
	defer m.Close()

	if err := m.Force(version); err != nil {
		return fmt.Errorf("force version: %w", err)
	}

	slog.Info("forced migration version", "version", version)
	return nil
}

// MigrateVersion returns the current migration version and dirty state.
func MigrateVersion(databaseURL string) (uint, bool, error) {
	source, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return 0, false, fmt.Errorf("create migration source: %w", err)
	}

	pgxURL := toPgx5URL(databaseURL)

	m, err := migrate.NewWithSourceInstance("iofs", source, pgxURL)
	if err != nil {
		return 0, false, fmt.Errorf("create migrator: %w", err)
	}
	defer m.Close()

	return m.Version()
}

// allowDirtyForce reports whether the operator has explicitly opted in to
// automatic dirty-state recovery via ALLOW_DIRTY_MIGRATION_FORCE=true.
// Anything unset or unparseable means false — never force unattended.
func allowDirtyForce() bool {
	b, err := strconv.ParseBool(os.Getenv("ALLOW_DIRTY_MIGRATION_FORCE"))
	return err == nil && b
}

// toPgx5URL converts a postgres:// or postgresql:// URL to the pgx5://
// scheme that golang-migrate's pgx5 driver expects.
func toPgx5URL(url string) string {
	if len(url) >= 11 && url[:11] == "postgresql:" {
		return "pgx5:" + url[11:]
	}
	if len(url) >= 9 && url[:9] == "postgres:" {
		return "pgx5:" + url[9:]
	}
	return url
}
