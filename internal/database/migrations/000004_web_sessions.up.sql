-- Web sessions for cookie-based authentication on the HTMX frontend.
-- Separate from the JWT-based API auth — these sessions are tied to a
-- browser cookie and carry the user + location context needed to render pages.

CREATE TABLE web_sessions (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    location_id   UUID REFERENCES locations(id) ON DELETE SET NULL,
    token_hash    TEXT NOT NULL UNIQUE,
    ip_address    INET,
    user_agent    TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at    TIMESTAMPTZ NOT NULL,
    last_seen_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Note: token_hash already has a UNIQUE constraint which creates an implicit index.
-- No additional index needed for token_hash lookups.

-- Cleanup expired sessions
CREATE INDEX idx_web_sessions_expires_at ON web_sessions (expires_at);

-- List sessions for a user (e.g. "manage active sessions" page)
CREATE INDEX idx_web_sessions_user_id ON web_sessions (user_id);
