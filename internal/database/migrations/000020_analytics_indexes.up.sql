-- Composite index for setter productivity queries that filter by
-- (setter_id, location_id, date_set). The existing idx_routes_setter
-- only covers setter_id which forces a filter scan on location + date.
CREATE INDEX IF NOT EXISTS idx_routes_setter_location_date
    ON routes (setter_id, location_id, date_set DESC)
    WHERE deleted_at IS NULL;

-- Composite index for setter labor queries in SetterProductivity
-- that need (user_id, location_id, date).
CREATE INDEX IF NOT EXISTS idx_setter_labor_user_location
    ON setter_labor_logs (user_id, location_id, date DESC);

-- Covering index on routes(location_id, id) to speed up the join path
-- from locations → routes → ascents in OrgOverview and dashboard queries.
-- Includes status for filter pushdown.
CREATE INDEX IF NOT EXISTS idx_routes_location_id_status
    ON routes (location_id, id, status)
    WHERE deleted_at IS NULL;

-- Partial index for overdue strip queries scoped to a location.
-- The existing idx_routes_strip_date doesn't include location_id.
CREATE INDEX IF NOT EXISTS idx_routes_location_overdue_strip
    ON routes (location_id, projected_strip_date)
    WHERE status = 'active' AND projected_strip_date IS NOT NULL AND deleted_at IS NULL;

-- Index for notification cleanup and listing by user + time range.
-- The notifications table indexes were already created in 000019 but
-- let's ensure we have one for the cleanup DELETE query.
CREATE INDEX IF NOT EXISTS idx_notifications_created_at
    ON notifications (created_at)
    WHERE read_at IS NOT NULL;
