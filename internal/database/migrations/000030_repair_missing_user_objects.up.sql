-- Repair migration: prod diverged from schema_migrations.
--
-- fly logs on 2026-04-22 showed:
--   ERROR: relation "user_route_tags" does not exist   (SQLSTATE 42P01)
--   ERROR: column u.settings_json does not exist       (SQLSTATE 42703)
-- despite schema_migrations.version = 29 — meaning migrations 13 (user_route_tags)
-- and 14 (users.settings_json) were recorded as applied on prod without their
-- SQL actually running. Most likely cause: an earlier `admin migrate-force N`
-- skipped past a failed migration.
--
-- This migration re-applies the missing objects using IF NOT EXISTS so it's a
-- no-op on any environment where they already exist. Do not fold this back into
-- migrations 13/14 directly — their SQL already has the same guards, and the
-- point here is a separate timestamped repair that every environment can run
-- safely.

-- From 000013_user_tags.up.sql
CREATE TABLE IF NOT EXISTS user_route_tags (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    route_id    UUID NOT NULL REFERENCES routes(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tag_name    TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_user_route_tag UNIQUE (route_id, user_id, tag_name)
);

CREATE INDEX IF NOT EXISTS idx_user_route_tags_route ON user_route_tags(route_id);
CREATE INDEX IF NOT EXISTS idx_user_route_tags_user  ON user_route_tags(user_id);

-- From 000014_user_settings.up.sql
ALTER TABLE users
ADD COLUMN IF NOT EXISTS settings_json JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE INDEX IF NOT EXISTS idx_users_settings ON users USING gin (settings_json);
