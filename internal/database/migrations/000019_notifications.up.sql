CREATE TABLE notifications (
    id            BIGSERIAL   PRIMARY KEY,
    user_id       UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type          TEXT        NOT NULL,
    title         TEXT        NOT NULL,
    body          TEXT        NOT NULL DEFAULT '',
    link          TEXT,                          -- optional deep link (e.g. "/routes/abc123")
    read_at       TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Unread notifications for a user (most common query)
CREATE INDEX idx_notifications_user_unread
    ON notifications (user_id, created_at DESC)
    WHERE read_at IS NULL;

-- Cleanup: all notifications for a user ordered by recency
CREATE INDEX idx_notifications_user_all
    ON notifications (user_id, created_at DESC);
