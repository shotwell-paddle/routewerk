-- User-submitted community tags on routes.
-- Separate from setter-managed org tags — these are free-form text added by climbers.
CREATE TABLE IF NOT EXISTS user_route_tags (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    route_id    UUID NOT NULL REFERENCES routes(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tag_name    TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Each user can add a given tag to a route only once
    CONSTRAINT uq_user_route_tag UNIQUE (route_id, user_id, tag_name)
);

-- Fast lookup: all community tags for a route (aggregated counts)
CREATE INDEX idx_user_route_tags_route ON user_route_tags(route_id);

-- Fast lookup: all tags a user has added (for profile / removal)
CREATE INDEX idx_user_route_tags_user ON user_route_tags(user_id);
