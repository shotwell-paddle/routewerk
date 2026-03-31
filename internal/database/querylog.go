package database

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
)

// SlowQueryThreshold is the duration above which a query is logged as slow.
const SlowQueryThreshold = 500 * time.Millisecond

// queryTracer implements pgx.QueryTracer to log slow queries. It fires on
// every query completion, but only emits a log line when the query exceeds
// SlowQueryThreshold. This keeps the hot path cheap — just a clock read and
// comparison — while surfacing performance regressions in production logs.
type queryTracer struct{}

type traceQueryKey struct{}

type traceQueryData struct {
	startTime time.Time
	sql       string
	args      []any
}

func (t *queryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, traceQueryKey{}, &traceQueryData{
		startTime: time.Now(),
		sql:       data.SQL,
		args:      data.Args,
	})
}

func (t *queryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	qd, ok := ctx.Value(traceQueryKey{}).(*traceQueryData)
	if !ok {
		return
	}

	elapsed := time.Since(qd.startTime)
	if elapsed < SlowQueryThreshold {
		return
	}

	attrs := []slog.Attr{
		slog.Duration("duration", elapsed),
		slog.String("sql", truncateSQL(qd.sql, 200)),
		slog.Int("rows_affected", int(data.CommandTag.RowsAffected())),
	}

	if data.Err != nil {
		attrs = append(attrs, slog.String("error", data.Err.Error()))
	}

	slog.LogAttrs(ctx, slog.LevelWarn, "slow query", attrs...)
}

// truncateSQL trims long SQL statements for log readability.
func truncateSQL(sql string, maxLen int) string {
	if len(sql) <= maxLen {
		return sql
	}
	return sql[:maxLen] + "..."
}
