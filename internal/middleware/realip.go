package middleware

import (
	"net"
	"net/http"
	"strings"
)

// flyClientIPHeader is set by Fly.io's edge proxy on every request it
// forwards, and any client-supplied value is overwritten at the edge —
// unlike X-Forwarded-For / X-Real-IP it cannot be forged by callers.
// See https://fly.io/docs/networking/request-headers/
const flyClientIPHeader = "Fly-Client-IP"

// TrustedClientIP sets r.RemoteAddr to the client IP asserted by Fly's
// proxy (Fly-Client-IP) when that header carries a valid IP address.
//
// It replaces chi's RealIP middleware, which trusted X-Forwarded-For and
// X-Real-IP from anyone: a forged header handed each request a fresh
// rate-limit bucket (defeating the 10/min login limiter) and could spoof
// an RFC1918 source address to pass the internal-caller check on /health.
// Those headers are now ignored entirely.
//
// When the header is absent (local dev, direct connections, health probes
// arriving over Fly's private network) or does not parse as an IP, the
// request's RemoteAddr is left untouched — the TCP peer address is the
// only other value we can trust. The rewritten value is a bare canonical
// IP with no port, matching chi RealIP's contract so downstream
// consumers (rate-limit keys, logging, magic-link audit) work unchanged.
func TrustedClientIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v := strings.TrimSpace(r.Header.Get(flyClientIPHeader)); v != "" {
			if ip := net.ParseIP(v); ip != nil {
				r.RemoteAddr = ip.String()
			}
		}
		next.ServeHTTP(w, r)
	})
}
