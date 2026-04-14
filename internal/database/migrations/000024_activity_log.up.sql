-- Append-only activity log. Social features (activity feeds, gym activity,
-- friend activity) all read from this table. Metadata is denormalized into
-- JSONB so feed queries need no joins.

CREATE TABLE activity_log (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    location_id     UUID NOT NULL REFERENCES locations(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    activity_type   TEXT NOT NULL,
    entity_type     TEXT NOT NULL,
    entity_id       UUID NOT NULL,
    metadata        JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Feed queries: "what's happening at this gym" (sorted by recency)
CREATE INDEX idx_activity_log_location ON activity_log(location_id, created_at DESC);

-- Profile queries: "what has this user done" (sorted by recency)
CREATE INDEX idx_activity_log_user ON activity_log(user_id, created_at DESC);

-- Filtering by activity type
CREATE INDEX idx_activity_log_type ON activity_log(activity_type);
