-- Revert 038: drop the new route-counter triggers, restore 000037's
-- update function (placeholder branch included, verbatim), and restore
-- the bare labor-log FK.

DROP TRIGGER IF EXISTS trg_ascent_delete_counts ON ascents;
DROP FUNCTION IF EXISTS decrement_route_ascent_counts();
DROP TRIGGER IF EXISTS trg_ascent_update_counts ON ascents;
DROP FUNCTION IF EXISTS adjust_route_ascent_counts_on_update();

CREATE OR REPLACE FUNCTION update_user_ascent_counters_on_update()
RETURNS TRIGGER AS $$
DECLARE
    old_send INTEGER := CASE WHEN OLD.ascent_type IN ('send','flash') THEN 1 ELSE 0 END;
    new_send INTEGER := CASE WHEN NEW.ascent_type IN ('send','flash') THEN 1 ELSE 0 END;
    old_flash INTEGER := CASE WHEN OLD.ascent_type = 'flash' THEN 1 ELSE 0 END;
    new_flash INTEGER := CASE WHEN NEW.ascent_type = 'flash' THEN 1 ELSE 0 END;
BEGIN
    IF OLD.ascent_type = NEW.ascent_type AND OLD.user_id = NEW.user_id AND OLD.route_id = NEW.route_id THEN
        RETURN NEW;
    END IF;
    IF OLD.user_id = NEW.user_id AND OLD.route_id = NEW.route_id THEN
        UPDATE users SET
            total_sends   = GREATEST(0, total_sends   + (new_send  - old_send)),
            total_flashes = GREATEST(0, total_flashes + (new_flash - old_flash))
        WHERE id = NEW.user_id;
        RETURN NEW;
    END IF;
    PERFORM update_user_ascent_counters() FROM (VALUES (1)) v(x); -- placeholder
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

ALTER TABLE setter_labor_logs DROP CONSTRAINT IF EXISTS setter_labor_logs_session_id_fkey;
ALTER TABLE setter_labor_logs ADD CONSTRAINT setter_labor_logs_session_id_fkey
    FOREIGN KEY (session_id) REFERENCES setting_sessions(id);
DROP INDEX IF EXISTS idx_setter_labor_session;
