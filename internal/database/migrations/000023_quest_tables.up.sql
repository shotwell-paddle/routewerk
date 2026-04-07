-- Quest system tables for the progressions feature.
-- Dependency order: quest_domains → badges → quests → climber_quests → quest_logs → climber_badges → route_skill_tags

-- ============================================================
-- QUEST DOMAINS
-- ============================================================

CREATE TABLE quest_domains (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    location_id     UUID NOT NULL REFERENCES locations(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    description     TEXT,
    color           TEXT,
    icon            TEXT,
    sort_order      INTEGER DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(location_id, name)
);

-- ============================================================
-- BADGES
-- ============================================================

CREATE TABLE badges (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    location_id     UUID NOT NULL REFERENCES locations(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    description     TEXT,
    icon            TEXT NOT NULL,
    color           TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- QUESTS
-- ============================================================

CREATE TABLE quests (
    id                      UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    location_id             UUID NOT NULL REFERENCES locations(id) ON DELETE CASCADE,
    domain_id               UUID NOT NULL REFERENCES quest_domains(id) ON DELETE CASCADE,
    badge_id                UUID REFERENCES badges(id) ON DELETE SET NULL,
    name                    TEXT NOT NULL,
    description             TEXT NOT NULL,
    quest_type              TEXT NOT NULL CHECK (quest_type IN ('permanent', 'seasonal', 'event')),
    completion_criteria     TEXT NOT NULL,
    target_count            INTEGER,
    suggested_duration_days INTEGER,
    available_from          TIMESTAMPTZ,
    available_until         TIMESTAMPTZ,
    skill_level             TEXT NOT NULL DEFAULT 'any' CHECK (skill_level IN ('any', 'beginner', 'intermediate', 'advanced')),
    requires_certification  TEXT,
    route_tag_filter        TEXT[],
    is_active               BOOLEAN NOT NULL DEFAULT true,
    sort_order              INTEGER DEFAULT 0,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_quests_location_active ON quests(location_id) WHERE is_active = true;
CREATE INDEX idx_quests_type ON quests(quest_type);
CREATE INDEX idx_quests_available ON quests(available_from, available_until);

-- ============================================================
-- CLIMBER QUESTS (enrollment + progress)
-- ============================================================

CREATE TABLE climber_quests (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    quest_id        UUID NOT NULL REFERENCES quests(id) ON DELETE CASCADE,
    status          TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'completed', 'abandoned')),
    progress_count  INTEGER NOT NULL DEFAULT 0,
    started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMPTZ
);

-- One active enrollment per quest per user.
CREATE UNIQUE INDEX idx_climber_quests_one_active
    ON climber_quests(user_id, quest_id) WHERE status = 'active';

CREATE INDEX idx_climber_quests_user ON climber_quests(user_id);
CREATE INDEX idx_climber_quests_status ON climber_quests(user_id, status);

-- ============================================================
-- QUEST LOGS (progress entries)
-- ============================================================

CREATE TABLE quest_logs (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    climber_quest_id    UUID NOT NULL REFERENCES climber_quests(id) ON DELETE CASCADE,
    log_type            TEXT NOT NULL CHECK (log_type IN ('route_climbed', 'session_logged', 'reflection', 'self_assessment', 'event_attended', 'custom')),
    route_id            UUID REFERENCES routes(id) ON DELETE SET NULL,
    notes               TEXT,
    rating              INTEGER CHECK (rating BETWEEN 1 AND 5),
    logged_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- No dedup constraints — climbers can log the same route/activity
-- multiple times in a day (e.g., two separate sessions).
CREATE INDEX idx_quest_logs_climber_quest ON quest_logs(climber_quest_id);

-- ============================================================
-- CLIMBER BADGES (awarded badges)
-- ============================================================

CREATE TABLE climber_badges (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    badge_id    UUID NOT NULL REFERENCES badges(id) ON DELETE CASCADE,
    earned_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, badge_id)
);

CREATE INDEX idx_climber_badges_user ON climber_badges(user_id);

-- ============================================================
-- ROUTE SKILL TAGS
-- ============================================================

CREATE TABLE route_skill_tags (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    route_id    UUID NOT NULL REFERENCES routes(id) ON DELETE CASCADE,
    tag         TEXT NOT NULL,
    UNIQUE(route_id, tag)
);

CREATE INDEX idx_route_skill_tags_route ON route_skill_tags(route_id);
CREATE INDEX idx_route_skill_tags_tag ON route_skill_tags(tag);
