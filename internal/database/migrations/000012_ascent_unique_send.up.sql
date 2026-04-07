-- Enforce at most one completed ascent (send or flash) per user per route.
-- This prevents TOCTOU race conditions where two concurrent requests could
-- both pass the application-level check and both insert a send/flash.

-- First, deduplicate: keep the earliest send/flash per (user, route) pair.
-- This is idempotent — if no duplicates exist, nothing is deleted.
DELETE FROM ascents a
USING ascents b
WHERE a.user_id = b.user_id
  AND a.route_id = b.route_id
  AND a.ascent_type IN ('send', 'flash')
  AND b.ascent_type IN ('send', 'flash')
  AND a.created_at > b.created_at;

-- Now create the unique index. IF NOT EXISTS so re-runs are safe.
CREATE UNIQUE INDEX IF NOT EXISTS idx_ascents_one_completion_per_user_route
    ON ascents (user_id, route_id)
    WHERE ascent_type IN ('send', 'flash');
