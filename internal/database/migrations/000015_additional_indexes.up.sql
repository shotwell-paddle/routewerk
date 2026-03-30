-- Additional indexes identified via performance audit

-- Setting sessions: status filter (for session list filtering by open/complete)
CREATE INDEX IF NOT EXISTS idx_setting_sessions_status
    ON setting_sessions(location_id, status);

-- Routes: compound index for browse routes filtering by type
CREATE INDEX IF NOT EXISTS idx_routes_location_status_type
    ON routes(location_id, status, route_type) WHERE deleted_at IS NULL;

-- User memberships: org-level lookups (org team page, org-scoped membership checks)
CREATE INDEX IF NOT EXISTS idx_user_memberships_org
    ON user_memberships(org_id, role) WHERE deleted_at IS NULL;

-- Web sessions: cleanup of expired sessions
CREATE INDEX IF NOT EXISTS idx_web_sessions_expires
    ON web_sessions(expires_at);

-- Difficulty votes: route-level aggregation
CREATE INDEX IF NOT EXISTS idx_difficulty_votes_route
    ON difficulty_votes(route_id);
