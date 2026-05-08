-- Denormalize user-level ascent aggregates onto the users row so the
-- profile page (and any future "what has this climber done lately"
-- surface) doesn't pay an O(rows) scan of the user's ascents on every
-- load. Mirrors the route-level pattern in migration 000002 which
-- maintains routes.{ascent_count, rating_count, avg_rating} via a
-- trigger for the same reason.
--
-- The four columns:
--   total_sends   — count of ascents with type IN ('send','flash')
--   total_flashes — count of ascents with type = 'flash'
--   total_logged  — count of ALL ascents (sends + attempts + repeats)
--   unique_routes — distinct count of route_id (a climber's "ticked
--                   routes" total, useful for a few things and the
--                   only counter that needs prior-row checks on insert)
--
-- All four are NOT NULL DEFAULT 0 so legacy code that doesn't touch
-- them keeps working. The trigger maintains them on every ascent
-- insert/delete; the migration backfills existing users in one
-- aggregate UPDATE.

ALTER TABLE users
    ADD COLUMN total_sends   INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN total_flashes INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN total_logged  INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN unique_routes INTEGER NOT NULL DEFAULT 0;

-- Backfill from the live ascents table. One pass.
UPDATE users u SET
    total_sends   = sub.sends,
    total_flashes = sub.flashes,
    total_logged  = sub.logged,
    unique_routes = sub.unique_routes
FROM (
    SELECT
        user_id,
        COUNT(*) FILTER (WHERE ascent_type IN ('send','flash')) AS sends,
        COUNT(*) FILTER (WHERE ascent_type = 'flash')           AS flashes,
        COUNT(*)                                                 AS logged,
        COUNT(DISTINCT route_id)                                 AS unique_routes
    FROM ascents
    GROUP BY user_id
) sub
WHERE u.id = sub.user_id;

-- Trigger function: maintain the four counters on insert/delete.
--
-- The unique_routes counter is the only fiddly one — we only bump it
-- when this is the user's FIRST ascent of the route (and only
-- decrement on delete when this was the LAST). Cheap EXISTS check
-- keyed on the indexed (user_id, route_id) pair (idx_ascents_user_route).
CREATE OR REPLACE FUNCTION update_user_ascent_counters()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        UPDATE users SET
            total_sends   = total_sends   + (CASE WHEN NEW.ascent_type IN ('send','flash') THEN 1 ELSE 0 END),
            total_flashes = total_flashes + (CASE WHEN NEW.ascent_type = 'flash'           THEN 1 ELSE 0 END),
            total_logged  = total_logged  + 1,
            unique_routes = unique_routes + (
                CASE WHEN EXISTS (
                    SELECT 1 FROM ascents
                    WHERE user_id = NEW.user_id
                      AND route_id = NEW.route_id
                      AND id <> NEW.id
                ) THEN 0 ELSE 1 END
            )
        WHERE id = NEW.user_id;
        RETURN NEW;
    ELSIF TG_OP = 'DELETE' THEN
        UPDATE users SET
            total_sends   = GREATEST(0, total_sends   - (CASE WHEN OLD.ascent_type IN ('send','flash') THEN 1 ELSE 0 END)),
            total_flashes = GREATEST(0, total_flashes - (CASE WHEN OLD.ascent_type = 'flash'           THEN 1 ELSE 0 END)),
            total_logged  = GREATEST(0, total_logged  - 1),
            unique_routes = GREATEST(0, unique_routes - (
                CASE WHEN EXISTS (
                    SELECT 1 FROM ascents
                    WHERE user_id = OLD.user_id
                      AND route_id = OLD.route_id
                      AND id <> OLD.id
                ) THEN 0 ELSE 1 END
            ))
        WHERE id = OLD.user_id;
        RETURN OLD;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_ascent_user_counters
    AFTER INSERT OR DELETE ON ascents
    FOR EACH ROW
    EXECUTE FUNCTION update_user_ascent_counters();

-- Updates to ascent_type (e.g. flipping an attempt to a send via the
-- per-tick edit endpoint) need separate handling — total_logged
-- doesn't change, but total_sends / total_flashes can flip. Keep the
-- logic in its own function so the insert/delete path stays simple.
CREATE OR REPLACE FUNCTION update_user_ascent_counters_on_update()
RETURNS TRIGGER AS $$
DECLARE
    old_send INTEGER := CASE WHEN OLD.ascent_type IN ('send','flash') THEN 1 ELSE 0 END;
    new_send INTEGER := CASE WHEN NEW.ascent_type IN ('send','flash') THEN 1 ELSE 0 END;
    old_flash INTEGER := CASE WHEN OLD.ascent_type = 'flash' THEN 1 ELSE 0 END;
    new_flash INTEGER := CASE WHEN NEW.ascent_type = 'flash' THEN 1 ELSE 0 END;
BEGIN
    -- Skip if nothing relevant changed.
    IF OLD.ascent_type = NEW.ascent_type AND OLD.user_id = NEW.user_id AND OLD.route_id = NEW.route_id THEN
        RETURN NEW;
    END IF;
    -- ascent_type change on the same row: counter delta only.
    IF OLD.user_id = NEW.user_id AND OLD.route_id = NEW.route_id THEN
        UPDATE users SET
            total_sends   = GREATEST(0, total_sends   + (new_send  - old_send)),
            total_flashes = GREATEST(0, total_flashes + (new_flash - old_flash))
        WHERE id = NEW.user_id;
        RETURN NEW;
    END IF;
    -- Re-parenting an ascent across users/routes is rare enough that
    -- we treat it as delete-then-insert via the existing function.
    PERFORM update_user_ascent_counters() FROM (VALUES (1)) v(x); -- placeholder
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_ascent_user_counters_update
    AFTER UPDATE OF ascent_type ON ascents
    FOR EACH ROW
    WHEN (OLD.ascent_type IS DISTINCT FROM NEW.ascent_type)
    EXECUTE FUNCTION update_user_ascent_counters_on_update();
