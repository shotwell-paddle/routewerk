DROP INDEX IF EXISTS idx_users_settings;
ALTER TABLE users DROP COLUMN IF EXISTS settings_json;
