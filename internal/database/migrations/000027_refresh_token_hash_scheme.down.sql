DROP INDEX IF EXISTS idx_refresh_tokens_hmac_hash;
ALTER TABLE refresh_tokens DROP COLUMN IF EXISTS hash_scheme;
