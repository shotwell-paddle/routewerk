package database

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect opens a connection pool to PostgreSQL. In non-development
// environments, it refuses to connect without TLS (sslmode=disable is
// rejected and the absence of an sslmode parameter defaults to require).
// PoolConfig holds tunable connection pool parameters.
type PoolConfig struct {
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

func Connect(databaseURL string, isDev bool, pool ...PoolConfig) (*pgxpool.Pool, error) {
	// On Fly.io, Postgres is accessed over an encrypted WireGuard tunnel
	// (*.flycast) so TLS at the Postgres layer is unnecessary and unsupported.
	onFlyNetwork := os.Getenv("FLY_APP_NAME") != "" &&
		strings.Contains(databaseURL, ".flycast")
	if !isDev && !onFlyNetwork {
		databaseURL = enforceTLS(databaseURL)
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}

	// Apply pool settings from caller, or use sensible defaults.
	pc := PoolConfig{MaxConns: 10, MinConns: 2, MaxConnLifetime: 1 * time.Hour, MaxConnIdleTime: 30 * time.Minute}
	if len(pool) > 0 {
		pc = pool[0]
	}
	config.MaxConns = pc.MaxConns
	config.MinConns = pc.MinConns
	config.MaxConnLifetime = pc.MaxConnLifetime
	config.MaxConnIdleTime = pc.MaxConnIdleTime

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return pool, nil
}

// enforceTLS ensures the connection string requires TLS. If sslmode is
// missing it adds sslmode=require. If sslmode=disable is present it
// upgrades to sslmode=require.
func enforceTLS(connStr string) string {
	lower := strings.ToLower(connStr)

	if strings.Contains(lower, "sslmode=disable") {
		// Replace disable → require
		return replaceParam(connStr, "sslmode=disable", "sslmode=require")
	}

	if !strings.Contains(lower, "sslmode=") {
		// No sslmode at all — append it
		sep := "?"
		if strings.Contains(connStr, "?") {
			sep = "&"
		}
		return connStr + sep + "sslmode=require"
	}

	return connStr
}

func replaceParam(s, old, replacement string) string {
	lower := strings.ToLower(s)
	idx := strings.Index(lower, strings.ToLower(old))
	if idx < 0 {
		return s
	}
	return s[:idx] + replacement + s[idx+len(old):]
}
