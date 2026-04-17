-- Adds a hash_scheme column to refresh_tokens so we can migrate from bcrypt
-- (which salts every hash, forcing O(n) lookups per refresh) to keyed
-- HMAC-SHA256 (which is deterministic and permits an O(1) indexed lookup)
-- without invalidating tokens currently in the wild.
--
-- Rollout strategy:
--   1. This migration ships first. New tokens are written with scheme='hmac';
--      old tokens remain scheme='bcrypt'. Refresh code handles both.
--   2. After REFRESH_TOKEN_EXPIRY (30 days by default), every remaining live
--      token will be scheme='hmac'. A follow-up migration can then drop the
--      bcrypt code path and (optionally) the bcrypt rows.
--
-- The column defaults to 'bcrypt' because every row present when this runs
-- was written by the legacy bcrypt path.
ALTER TABLE refresh_tokens
    ADD COLUMN hash_scheme TEXT NOT NULL DEFAULT 'bcrypt'
    CHECK (hash_scheme IN ('bcrypt', 'hmac'));

-- HMAC lookups are equality-indexed on (hash_scheme, token_hash). Partial
-- index on 'hmac' keeps the index small — bcrypt rows are never looked up
-- this way (they're scanned via user_id).
CREATE INDEX idx_refresh_tokens_hmac_hash
    ON refresh_tokens (token_hash)
    WHERE hash_scheme = 'hmac' AND revoked_at IS NULL;
