-- Rollback: remove settings columns
ALTER TABLE locations DROP COLUMN IF EXISTS settings_json;
ALTER TABLE organizations DROP COLUMN IF EXISTS settings_json;
