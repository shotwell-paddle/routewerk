-- ============================================================
-- Migration 010: Session lifecycle + session-linked routes
-- ============================================================

-- 1. Add status to setting_sessions
ALTER TABLE setting_sessions
    ADD COLUMN status TEXT NOT NULL DEFAULT 'planning';

-- 2. Add 'draft' to route_status enum
ALTER TYPE route_status ADD VALUE IF NOT EXISTS 'draft' BEFORE 'active';

-- 3. Link routes to the session that created them
ALTER TABLE routes
    ADD COLUMN session_id UUID REFERENCES setting_sessions(id) ON DELETE SET NULL;

CREATE INDEX idx_routes_session_id ON routes(session_id) WHERE session_id IS NOT NULL;
