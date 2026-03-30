-- Reverse of 000004: drop web_sessions table and indexes.

DROP INDEX IF EXISTS idx_web_sessions_user_id;
DROP INDEX IF EXISTS idx_web_sessions_expires_at;
DROP TABLE IF EXISTS web_sessions;
