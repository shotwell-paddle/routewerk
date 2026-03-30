-- Enforce at most one completed ascent (send or flash) per user per route.
-- This prevents TOCTOU race conditions where two concurrent requests could
-- both pass the application-level check and both insert a send/flash.
CREATE UNIQUE INDEX idx_ascents_one_completion_per_user_route
    ON ascents (user_id, route_id)
    WHERE ascent_type IN ('send', 'flash');
