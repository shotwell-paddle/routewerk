-- Feature flag: controls whether climbers see progressions UI at this location.
-- Admin quest tools are always accessible regardless of this flag.
ALTER TABLE locations ADD COLUMN IF NOT EXISTS progressions_enabled BOOLEAN NOT NULL DEFAULT false;
