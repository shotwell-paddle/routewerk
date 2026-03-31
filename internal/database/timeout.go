package database

import (
	"context"
	"time"
)

// Default query timeout categories. Handlers and repositories can use these
// to create context deadlines that match the expected cost of their queries.
//
// Usage:
//
//	ctx, cancel := database.QueryTimeout(ctx, database.TimeoutLong)
//	defer cancel()
//	rows, err := db.Query(ctx, heavyAnalyticsQuery, ...)
const (
	// TimeoutFast is for simple single-row lookups (e.g. GetByID, settings).
	TimeoutFast = 2 * time.Second

	// TimeoutDefault is the standard timeout for most queries.
	TimeoutDefault = 5 * time.Second

	// TimeoutLong is for complex analytics queries with joins and aggregations.
	TimeoutLong = 15 * time.Second

	// TimeoutBatch is for bulk operations (e.g. bulk archive, cleanup jobs).
	TimeoutBatch = 30 * time.Second
)

// QueryTimeout wraps a context with a deadline for the given timeout category.
// If the context already has a shorter deadline, the existing deadline is kept.
// Returns a cancel function that must be deferred.
func QueryTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining < timeout {
			// Existing deadline is tighter — don't extend it
			return context.WithCancel(ctx)
		}
	}
	return context.WithTimeout(ctx, timeout)
}
