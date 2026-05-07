-- Phase 1d of the comp module: magic-link auth (passwordless sign-in).
--
-- Flow:
--   1. POST /api/v1/auth/magic/request {email, next?}
--      → server generates a 32-byte random token, hashes it (sha256),
--        stores a row here, mails the user a link containing the
--        plaintext token. Always returns 202 (no enumeration of which
--        emails are registered).
--   2. GET /verify-magic?token=…&next=/comp/...
--      → server hashes the token, looks up the row, marks consumed,
--        creates a web session (cookie), redirects to next.
--
-- Security notes:
--   - Plaintext token never stored. Lookup is by hash.
--   - Single-use: consumed_at set on verify; partial unique constraint
--     prevents two rows for the same hash (token collision is
--     statistically impossible at 256-bit randomness, but the
--     constraint catches the impossible case at insert time).
--   - 15-minute expiry enforced in app code at verify time.
--   - next_path is validated by safeRedirect() before storage so a
--     stale email link can't open-redirect.

CREATE TABLE magic_link_tokens (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email        text NOT NULL,                                  -- lowercased + trimmed at insert
  user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash   bytea NOT NULL,                                 -- sha256(token), 32 bytes
  next_path    text,                                            -- safeRedirect-validated
  requested_ip text,
  user_agent   text,
  expires_at   timestamptz NOT NULL,
  consumed_at  timestamptz,
  created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX magic_link_tokens_token_hash_idx ON magic_link_tokens(token_hash);

-- Lets the per-email rate-limit query (count of pending requests in the
-- last N minutes) hit an index instead of scanning.
CREATE INDEX magic_link_tokens_email_pending_idx
  ON magic_link_tokens(email, created_at DESC)
  WHERE consumed_at IS NULL;
