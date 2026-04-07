-- Enforce at most one completed ascent (send or flash) per user per route.
-- This prevents TOCTOU race conditions where two concurrent requests could
-- both pass the application-level check and both insert a send/flash.

-- First, deduplicate: keep one send/flash per (user, route) pair.
-- Uses ROW_NUMBER to handle ties on created_at (e.g. seed data).
WITH dupes AS (
  SELECT id, ROW_NUMBER() OVER (
    PARTITION BY user_id, route_id
    ORDER BY created_at ASC, id ASC
  ) AS rn
  FROM ascents
  WHERE ascent_type IN ('send', 'flash')
)
DELETE FROM ascents WHERE id IN (SELECT id FROM dupes WHERE rn > 1);

-- Now create the unique index. IF NOT EXISTS so re-runs are safe.
CREATE UNIQUE INDEX IF NOT EXISTS idx_ascents_one_completion_per_user_route
    ON ascents (user_id, route_id)
    WHERE ascent_type IN ('send', 'flash');
