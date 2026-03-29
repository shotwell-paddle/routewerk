-- 003_security_hardening.sql
-- Account lockout + audit logging tables

-- ── Login attempt tracking ────────────────────────────────────────
-- Tracks failed login attempts per email for account lockout.
-- Rows are transient — cleared on successful login.
CREATE TABLE IF NOT EXISTS login_attempts (
    email           TEXT NOT NULL PRIMARY KEY,
    failed_count    INT NOT NULL DEFAULT 0,
    locked_until    TIMESTAMPTZ,
    last_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ── Audit log ─────────────────────────────────────────────────────
-- Immutable append-only log of sensitive operations. Never delete rows.
CREATE TABLE IF NOT EXISTS audit_logs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_id    TEXT NOT NULL,
    action      TEXT NOT NULL,
    resource    TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    org_id      TEXT,
    metadata    JSONB,
    ip_address  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_org
    ON audit_logs (org_id, created_at DESC)
    WHERE org_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_audit_logs_actor
    ON audit_logs (actor_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_logs_resource
    ON audit_logs (resource, resource_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_logs_action
    ON audit_logs (action, created_at DESC);
