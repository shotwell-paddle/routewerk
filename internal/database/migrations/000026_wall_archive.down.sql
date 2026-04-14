DROP INDEX IF EXISTS idx_walls_location_active;
ALTER TABLE walls DROP COLUMN IF EXISTS archived_at;
