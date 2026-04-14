-- Team challenges: group quests where multiple climbers contribute together
CREATE TABLE team_challenges (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    location_id     UUID NOT NULL REFERENCES locations(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    description     TEXT NOT NULL,
    challenge_type  TEXT NOT NULL DEFAULT 'route_count' CHECK (challenge_type IN ('route_count', 'ascent_count', 'points')),
    target_count    INTEGER NOT NULL,
    start_date      TIMESTAMPTZ NOT NULL,
    end_date        TIMESTAMPTZ NOT NULL,
    max_team_size   INTEGER DEFAULT 6,
    is_active       BOOLEAN NOT NULL DEFAULT true,
    created_by      UUID NOT NULL REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_team_challenges_location ON team_challenges(location_id) WHERE is_active = true;

-- Teams within a challenge
CREATE TABLE challenge_teams (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    challenge_id    UUID NOT NULL REFERENCES team_challenges(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    captain_id      UUID NOT NULL REFERENCES users(id),
    progress_count  INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(challenge_id, name)
);

CREATE INDEX idx_challenge_teams_challenge ON challenge_teams(challenge_id);

-- Team membership
CREATE TABLE challenge_team_members (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    team_id     UUID NOT NULL REFERENCES challenge_teams(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    joined_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(team_id, user_id)
);

CREATE INDEX idx_challenge_team_members_user ON challenge_team_members(user_id);
CREATE INDEX idx_challenge_team_members_team ON challenge_team_members(team_id);

-- Individual contributions logged
CREATE TABLE challenge_contributions (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    team_id     UUID NOT NULL REFERENCES challenge_teams(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    route_id    UUID REFERENCES routes(id) ON DELETE SET NULL,
    points      INTEGER NOT NULL DEFAULT 1,
    notes       TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_challenge_contributions_team ON challenge_contributions(team_id, created_at DESC);
