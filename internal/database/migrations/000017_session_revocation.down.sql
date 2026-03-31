DROP INDEX IF EXISTS idx_web_sessions_active_token;
ALTER TABLE web_sessions DROP COLUMN IF EXISTS revoked_at;
