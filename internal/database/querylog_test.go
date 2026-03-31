package database

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestQueryTracer_SlowQuerySetsContext(t *testing.T) {
	tracer := &queryTracer{}
	ctx := context.Background()

	// Simulate starting a query
	ctx = tracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{
		SQL:  "SELECT * FROM users WHERE id = $1",
		Args: []any{"abc123"},
	})

	// Verify the context carries trace data
	qd, ok := ctx.Value(traceQueryKey{}).(*traceQueryData)
	if !ok {
		t.Fatal("TraceQueryStart should set trace data in context")
	}
	if qd.sql != "SELECT * FROM users WHERE id = $1" {
		t.Errorf("sql = %q, want SELECT query", qd.sql)
	}
	if len(qd.args) != 1 {
		t.Errorf("args len = %d, want 1", len(qd.args))
	}
	if time.Since(qd.startTime) > time.Second {
		t.Error("startTime should be recent")
	}
}

func TestQueryTracer_EndWithoutStart(t *testing.T) {
	tracer := &queryTracer{}
	// Should not panic when context doesn't have trace data
	tracer.TraceQueryEnd(context.Background(), nil, pgx.TraceQueryEndData{
		CommandTag: pgconn.NewCommandTag("SELECT 0"),
	})
}

func TestTruncateSQL(t *testing.T) {
	tests := []struct {
		name   string
		sql    string
		maxLen int
		want   string
	}{
		{"short", "SELECT 1", 200, "SELECT 1"},
		{"exact", "SELECT 1", 8, "SELECT 1"},
		{"truncated", "SELECT * FROM very_long_table_name WHERE condition", 20, "SELECT * FROM very_l..."},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateSQL(tc.sql, tc.maxLen)
			if got != tc.want {
				t.Errorf("truncateSQL(%q, %d) = %q, want %q", tc.sql, tc.maxLen, got, tc.want)
			}
		})
	}
}
