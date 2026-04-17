-- Adds a stable storage_key column to route_photos so we can delete files
-- from the object store without having to parse the public URL.
--
-- Rationale:
--   Before this change, StorageService.Delete took a photo_url and
--   reverse-engineered the S3 key by stripping a "<endpoint>/<bucket>/"
--   prefix. That worked only as long as StorageEndpoint never changed —
--   rotating the CDN or moving buckets would silently break delete (S3
--   returns 204 NoSuchKey, so deletes would pass while leaving orphans).
--
-- Rollout:
--   * storage_key is nullable for now. Every new upload writes both the
--     key and the URL; existing rows keep storage_key = NULL and fall
--     back to URL-parsing on delete. That drift is bounded: the oldest
--     rows without a key stay readable via the URL, they just miss the
--     new safety net.
--   * A future migration can require NOT NULL after a backfill job
--     (or after the legacy photos have aged out).
ALTER TABLE route_photos
    ADD COLUMN storage_key TEXT;

COMMENT ON COLUMN route_photos.storage_key IS
    'Object-store key (e.g. photos/<routeID>/<ts>.webp). NULL for legacy rows that predate migration 28; delete falls back to parsing photo_url.';
