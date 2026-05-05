-- Phase 1a of the competition tracking module. See
-- docs/competitions-handoff.md for the full plan.
--
-- This migration adds the comp metadata tables: a competition belongs to a
-- location, has one or more events (a single comp is just a series of
-- length 1), each event has problems, and the comp defines categories
-- climbers register into.
--
-- Score storage is intentionally absent. Leaderboards are computed from
-- competition_attempts on read, by the scorer registered under the comp's
-- scoring_rule (or the event's override). See migration 000034.

CREATE TABLE competitions (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  location_id     uuid NOT NULL REFERENCES locations(id) ON DELETE CASCADE,
  name            text NOT NULL,
  slug            text NOT NULL,
  -- format = single | series. Aggregation specifics live in `aggregation`.
  format          text NOT NULL CHECK (format IN ('single','series')),
  -- aggregation jsonb shape (validated in app code, not by Postgres):
  --   { "method": "sum"|"sum_drop_n"|"weighted_finals"|"best_n",
  --     "drop": 1,                        // for sum_drop_n / best_n
  --     "weights": [1,1,1,2],             // for weighted_finals
  --     "finals_event_id": "<uuid>" }     // for weighted_finals
  aggregation     jsonb NOT NULL DEFAULT '{}'::jsonb,
  -- Default scorer for the comp. Per-event overrides on competition_events.
  scoring_rule    text NOT NULL,
  scoring_config  jsonb NOT NULL DEFAULT '{}'::jsonb,
  status          text NOT NULL DEFAULT 'draft'
                    CHECK (status IN ('draft','open','live','closed','archived')),
  leaderboard_visibility text NOT NULL DEFAULT 'public'
                    CHECK (leaderboard_visibility IN ('public','members','registrants')),
  starts_at              timestamptz NOT NULL,
  ends_at                timestamptz NOT NULL,
  registration_opens_at  timestamptz,
  registration_closes_at timestamptz,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE (location_id, slug)
);

CREATE TABLE competition_events (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  competition_id  uuid NOT NULL REFERENCES competitions(id) ON DELETE CASCADE,
  name            text NOT NULL,
  sequence        int  NOT NULL,
  starts_at       timestamptz NOT NULL,
  ends_at         timestamptz NOT NULL,
  weight          numeric NOT NULL DEFAULT 1.0,
  -- Optional per-event override of the comp-level scorer. Lets a single
  -- series mix event types (e.g. boulder night + speed night). NULL means
  -- "use the comp's scoring_rule + scoring_config".
  scoring_rule_override   text,
  scoring_config_override jsonb,
  UNIQUE (competition_id, sequence)
);

CREATE TABLE competition_categories (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  competition_id  uuid NOT NULL REFERENCES competitions(id) ON DELETE CASCADE,
  name            text NOT NULL,
  sort_order      int  NOT NULL DEFAULT 0,
  rules           jsonb NOT NULL DEFAULT '{}'::jsonb,
  UNIQUE (competition_id, name)
);

CREATE TABLE competition_problems (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  event_id        uuid NOT NULL REFERENCES competition_events(id) ON DELETE CASCADE,
  -- Optional link to the gym's route inventory. Many comp problems will be
  -- bespoke (paint-on-the-wall comp boulders set just for the event), so
  -- route_id is nullable. SET NULL on route delete keeps the comp record
  -- intact for historical leaderboards.
  route_id        uuid REFERENCES routes(id) ON DELETE SET NULL,
  label           text NOT NULL,
  points          numeric,
  zone_points     numeric,
  grade           text,
  color           text,
  sort_order      int NOT NULL DEFAULT 0,
  UNIQUE (event_id, label)
);

CREATE INDEX competitions_location_status_idx ON competitions(location_id, status);
CREATE INDEX competition_events_comp_idx ON competition_events(competition_id, sequence);
CREATE INDEX competition_problems_event_idx ON competition_problems(event_id, sort_order);
