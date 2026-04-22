-- Reverts virtual-hosted URLs back to path-style. Runs automatically if
-- this migration is rolled back.
--
-- After rollback, StorageService.Upload (if also reverted) would write
-- path-style URLs again; images would once more render as broken-image
-- placeholders because Tigris 403s anonymous path-style GETs. The down
-- migration is here for completeness, not because it gets you to a
-- working state — you also need to revert storage.go.

UPDATE users
   SET avatar_url = REPLACE(avatar_url,
       'https://routewerk-images.fly.storage.tigris.dev/',
       'https://fly.storage.tigris.dev/routewerk-images/')
 WHERE avatar_url LIKE 'https://routewerk-images.fly.storage.tigris.dev/%';

UPDATE routes
   SET photo_url = REPLACE(photo_url,
       'https://routewerk-images.fly.storage.tigris.dev/',
       'https://fly.storage.tigris.dev/routewerk-images/')
 WHERE photo_url LIKE 'https://routewerk-images.fly.storage.tigris.dev/%';

UPDATE route_photos
   SET photo_url = REPLACE(photo_url,
       'https://routewerk-images.fly.storage.tigris.dev/',
       'https://fly.storage.tigris.dev/routewerk-images/')
 WHERE photo_url LIKE 'https://routewerk-images.fly.storage.tigris.dev/%';
