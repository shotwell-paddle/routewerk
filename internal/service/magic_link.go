package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/shotwell-paddle/routewerk/internal/jobs"
	"github.com/shotwell-paddle/routewerk/internal/model"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

// Magic-link tunables. Public so the API handler / tests can refer to
// them — consumed by the rate-limit / expiry checks below.
const (
	// MagicLinkTokenBytes is the entropy of the plaintext token. 32 bytes
	// = 256 bits = collision-resistant under sha256.
	MagicLinkTokenBytes = 32

	// MagicLinkTTL is how long a token is valid after being issued.
	MagicLinkTTL = 15 * time.Minute

	// MagicLinkRateWindow + MagicLinkRateLimit form the per-email
	// throttle: at most N requests in the window. Implemented at the
	// repo layer (DB count) so it survives process restarts and can't
	// be bypassed by hitting different API instances. Layered with the
	// existing per-IP auth limiter (20/min) in the router.
	MagicLinkRateWindow = 15 * time.Minute
	MagicLinkRateLimit  = 3
)

// MagicLinkService orchestrates the magic-link request flow:
// rate-limit, user lookup, token generation, DB insert, email enqueue.
//
// Verification (consume + session creation) lives in the web handler so
// it can write the session cookie directly; the service's role on the
// verify side is just to consume the token via the repo and report who
// it belonged to.
type MagicLinkService struct {
	repo        *repository.MagicLinkRepo
	users       *repository.UserRepo
	queue       *jobs.Queue
	frontendURL string
}

func NewMagicLinkService(repo *repository.MagicLinkRepo, users *repository.UserRepo, queue *jobs.Queue, frontendURL string) *MagicLinkService {
	return &MagicLinkService{
		repo:        repo,
		users:       users,
		queue:       queue,
		frontendURL: frontendURL,
	}
}

// RequestParams is the input to Request. Email is required; everything
// else is optional metadata captured for audit / next-redirect.
type RequestParams struct {
	Email     string
	NextPath  *string // already validated by safeRedirect()
	IP        string
	UserAgent string
}

// Request issues a magic link if the email maps to a known user and the
// per-email rate limit hasn't been hit. The handler always responds 202
// regardless of outcome — this method's return signals only that the
// service ran cleanly, not whether an email was actually sent.
//
// The error return is reserved for unexpected infrastructure failures
// (DB down, queue insert fails). "User unknown" and "rate limit
// exceeded" are NOT errors here — they're logged and silently ignored
// to avoid email-enumeration via timing or response differences.
func (s *MagicLinkService) Request(ctx context.Context, p RequestParams) error {
	email := normalizeEmail(p.Email)
	if email == "" {
		return errors.New("magic link: email is required")
	}

	user, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		// Real DB error — bubble up. The handler still returns 202; the
		// error is logged for ops.
		return fmt.Errorf("user lookup: %w", err)
	}
	if user == nil {
		// Unknown email. No-op silently. We deliberately don't insert a
		// dummy row or return an error — the response timing should be
		// indistinguishable from a successful request.
		slog.Info("magic link requested for unknown email", "email", email)
		return nil
	}

	count, err := s.repo.CountRecentByEmail(ctx, email, time.Now().Add(-MagicLinkRateWindow))
	if err != nil {
		return fmt.Errorf("rate-limit lookup: %w", err)
	}
	if count >= MagicLinkRateLimit {
		slog.Warn("magic link rate limit hit",
			"email", email, "user_id", user.ID,
			"window", MagicLinkRateWindow, "limit", MagicLinkRateLimit,
		)
		return nil
	}

	token, hash, err := generateMagicToken()
	if err != nil {
		return fmt.Errorf("generate token: %w", err)
	}

	now := time.Now()
	row := &model.MagicLinkToken{
		Email:       email,
		UserID:      user.ID,
		TokenHash:   hash,
		NextPath:    p.NextPath,
		RequestedIP: nilIfEmpty(p.IP),
		UserAgent:   nilIfEmpty(p.UserAgent),
		ExpiresAt:   now.Add(MagicLinkTTL),
	}
	if err := s.repo.Create(ctx, row); err != nil {
		return fmt.Errorf("create token row: %w", err)
	}

	payload, err := json.Marshal(MagicLinkPayload{
		UserEmail:   email,
		DisplayName: user.DisplayName,
		Token:       token,
		NextPath:    derefStringOr(p.NextPath, ""),
	})
	if err != nil {
		return fmt.Errorf("marshal email payload: %w", err)
	}
	if _, err := s.queue.Enqueue(ctx, jobs.EnqueueParams{
		JobType: "email.magic_link",
		Payload: payload,
	}); err != nil {
		return fmt.Errorf("enqueue email: %w", err)
	}
	return nil
}

// Consume validates a token and returns the user it belongs to. Errors
// from this method are caller-facing — the verify handler maps them to
// a generic "invalid or expired link" page so an attacker can't probe
// whether a token existed.
func (s *MagicLinkService) Consume(ctx context.Context, token string) (*model.User, *model.MagicLinkToken, error) {
	hash, err := hashMagicToken(token)
	if err != nil {
		return nil, nil, ErrMagicLinkInvalid
	}

	row, err := s.repo.Consume(ctx, hash, time.Now())
	if err != nil {
		if errors.Is(err, repository.ErrMagicLinkNotFound) {
			return nil, nil, ErrMagicLinkInvalid
		}
		return nil, nil, fmt.Errorf("consume token: %w", err)
	}

	user, err := s.users.GetByID(ctx, row.UserID)
	if err != nil {
		return nil, nil, fmt.Errorf("user lookup after consume: %w", err)
	}
	if user == nil {
		// User soft-deleted between request and verify. Treat as invalid.
		return nil, nil, ErrMagicLinkInvalid
	}
	return user, row, nil
}

// ErrMagicLinkInvalid is the only caller-facing error from Consume.
// It collapses unknown / expired / consumed / decode-failed into one
// surface so we never leak which case applied.
var ErrMagicLinkInvalid = errors.New("magic link invalid or expired")

// ── Token generation ──────────────────────────────────────

// generateMagicToken returns the URL-safe plaintext token and its
// sha256 hash (32 bytes). Random source is crypto/rand.
func generateMagicToken() (token string, hash []byte, err error) {
	buf := make([]byte, MagicLinkTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", nil, fmt.Errorf("read random: %w", err)
	}
	sum := sha256.Sum256(buf)
	return base64.RawURLEncoding.EncodeToString(buf), sum[:], nil
}

// hashMagicToken decodes the URL-safe token and returns its sha256 hash.
// Caller treats decode failure as ErrMagicLinkInvalid (same surface as
// "no such token") to avoid revealing parsing details.
func hashMagicToken(token string) ([]byte, error) {
	buf, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, err
	}
	if len(buf) != MagicLinkTokenBytes {
		return nil, fmt.Errorf("magic link: token length %d, want %d", len(buf), MagicLinkTokenBytes)
	}
	sum := sha256.Sum256(buf)
	return sum[:], nil
}

// ── Email payload ─────────────────────────────────────────

// MagicLinkPayload is the job payload for magic-link emails. The plain
// text token is included so the email handler can build the verify URL.
// Stored in the jobs table; rotated/consumed within ~15 minutes per the
// TTL.
type MagicLinkPayload struct {
	UserEmail   string `json:"user_email"`
	DisplayName string `json:"display_name"`
	Token       string `json:"token"`
	NextPath    string `json:"next_path,omitempty"`
}

// ── Helpers ───────────────────────────────────────────────

func normalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func derefStringOr(p *string, fallback string) string {
	if p == nil {
		return fallback
	}
	return *p
}
