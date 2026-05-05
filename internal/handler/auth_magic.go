package handler

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/shotwell-paddle/routewerk/internal/service"
)

// MagicAuthHandler exposes the JSON API for magic-link auth. The verify
// step (cookie + redirect) lives in the web handler since it sets a
// session cookie and returns HTML.
type MagicAuthHandler struct {
	svc *service.MagicLinkService
}

func NewMagicAuthHandler(svc *service.MagicLinkService) *MagicAuthHandler {
	return &MagicAuthHandler{svc: svc}
}

type magicRequestBody struct {
	Email string `json:"email"`
	Next  string `json:"next,omitempty"`
}

// Request issues a magic-link email if the address is registered. Always
// returns 202 with {"ok": true} regardless of outcome — the response
// timing/body must not reveal whether the email is associated with an
// account. Per-IP rate limit (existing 20/min auth limiter) and
// per-email rate limit (3/15min, enforced in the service against the DB)
// keep abuse contained.
//
// Response is 202 instead of 200 to signal "request accepted, no
// content yet" — the email send happens in the background via the job
// queue.
func (h *MagicAuthHandler) Request(w http.ResponseWriter, r *http.Request) {
	var body magicRequestBody
	if err := Decode(r, &body); err != nil {
		// Even malformed bodies get a 202 with the standard ok=true
		// response — we don't want a different error code to be a
		// signal about anything (although a parse failure is harmless,
		// keeping the response uniform makes the contract simpler).
		writeMagicAccepted(w)
		return
	}

	email := strings.ToLower(strings.TrimSpace(body.Email))
	if !emailRegex.MatchString(email) || len(email) > 254 {
		// Silently accept invalid emails too — same enumeration
		// argument as user-not-found. Log for ops.
		slog.Info("magic link request: malformed email", "email_len", len(email))
		writeMagicAccepted(w)
		return
	}

	next := safeMagicNext(body.Next)

	if err := h.svc.Request(r.Context(), service.RequestParams{
		Email:     email,
		NextPath:  next,
		IP:        clientIP(r),
		UserAgent: truncate(r.UserAgent(), 256),
	}); err != nil {
		// Real infrastructure error — log it. Still return 202 so the
		// client UX doesn't change based on whether the DB is happy.
		slog.Error("magic link request failed", "email", email, "error", err)
	}
	writeMagicAccepted(w)
}

func writeMagicAccepted(w http.ResponseWriter) {
	JSON(w, http.StatusAccepted, map[string]any{"ok": true})
}

// safeMagicNext returns the validated next-path pointer or nil. Mirrors
// the safeRedirect rules used by the web handlers (relative path only,
// no scheme, no host) to prevent open-redirect via the email link.
func safeMagicNext(raw string) *string {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil
	}
	if u.Scheme != "" || u.Host != "" {
		return nil
	}
	if !strings.HasPrefix(raw, "/") {
		return nil
	}
	return &raw
}

// clientIP returns the request's IP, preferring chi's RealIP middleware
// header. Mirrors what the existing web handler captures for sessions.
func clientIP(r *http.Request) string {
	// chi's RealIP middleware (mounted globally in router.go) sets
	// r.RemoteAddr from X-Forwarded-For when present.
	host, _, err := splitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func splitHostPort(addr string) (host, port string, err error) {
	// Tiny inline implementation rather than importing net for one call,
	// keeps this file's deps minimal. Acceptable: addr is always either
	// "host:port" or empty.
	idx := strings.LastIndex(addr, ":")
	if idx < 0 {
		return addr, "", nil
	}
	return addr[:idx], addr[idx+1:], nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

