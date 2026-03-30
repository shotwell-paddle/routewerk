-- Session checklist: a per-session instance of the setting playbook.
-- Each row is a checklist step that can be checked off during the session.
CREATE TABLE session_checklist_items (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    session_id  UUID NOT NULL REFERENCES setting_sessions(id) ON DELETE CASCADE,
    sort_order  INT NOT NULL DEFAULT 0,
    title       TEXT NOT NULL,
    completed   BOOLEAN NOT NULL DEFAULT FALSE,
    completed_by UUID REFERENCES users(id),
    completed_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_session_checklist_session ON session_checklist_items(session_id, sort_order);

-- Default playbook steps. These get copied into session_checklist_items when
-- a session is created. Stored here so the location can customize them later.
CREATE TABLE location_playbook_steps (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    location_id UUID NOT NULL REFERENCES locations(id),
    sort_order  INT NOT NULL DEFAULT 0,
    title       TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (location_id, sort_order)
);

-- Seed the default playbook for Mosaic Climbing
INSERT INTO location_playbook_steps (location_id, sort_order, title) VALUES
    ('b0000000-0000-4000-8000-000000000001', 1,  'Set up access control ropes'),
    ('b0000000-0000-4000-8000-000000000001', 2,  'Strip and clean walls'),
    ('b0000000-0000-4000-8000-000000000001', 3,  'Fix broken t-nuts'),
    ('b0000000-0000-4000-8000-000000000001', 4,  'Clean volumes'),
    ('b0000000-0000-4000-8000-000000000001', 5,  'Start washing holds'),
    ('b0000000-0000-4000-8000-000000000001', 6,  'Vacuum floor'),
    ('b0000000-0000-4000-8000-000000000001', 7,  'Route set'),
    ('b0000000-0000-4000-8000-000000000001', 8,  'Forerun'),
    ('b0000000-0000-4000-8000-000000000001', 9,  'Verify all holds are properly attached'),
    ('b0000000-0000-4000-8000-000000000001', 10, 'Tag routes'),
    ('b0000000-0000-4000-8000-000000000001', 11, 'Vacuum again'),
    ('b0000000-0000-4000-8000-000000000001', 12, 'Mop'),
    ('b0000000-0000-4000-8000-000000000001', 13, 'Open the sector');
