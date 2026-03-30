-- Difficulty consensus votes (easy / right / hard) per user per route.
-- One vote per user+route; changes overwrite the previous vote.

CREATE TYPE difficulty_vote AS ENUM ('easy', 'right', 'hard');

CREATE TABLE difficulty_votes (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    route_id   UUID NOT NULL REFERENCES routes(id) ON DELETE CASCADE,
    vote       difficulty_vote NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, route_id)
);

CREATE INDEX idx_difficulty_votes_route ON difficulty_votes (route_id);
