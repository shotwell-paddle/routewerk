-- Session strip targets: walls or individual routes to strip during a setting session.
-- A row with wall_id + route_id NULL means "strip the whole wall".
-- A row with route_id means "strip this specific route".
CREATE TABLE session_strip_targets (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    session_id  UUID NOT NULL REFERENCES setting_sessions(id) ON DELETE CASCADE,
    wall_id     UUID NOT NULL REFERENCES walls(id),
    route_id    UUID REFERENCES routes(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Prevent duplicate individual route entries
    UNIQUE (session_id, wall_id, route_id)
);

-- Prevent duplicate "whole wall" entries (route_id IS NULL)
CREATE UNIQUE INDEX idx_session_strip_wall_unique
    ON session_strip_targets(session_id, wall_id)
    WHERE route_id IS NULL;

CREATE INDEX idx_session_strip_targets_session ON session_strip_targets(session_id);
