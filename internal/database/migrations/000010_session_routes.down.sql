-- Down migration 010
ALTER TABLE routes DROP COLUMN IF EXISTS session_id;
ALTER TABLE setting_sessions DROP COLUMN IF EXISTS status;
-- Note: cannot remove enum values in PostgreSQL without recreation
