package model

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// MagicLinkToken is one outstanding (or already-consumed) magic-link
// request. Plaintext token is never stored; TokenHash holds sha256(token).
// See migration 000035 for the schema and docs/competitions-handoff.md
// (Phase 1d) for the flow.
type MagicLinkToken struct {
	ID          string             `json:"id"`
	Email       string             `json:"email"` // lowercased + trimmed
	UserID      string             `json:"user_id"`
	TokenHash   []byte             `json:"-"` // sha256(plaintext token), 32 bytes
	NextPath    *string            `json:"next_path,omitempty"`
	RequestedIP *string            `json:"requested_ip,omitempty"`
	UserAgent   *string            `json:"user_agent,omitempty"`
	ExpiresAt   time.Time          `json:"expires_at"`
	ConsumedAt  pgtype.Timestamptz `json:"consumed_at,omitempty"`
	CreatedAt   time.Time          `json:"created_at"`
}

// IsExpired reports whether the token's lifetime has elapsed. Caller
// should also check IsConsumed — both states make a token unusable.
func (t MagicLinkToken) IsExpired(now time.Time) bool {
	return now.After(t.ExpiresAt)
}

// IsConsumed reports whether the token has already been used. Single-use
// is enforced at the DB level by the consume update; this helper is for
// pre-flight checks and error messages.
func (t MagicLinkToken) IsConsumed() bool {
	return t.ConsumedAt.Valid
}
