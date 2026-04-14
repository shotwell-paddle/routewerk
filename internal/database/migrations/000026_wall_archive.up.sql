-- Allow head setters to archive walls.
-- Archived walls are hidden from climbers and cannot be edited by setters,
-- but remain visible (and reversible) to head setters and above.
ALTER TABLE walls ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ;

-- Index to accelerate "active walls" lookups (the common path for climbers
-- and setter route-creation flows).
CREATE INDEX IF NOT EXISTS idx_walls_location_active
    ON walls (location_id, sort_order, name)
    WHERE archived_at IS NULL AND deleted_at IS NULL;
