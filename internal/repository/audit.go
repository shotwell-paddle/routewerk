package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AuditRepo writes to the audit_logs table. The table schema is:
//
//	CREATE TABLE audit_logs (
//	    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
//	    actor_id   TEXT NOT NULL,          -- user who performed the action
//	    action     TEXT NOT NULL,          -- e.g. "org.update", "member.add"
//	    resource   TEXT NOT NULL,          -- e.g. "org", "location", "route"
//	    resource_id TEXT NOT NULL,         -- ID of the affected resource
//	    org_id     TEXT,                   -- org context (nullable for cross-org)
//	    metadata   JSONB,                 -- additional context
//	    ip_address TEXT,
//	    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
//	);
//	CREATE INDEX idx_audit_logs_org ON audit_logs (org_id, created_at DESC);
//	CREATE INDEX idx_audit_logs_actor ON audit_logs (actor_id, created_at DESC);
//	CREATE INDEX idx_audit_logs_resource ON audit_logs (resource, resource_id, created_at DESC);
type AuditRepo struct {
	db *pgxpool.Pool
}

func NewAuditRepo(db *pgxpool.Pool) *AuditRepo {
	return &AuditRepo{db: db}
}

// AuditEntry represents a single auditable event.
type AuditEntry struct {
	ActorID    string
	Action     string
	Resource   string
	ResourceID string
	OrgID      string                 // optional
	Metadata   map[string]interface{} // optional extra context
	IPAddress  string
}

// Log writes an audit entry. It never returns an error to the caller —
// audit failures are logged but must not break the business operation.
func (r *AuditRepo) Log(ctx context.Context, entry AuditEntry) {
	var metadataJSON []byte
	if entry.Metadata != nil {
		metadataJSON, _ = json.Marshal(entry.Metadata)
	}

	query := `
		INSERT INTO audit_logs (actor_id, action, resource, resource_id, org_id, metadata, ip_address)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7)`

	_, err := r.db.Exec(ctx, query,
		entry.ActorID,
		entry.Action,
		entry.Resource,
		entry.ResourceID,
		entry.OrgID,
		metadataJSON,
		entry.IPAddress,
	)
	if err != nil {
		// Log but don't fail — audit is best-effort
		fmt.Printf("AUDIT_LOG_ERROR: %v entry=%+v\n", err, entry)
	}
}
