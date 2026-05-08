DROP TRIGGER IF EXISTS trg_ascent_user_counters_update ON ascents;
DROP TRIGGER IF EXISTS trg_ascent_user_counters ON ascents;
DROP FUNCTION IF EXISTS update_user_ascent_counters_on_update();
DROP FUNCTION IF EXISTS update_user_ascent_counters();

ALTER TABLE users
    DROP COLUMN IF EXISTS total_sends,
    DROP COLUMN IF EXISTS total_flashes,
    DROP COLUMN IF EXISTS total_logged,
    DROP COLUMN IF EXISTS unique_routes;
