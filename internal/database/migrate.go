package database

import (
	"embed"
	"fmt"
	"log/slog"

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

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("run migrations: %w", err)
	}

	version, dirty, _ := m.Version()
	if dirty {
		return fmt.Errorf("migration version %d is dirty — run `migrate force` to fix", version)
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
