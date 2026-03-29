-- Routewerk Initial Schema
-- Multi-tenant: org → location → wall → route

-- ============================================================
-- EXTENSIONS
-- ============================================================

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ============================================================
-- ENUMS
-- ============================================================

CREATE TYPE user_role AS ENUM (
    'org_admin',
    'gym_manager',
    'head_setter',
    'setter',
    'climber'
);

CREATE TYPE route_type AS ENUM (
    'boulder',
    'route'
);

CREATE TYPE route_status AS ENUM (
    'active',
    'flagged',
    'archived'
);

CREATE TYPE ascent_type AS ENUM (
    'send',
    'flash',
    'attempt',
    'project'
);

CREATE TYPE grading_system AS ENUM (
    'circuit',      -- color/circuit-based boulder grading
    'v_scale',      -- V0-V17
    'font',         -- Fontainebleau
    'yds',          -- 5.6-5.15d (range-based for routes)
    'french',       -- French sport grades
    'ewbank',       -- Australian
    'uiaa'          -- UIAA
);

-- ============================================================
-- ORGANIZATIONS & LOCATIONS
-- ============================================================

CREATE TABLE organizations (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name            TEXT NOT NULL,
    slug            TEXT NOT NULL UNIQUE,
    logo_url        TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

CREATE TABLE locations (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    org_id          UUID NOT NULL REFERENCES organizations(id),
    name            TEXT NOT NULL,
    slug            TEXT NOT NULL,
    address         TEXT,
    timezone        TEXT NOT NULL DEFAULT 'America/New_York',
    website_url     TEXT,
    phone           TEXT,
    hours_json      JSONB,               -- flexible hours storage
    day_pass_info   TEXT,
    waiver_url      TEXT,
    allow_shared_setters BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ,

    UNIQUE (org_id, slug)
);

-- ============================================================
-- USERS & MEMBERSHIPS
-- ============================================================

CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email           TEXT NOT NULL UNIQUE,
    password_hash   TEXT NOT NULL,
    display_name    TEXT NOT NULL,
    avatar_url      TEXT,
    bio             TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

-- A user's role at a specific location (or org-wide for org_admin)
CREATE TABLE user_memberships (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id),
    org_id          UUID NOT NULL REFERENCES organizations(id),
    location_id     UUID REFERENCES locations(id),  -- NULL for org_admin (org-wide)
    role            user_role NOT NULL,
    specialties     TEXT[],                          -- for setters: ["slab", "comp", "youth"]
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ,

    UNIQUE (user_id, org_id, location_id, role)
);

CREATE TABLE refresh_tokens (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id),
    token_hash      TEXT NOT NULL UNIQUE,
    expires_at      TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at      TIMESTAMPTZ
);

-- ============================================================
-- WALLS & SECTORS
-- ============================================================

CREATE TABLE walls (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    location_id     UUID NOT NULL REFERENCES locations(id),
    name            TEXT NOT NULL,
    wall_type       route_type NOT NULL,             -- boulder wall vs rope wall
    angle           TEXT,                             -- "slab", "vertical", "15°", "30°", "45°", "roof"
    height_meters   NUMERIC(4,1),
    num_anchors     INTEGER,                         -- for rope walls
    surface_type    TEXT,                             -- "plywood", "concrete", "fiberglass"
    sort_order      INTEGER NOT NULL DEFAULT 0,
    map_x           NUMERIC(6,2),                    -- position on gym map
    map_y           NUMERIC(6,2),
    map_width       NUMERIC(6,2),
    map_height      NUMERIC(6,2),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

-- ============================================================
-- TAGS (flexible, user-defined)
-- ============================================================

CREATE TABLE tags (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    org_id          UUID NOT NULL REFERENCES organizations(id),
    category        TEXT NOT NULL,                    -- "style", "hold_type", "movement", "feature"
    name            TEXT NOT NULL,
    color           TEXT,                             -- hex color for UI display
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (org_id, category, name)
);

-- Seed some default tags per org via application code, not here

-- ============================================================
-- ROUTES & BOULDERS
-- ============================================================

CREATE TABLE routes (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    location_id     UUID NOT NULL REFERENCES locations(id),
    wall_id         UUID NOT NULL REFERENCES walls(id),
    setter_id       UUID REFERENCES users(id),

    -- Type & status
    route_type      route_type NOT NULL,
    status          route_status NOT NULL DEFAULT 'active',

    -- Grading
    grading_system  grading_system NOT NULL,
    grade           TEXT NOT NULL,                    -- "V4", "5.11a", "5.11a-5.11c", "Green Circuit"
    grade_low       TEXT,                             -- for range grades: low end
    grade_high      TEXT,                             -- for range grades: high end
    circuit_color   TEXT,                             -- for circuit grading: the color name

    -- Metadata
    name            TEXT,                             -- optional route name
    color           TEXT NOT NULL,                    -- tape/hold color used on wall
    description     TEXT,
    photo_url       TEXT,

    -- Lifecycle
    date_set        DATE NOT NULL DEFAULT CURRENT_DATE,
    projected_strip_date DATE,                       -- modifiable by head setter
    date_stripped   DATE,

    -- Stats (denormalized for fast reads, updated by workers)
    avg_rating      NUMERIC(3,2) DEFAULT 0,
    ascent_count    INTEGER DEFAULT 0,
    attempt_count   INTEGER DEFAULT 0,

    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

CREATE TABLE route_tags (
    route_id        UUID NOT NULL REFERENCES routes(id),
    tag_id          UUID NOT NULL REFERENCES tags(id),
    PRIMARY KEY (route_id, tag_id)
);

CREATE TABLE route_photos (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    route_id        UUID NOT NULL REFERENCES routes(id),
    photo_url       TEXT NOT NULL,
    caption         TEXT,
    uploaded_by     UUID REFERENCES users(id),
    sort_order      INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- SETTING SESSIONS
-- ============================================================

CREATE TABLE setting_sessions (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    location_id     UUID NOT NULL REFERENCES locations(id),
    scheduled_date  DATE NOT NULL,
    notes           TEXT,
    created_by      UUID NOT NULL REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE setting_session_assignments (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    session_id      UUID NOT NULL REFERENCES setting_sessions(id),
    setter_id       UUID NOT NULL REFERENCES users(id),
    wall_id         UUID REFERENCES walls(id),       -- NULL = unassigned to specific wall
    target_grades   TEXT[],                           -- ["V3", "V4", "V5"]
    notes           TEXT,

    UNIQUE (session_id, setter_id, wall_id)
);

-- ============================================================
-- SETTER PAY & LABOR
-- ============================================================

CREATE TABLE setter_pay_rates (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id),
    location_id     UUID NOT NULL REFERENCES locations(id),
    hourly_rate     NUMERIC(8,2),
    per_route_rate  NUMERIC(8,2),                    -- some gyms pay per route
    effective_from  DATE NOT NULL DEFAULT CURRENT_DATE,
    effective_to    DATE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (user_id, location_id, effective_from)
);

CREATE TABLE setter_labor_logs (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id),
    location_id     UUID NOT NULL REFERENCES locations(id),
    session_id      UUID REFERENCES setting_sessions(id),
    date            DATE NOT NULL,
    hours_worked    NUMERIC(4,2),
    routes_set      INTEGER DEFAULT 0,
    notes           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- CLIMBER: ASCENTS & RATINGS
-- ============================================================

CREATE TABLE ascents (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id),
    route_id        UUID NOT NULL REFERENCES routes(id),
    ascent_type     ascent_type NOT NULL,
    attempts        INTEGER DEFAULT 1,               -- number of attempts this session
    notes           TEXT,
    climbed_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE route_ratings (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id),
    route_id        UUID NOT NULL REFERENCES routes(id),
    rating          INTEGER NOT NULL CHECK (rating >= 1 AND rating <= 5),
    comment         TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (user_id, route_id)
);

-- ============================================================
-- COMMUNITY: FOLLOWS & ACTIVITY
-- ============================================================

CREATE TABLE follows (
    follower_id     UUID NOT NULL REFERENCES users(id),
    following_id    UUID NOT NULL REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (follower_id, following_id),
    CHECK (follower_id != following_id)
);

-- ============================================================
-- ACHIEVEMENTS & BADGES
-- ============================================================

CREATE TABLE achievement_definitions (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    org_id          UUID REFERENCES organizations(id),  -- NULL = global achievements
    slug            TEXT NOT NULL UNIQUE,
    name            TEXT NOT NULL,
    description     TEXT NOT NULL,
    icon_url        TEXT,
    criteria_json   JSONB NOT NULL,                  -- flexible rules engine: {"type": "ascent_count", "threshold": 100}
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE user_achievements (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id),
    achievement_id  UUID NOT NULL REFERENCES achievement_definitions(id),
    earned_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (user_id, achievement_id)
);

-- ============================================================
-- COACHING (lightweight v1)
-- ============================================================

CREATE TABLE training_plans (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    coach_id        UUID NOT NULL REFERENCES users(id),
    climber_id      UUID NOT NULL REFERENCES users(id),
    location_id     UUID NOT NULL REFERENCES locations(id),
    name            TEXT NOT NULL,
    description     TEXT,
    active          BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE training_plan_items (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    plan_id         UUID NOT NULL REFERENCES training_plans(id),
    route_id        UUID REFERENCES routes(id),      -- NULL for non-route tasks
    sort_order      INTEGER NOT NULL DEFAULT 0,
    title           TEXT NOT NULL,                    -- "Send this V4 slab" or "10 min hangboard"
    notes           TEXT,                             -- coach notes / instructions
    completed       BOOLEAN NOT NULL DEFAULT FALSE,
    completed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- PARTNER MATCHING (lightweight v1)
-- ============================================================

CREATE TABLE partner_profiles (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) UNIQUE,
    location_id     UUID NOT NULL REFERENCES locations(id),
    looking_for     TEXT[],                           -- ["belay_partner", "bouldering_buddy", "training_partner"]
    climbing_types  TEXT[],                           -- ["sport", "boulder", "trad"]
    grade_range     TEXT,                             -- "V3-V6" or "5.10-5.11"
    availability    JSONB,                            -- {"monday": ["morning", "evening"], ...}
    bio             TEXT,
    active          BOOLEAN NOT NULL DEFAULT TRUE,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- PUSH NOTIFICATIONS
-- ============================================================

CREATE TABLE device_tokens (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id),
    token           TEXT NOT NULL UNIQUE,
    platform        TEXT NOT NULL,                    -- "ios", "android"
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- INDEXES
-- ============================================================

-- Org/location lookups
CREATE INDEX idx_locations_org ON locations(org_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_user_memberships_user ON user_memberships(user_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_user_memberships_location ON user_memberships(location_id, role) WHERE deleted_at IS NULL;

-- Route queries (the hot path)
CREATE INDEX idx_routes_location_status ON routes(location_id, status) WHERE deleted_at IS NULL;
CREATE INDEX idx_routes_wall ON routes(wall_id, status) WHERE deleted_at IS NULL;
CREATE INDEX idx_routes_setter ON routes(setter_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_routes_strip_date ON routes(projected_strip_date) WHERE status = 'active' AND deleted_at IS NULL;
CREATE INDEX idx_routes_date_set ON routes(date_set DESC) WHERE deleted_at IS NULL;

-- Ascent queries
CREATE INDEX idx_ascents_user ON ascents(user_id, climbed_at DESC);
CREATE INDEX idx_ascents_route ON ascents(route_id);
CREATE INDEX idx_ascents_user_route ON ascents(user_id, route_id);

-- Ratings
CREATE INDEX idx_route_ratings_route ON route_ratings(route_id);

-- Follows
CREATE INDEX idx_follows_following ON follows(following_id);

-- Walls
CREATE INDEX idx_walls_location ON walls(location_id) WHERE deleted_at IS NULL;

-- Tags
CREATE INDEX idx_route_tags_tag ON route_tags(tag_id);

-- Setting sessions
CREATE INDEX idx_setting_sessions_location ON setting_sessions(location_id, scheduled_date);

-- Labor
CREATE INDEX idx_setter_labor_user ON setter_labor_logs(user_id, date DESC);
CREATE INDEX idx_setter_labor_location ON setter_labor_logs(location_id, date DESC);

-- Training plans
CREATE INDEX idx_training_plans_climber ON training_plans(climber_id) WHERE active = TRUE;
CREATE INDEX idx_training_plans_coach ON training_plans(coach_id) WHERE active = TRUE;

-- Partner profiles
CREATE INDEX idx_partner_profiles_location ON partner_profiles(location_id) WHERE active = TRUE;

-- Device tokens
CREATE INDEX idx_device_tokens_user ON device_tokens(user_id);

-- ============================================================
-- UPDATED_AT TRIGGER
-- ============================================================

CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Apply to all tables with updated_at
CREATE TRIGGER trg_organizations_updated_at BEFORE UPDATE ON organizations FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER trg_locations_updated_at BEFORE UPDATE ON locations FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER trg_users_updated_at BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER trg_user_memberships_updated_at BEFORE UPDATE ON user_memberships FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER trg_walls_updated_at BEFORE UPDATE ON walls FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER trg_routes_updated_at BEFORE UPDATE ON routes FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER trg_route_ratings_updated_at BEFORE UPDATE ON route_ratings FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER trg_setting_sessions_updated_at BEFORE UPDATE ON setting_sessions FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER trg_setter_labor_logs_updated_at BEFORE UPDATE ON setter_labor_logs FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER trg_training_plans_updated_at BEFORE UPDATE ON training_plans FOR EACH ROW EXECUTE FUNCTION update_updated_at();
CREATE TRIGGER trg_device_tokens_updated_at BEFORE UPDATE ON device_tokens FOR EACH ROW EXECUTE FUNCTION update_updated_at();
