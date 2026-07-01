-- ============================================================
-- Migration 038: counter integrity + labor-log FK
--
-- 1) Route ascent counters (000002) were INSERT-only: every ascent
--    delete or ascent_type edit silently drifts routes.ascent_count /
--    attempt_count, permanently. Add DELETE + UPDATE triggers and
--    re-sync existing counters once.
-- 2) 000037's update_user_ascent_counters_on_update() left a broken
--    PERFORM placeholder in its re-parent branch — an ascent update
--    that changes user/route AND type would error at runtime. Replace
--    with correct delta math.
-- 3) setter_labor_logs.session_id had a bare FK: hard-deleting a
--    session with labor logged against it violated the constraint
--    (500 on session delete). ON DELETE SET NULL + supporting index.
-- ============================================================

-- ── 1) Route counters: DELETE ───────────────────────────────
CREATE OR REPLACE FUNCTION decrement_route_ascent_counts()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE routes SET
        ascent_count = GREATEST(0, ascent_count - CASE WHEN OLD.ascent_type IN ('send', 'flash') THEN 1 ELSE 0 END),
        attempt_count = GREATEST(0, attempt_count - 1)
    WHERE id = OLD.route_id;
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_ascent_delete_counts ON ascents;
CREATE TRIGGER trg_ascent_delete_counts
    AFTER DELETE ON ascents
    FOR EACH ROW
    EXECUTE FUNCTION decrement_route_ascent_counts();

-- ── 1) Route counters: UPDATE of ascent_type ────────────────
-- attempt_count counts every ascent row, so a type change only moves
-- the send/flash contribution.
CREATE OR REPLACE FUNCTION adjust_route_ascent_counts_on_update()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE routes SET
        ascent_count = GREATEST(0, ascent_count
            + CASE WHEN NEW.ascent_type IN ('send', 'flash') THEN 1 ELSE 0 END
            - CASE WHEN OLD.ascent_type IN ('send', 'flash') THEN 1 ELSE 0 END)
    WHERE id = NEW.route_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_ascent_update_counts ON ascents;
CREATE TRIGGER trg_ascent_update_counts
    AFTER UPDATE OF ascent_type ON ascents
    FOR EACH ROW
    WHEN (OLD.ascent_type IS DISTINCT FROM NEW.ascent_type)
    EXECUTE FUNCTION adjust_route_ascent_counts_on_update();

-- ── 1) One-time re-sync of accumulated drift ────────────────
UPDATE routes SET ascent_count = 0, attempt_count = 0;
UPDATE routes r SET
    ascent_count = a.sends,
    attempt_count = a.attempts
FROM (
    SELECT route_id,
        COUNT(*) FILTER (WHERE ascent_type IN ('send', 'flash')) AS sends,
        COUNT(*) AS attempts
    FROM ascents
    GROUP BY route_id
) a
WHERE a.route_id = r.id;

-- ── 2) Fix the 000037 placeholder branch ────────────────────
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
    -- Re-parented ascent (user and/or route changed): NOT supported.
    -- No application path re-parents ascents, and doing the counter math
    -- correctly here means moving total_sends/total_flashes AND
    -- total_logged AND unique_routes across both users plus both routes'
    -- counters — easy to get silently wrong (000037's placeholder proved
    -- it). Fail loudly instead of drifting.
    -- (Replaces the invalid PERFORM placeholder from 000037.)
    RAISE EXCEPTION 'ascent re-parenting (user_id/route_id change) is not supported';
END;
$$ LANGUAGE plpgsql;

-- ── 3) Labor-log FK + index ─────────────────────────────────
ALTER TABLE setter_labor_logs DROP CONSTRAINT IF EXISTS setter_labor_logs_session_id_fkey;
ALTER TABLE setter_labor_logs ADD CONSTRAINT setter_labor_logs_session_id_fkey
    FOREIGN KEY (session_id) REFERENCES setting_sessions(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_setter_labor_session
    ON setter_labor_logs(session_id) WHERE session_id IS NOT NULL;
