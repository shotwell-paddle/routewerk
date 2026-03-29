package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect opens a connection pool to PostgreSQL. In non-development
// environments, it refuses to connect without TLS (sslmode=disable is
// rejected and the absence of an sslmode parameter defaults to require).
func Connect(databaseURL string, isDev bool) (*pgxpool.Pool, error) {
	if !isDev {
		databaseURL = enforceTLS(databaseURL)
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}

	config.MaxConns = 25
	config.MinConns = 5
	config.MaxConnLifetime = 1 * time.Hour
	config.MaxConnIdleTime = 30 * time.Minute

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
