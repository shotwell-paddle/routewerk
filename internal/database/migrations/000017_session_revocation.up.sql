-- Session revocation support.
-- Allows invalidating sessions on password change, role change, or admin action
-- without waiting for natural expiry. Previously sessions were only deleted;
-- revocation adds an auditable soft-invalidation path.

ALTER TABLE web_sessions ADD COLUMN revoked_at TIMESTAMPTZ;

-- Partial index: only non-revoked sessions matter for lookups.
-- expires_at filtering happens at query time since NOW() is not immutable.
CREATE INDEX idx_web_sessions_active_token
    ON web_sessions (token_hash)
    WHERE revoked_at IS NULL;
