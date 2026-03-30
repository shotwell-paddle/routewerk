-- Add user settings (privacy, preferences) as JSONB on users table
ALTER TABLE users
ADD COLUMN IF NOT EXISTS settings_json JSONB NOT NULL DEFAULT '{}'::jsonb;

-- Index for potential queries on privacy settings
CREATE INDEX IF NOT EXISTS idx_users_settings ON users USING gin (settings_json);
