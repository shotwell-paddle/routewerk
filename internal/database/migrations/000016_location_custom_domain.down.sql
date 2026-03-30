DROP INDEX IF EXISTS idx_locations_custom_domain;
ALTER TABLE locations DROP COLUMN IF EXISTS custom_domain;
