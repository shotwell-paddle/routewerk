-- Rewrites persisted Tigris URLs from path-style to virtual-hosted style.
--
-- Background:
--   Tigris (Fly.io's S3-compatible object store) only serves anonymous GETs
--   on virtual-hosted URLs:
--     https://<bucket>.fly.storage.tigris.dev/<key>   ← 200
--     https://fly.storage.tigris.dev/<bucket>/<key>   ← 403 AccessDenied
--   even when the bucket/object is public-read. Tigris also does NOT
--   implement PutBucketPolicy, so there's no way to open up path-style
--   anonymous reads server-side — the public-read mechanism on Tigris
--   IS virtual-hosted addressing.
--
--   Before this migration, StorageService.Upload wrote path-style public
--   URLs, which rendered as broken-image placeholders in every template.
--   storage.go now builds virtual-hosted URLs for new uploads; this
--   migration backfills existing rows to the same shape.
--
-- Scope:
--   Dev and prod share the bucket `routewerk-images` (per ops/runbook),
--   so a single REPLACE covers both environments. Three columns persist
--   public URLs:
--     users.avatar_url           — avatar uploads (climber_profile.go)
--     routes.photo_url           — primary route photo (setter_routes.go)
--     route_photos.photo_url     — per-route photo gallery (photos.go)
--
--   Other *_url columns (orgs.logo_url, orgs.website_url, orgs.waiver_url,
--   quests.icon_url) accept external URLs and are not written by our
--   upload path — safe to leave alone.

UPDATE users
   SET avatar_url = REPLACE(avatar_url,
       'https://fly.storage.tigris.dev/routewerk-images/',
       'https://routewerk-images.fly.storage.tigris.dev/')
 WHERE avatar_url LIKE 'https://fly.storage.tigris.dev/routewerk-images/%';

UPDATE routes
   SET photo_url = REPLACE(photo_url,
       'https://fly.storage.tigris.dev/routewerk-images/',
       'https://routewerk-images.fly.storage.tigris.dev/')
 WHERE photo_url LIKE 'https://fly.storage.tigris.dev/routewerk-images/%';

UPDATE route_photos
   SET photo_url = REPLACE(photo_url,
       'https://fly.storage.tigris.dev/routewerk-images/',
       'https://routewerk-images.fly.storage.tigris.dev/')
 WHERE photo_url LIKE 'https://fly.storage.tigris.dev/routewerk-images/%';
