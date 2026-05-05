-- Phase 1a: registration table for competitions.
--
-- A registration ties a user to a comp + category. user_id is mandatory
-- (walk-ins are out of scope; magic-link login + one-tap registration is
-- the UX). display_name is snapshotted at registration time so a user
-- changing their profile name later doesn't rewrite history.
--
-- Bib numbers are unique among ACTIVE registrations only — withdrawing
-- a climber frees up their bib for reuse later in the same comp.

CREATE TABLE competition_registrations (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  competition_id  uuid NOT NULL REFERENCES competitions(id) ON DELETE CASCADE,
  category_id     uuid NOT NULL REFERENCES competition_categories(id) ON DELETE RESTRICT,
  user_id         uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  display_name    text NOT NULL,                  -- snapshot at registration
  bib_number      int,
  waiver_signed_at timestamptz,
  paid_at         timestamptz,
  withdrawn_at    timestamptz,
  created_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE (competition_id, user_id)
);

-- Bib uniqueness only among active registrations; freed when withdrawn.
CREATE UNIQUE INDEX competition_reg_bib_active_idx
  ON competition_registrations(competition_id, bib_number)
  WHERE withdrawn_at IS NULL AND bib_number IS NOT NULL;

CREATE INDEX competition_reg_comp_idx ON competition_registrations(competition_id);
