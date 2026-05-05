-- Phase 1a: per-(registration, problem) attempt state plus an append-only
-- audit log.
--
-- competition_attempts holds the CURRENT state for a given climber on a
-- given problem (one row per pair, upserted). competition_attempt_log
-- records every action that produced that state — enables undo, audit,
-- and idempotent replay of climber actions.
--
-- The unified action endpoint (POST /api/v1/competitions/{id}/actions)
-- generates an idempotency_key client-side per action. The partial unique
-- index on idempotency_key dedupes accidental retries: server matches by
-- key and returns the prior result rather than applying twice.

CREATE TABLE competition_attempts (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  registration_id  uuid NOT NULL REFERENCES competition_registrations(id) ON DELETE CASCADE,
  problem_id       uuid NOT NULL REFERENCES competition_problems(id) ON DELETE CASCADE,
  attempts         int  NOT NULL DEFAULT 0,
  zone_attempts    int,
  zone_reached     bool NOT NULL DEFAULT false,
  top_reached      bool NOT NULL DEFAULT false,
  notes            text,
  logged_at        timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now(),
  verified_by      uuid REFERENCES users(id),
  verified_at      timestamptz,
  UNIQUE (registration_id, problem_id)
);

CREATE INDEX competition_attempts_problem_idx ON competition_attempts(problem_id);

CREATE TABLE competition_attempt_log (
  id               bigserial PRIMARY KEY,
  attempt_id       uuid NOT NULL REFERENCES competition_attempts(id) ON DELETE CASCADE,
  actor_user_id    uuid REFERENCES users(id),
  action           text NOT NULL,
  before           jsonb,
  after            jsonb,
  idempotency_key  uuid,
  at               timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX competition_attempt_log_attempt_idx
  ON competition_attempt_log(attempt_id, at DESC);

-- Idempotency dedupe: identical action submitted twice returns the same
-- result. Partial index lets non-idempotent legacy rows (no key) coexist.
CREATE UNIQUE INDEX competition_attempt_log_idem_idx
  ON competition_attempt_log(idempotency_key)
  WHERE idempotency_key IS NOT NULL;
