-- Session revocation support.
-- Allows invalidating sessions on password change, role change, or admin action
-- without waiting for natural expiry. Previously sessions were only deleted;
-- revocation adds an auditable soft-invalidation path.

ALTER TABLE web_sessions ADD COLUMN revoked_at TIMESTAMPTZ;

-- Partial index: only non-revoked, non-expired sessions matter for lookups.
-- This replaces the implicit unique index on token_hash for the hot path.
CREATE INDEX idx_web_sessions_active_token
    ON web_sessions (token_hash)
    WHERE revoked_at IS NULL AND expires_at > NOW();
