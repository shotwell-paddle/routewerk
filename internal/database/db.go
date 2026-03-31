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
//
// Fly.io tuning notes:
//   - Shared Postgres allows ~25 connections. With 2 app instances, keep
//     MaxConns at 5 per instance (10 total + headroom for migrations).
//   - MinConns = 1 keeps one warm connection while not wasting resources
//     on idle shared-CPU VMs.
//   - MaxConnLifetime = 30min prevents stale connections after Fly proxy
//     restarts (their internal proxy can silently drop connections).
//   - MaxConnIdleTime = 5min aggressively reclaims idle connections since
//     Fly charges per-connection memory on shared Postgres.
//   - HealthCheckPeriod = 30s catches dead connections from Fly proxy
//     restarts before they cause user-facing errors.
type PoolConfig struct {
	MaxConns          int32
	MinConns          int32
	MaxConnLifetime   time.Duration
	MaxConnIdleTime   time.Duration
	HealthCheckPeriod time.Duration
}

// DefaultPoolConfig returns conservative defaults suitable for Fly.io
// shared Postgres. Override via DB_MAX_CONNS, DB_MIN_CONNS, etc. env vars.
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxConns:          5,
		MinConns:          1,
		MaxConnLifetime:   30 * time.Minute,
		MaxConnIdleTime:   5 * time.Minute,
		HealthCheckPeriod: 30 * time.Second,
	}
}

func Connect(databaseURL string, isDev bool, poolCfg ...PoolConfig) (*pgxpool.Pool, error) {
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
	// Fly.io shared Postgres typically allows ~25 connections total. With
	// 2 app instances each running a job queue worker, we default to 5 max
	// conns per instance to stay well within limits and leave headroom for
	// admin connections and migrations.
	pc := DefaultPoolConfig()
	if len(poolCfg) > 0 {
		pc = poolCfg[0]
	}
	config.MaxConns = pc.MaxConns
	config.MinConns = pc.MinConns
	config.MaxConnLifetime = pc.MaxConnLifetime
	config.MaxConnIdleTime = pc.MaxConnIdleTime
	config.HealthCheckPeriod = pc.HealthCheckPeriod

	// Attach slow-query tracer — logs any query exceeding SlowQueryThreshold.
	config.ConnConfig.Tracer = &queryTracer{}

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
