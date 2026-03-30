-- Add custom_domain column so locations can be mapped to a vanity hostname.
-- e.g. "routes.mosaicclimbing.com" → the Mosaic location.
ALTER TABLE locations ADD COLUMN custom_domain TEXT UNIQUE;

-- Index for fast lookups by hostname.
CREATE INDEX idx_locations_custom_domain ON locations (custom_domain) WHERE custom_domain IS NOT NULL;
