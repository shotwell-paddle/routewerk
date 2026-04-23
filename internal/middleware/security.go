package middleware

import (
	"compress/gzip"
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// commonSecurityHeaders sets headers shared by both API and web responses.
func commonSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-XSS-Protection", "0") // modern best practice: disable legacy XSS filter
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
}

// SecureHeaders adds security headers tuned for JSON API responses.
// CSP is restrictive (default-src 'none') since APIs serve no HTML resources.
func SecureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		commonSecurityHeaders(w)
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// SecureHeadersWeb returns middleware that sets security headers tuned for the
// HTML frontend. The base CSP is `default-src 'none'` with per-directive
// allowances for self-hosted assets. If storageEndpoint is non-empty, its
// scheme+host is added to img-src so cross-origin URLs from S3-compatible
// object storage (e.g. Tigris) can render avatars and route photos.
//
// Only the scheme+host portion is used — any path/query on the endpoint is
// ignored, matching how the browser matches CSP source expressions.
//
// Note on workers: no `worker-src` directive. An earlier version allowed
// `worker-src 'self' blob:` to accommodate heic2any's libheif Web Worker,
// but that library has been removed in favor of the browser's native image
// pipeline (createImageBitmap + canvas). With nothing spawning workers on
// this app, we let worker-src fall through to default-src 'none' so any
// future accidental Worker spawn is blocked by the CSP instead of silently
// working.
func SecureHeadersWeb(storageEndpoint string) func(http.Handler) http.Handler {
	imgSrc := "img-src 'self' data:"
	if origin := originFromURL(storageEndpoint); origin != "" {
		// We emit both the apex (e.g. https://fly.storage.tigris.dev) and a
		// wildcard subdomain (https://*.fly.storage.tigris.dev) because we
		// serve images via virtual-hosted URLs (<bucket>.<host>/<key>) but
		// legacy rows and any in-flight path-style URLs still hit the apex.
		// CSP wildcards do NOT match the apex, so both are required during
		// the migration window.
		imgSrc = "img-src 'self' data: " + origin + " " + wildcardHostFromURL(storageEndpoint)
	}

	// TODO(audit-2026-04-22 S1): drop `'unsafe-inline'` from style-src.
	// The app currently ships ~185 inline style="color: ..." attributes
	// across templates for dynamic route/domain colors. Removing unsafe-
	// inline needs those refactored to CSS custom properties (or hashed
	// inline style blocks) as a separate pass. Tracked as a follow-up.
	csp := strings.Join([]string{
		"default-src 'none'",
		"script-src 'self'",
		"style-src 'self' 'unsafe-inline'",
		"font-src 'self'",
		imgSrc,
		"connect-src 'self'",
		"frame-ancestors 'none'",
		"base-uri 'self'",
		"form-action 'self'",
	}, "; ")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			commonSecurityHeaders(w)
			w.Header().Set("Content-Security-Policy", csp)
			w.Header().Set("Cache-Control", "no-store")
			next.ServeHTTP(w, r)
		})
	}
}

// originFromURL extracts scheme://host from a URL. Returns "" if the input is
// empty or not a usable absolute URL. Used to derive CSP source expressions
// from configured service endpoints without leaking path/query fragments.
func originFromURL(u string) string {
	if u == "" {
		return ""
	}
	parsed, err := url.Parse(u)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

// wildcardHostFromURL returns a CSP source expression matching any subdomain
// of the URL's host, e.g. "https://*.fly.storage.tigris.dev". Returns "" on
// an unusable input. This is paired with originFromURL when we need to match
// both the apex (e.g. legacy path-style URLs) and virtual-hosted subdomain
// URLs (e.g. https://<bucket>.fly.storage.tigris.dev/<key>) — CSP wildcards
// match strictly one label at the leftmost position and do NOT match the
// apex itself, so both expressions are needed to cover both URL shapes.
func wildcardHostFromURL(u string) string {
	if u == "" {
		return ""
	}
	parsed, err := url.Parse(u)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://*." + parsed.Host
}

// SecureHeadersStatic adds headers for immutable embedded static assets.
// Long-lived cache since assets are versioned by the binary build.
func SecureHeadersStatic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		next.ServeHTTP(w, r)
	})
}

// HSTS adds Strict-Transport-Security for production deployments.
// Only applied when isDev is false to avoid breaking local HTTP development.
func HSTS(isDev bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if isDev {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
			next.ServeHTTP(w, r)
		})
	}
}

// ── Gzip Compression ─────────────────────────────────────────────

// gzipPool reuses gzip writers to avoid per-request allocation of compression buffers.
var gzipPool = sync.Pool{
	New: func() any {
		gz, _ := gzip.NewWriterLevel(nil, gzip.DefaultCompression)
		return gz
	},
}

// gzippableContentType returns true for text payload types that are worth
// compressing. Binary formats (PNG, JPEG, WOFF2, pre-compressed archives)
// are left alone — gzipping them wastes CPU for zero payload win, and the
// response-writer wrapper itself has real allocation cost. See perf audit
// 2026-04-22 finding #7.
func gzippableContentType(ct string) bool {
	if ct == "" {
		return false
	}
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = ct[:i]
	}
	ct = strings.TrimSpace(ct)
	switch ct {
	case "text/html",
		"text/css",
		"text/plain",
		"text/xml",
		"application/javascript",
		"application/json",
		"application/xml",
		"image/svg+xml":
		return true
	}
	return false
}

// gzipResponseWriter lazily decides whether to compress based on the
// outbound Content-Type. We can't gate at request time because handlers set
// their own Content-Type inside ServeHTTP, so the decision has to be
// deferred until the header is actually flushed (WriteHeader or first
// Write). For non-compressible responses we never allocate a pooled gzip
// writer and never rewrite headers — the wrapper becomes a pass-through.
type gzipResponseWriter struct {
	http.ResponseWriter

	gz          *gzip.Writer
	passThrough bool
	wroteHeader bool
	decided     bool
}

// decide inspects the current response headers and either installs the
// gzip stream or flips to pass-through. Idempotent — callable from both
// WriteHeader and Write.
func (w *gzipResponseWriter) decide() {
	if w.decided {
		return
	}
	w.decided = true

	h := w.ResponseWriter.Header()
	// Don't double-compress. A handler that already set Content-Encoding
	// (pre-gzipped asset, sse event stream with identity encoding, etc.)
	// gets to keep its choice.
	if h.Get("Content-Encoding") != "" {
		w.passThrough = true
		return
	}
	if !gzippableContentType(h.Get("Content-Type")) {
		w.passThrough = true
		return
	}

	h.Set("Content-Encoding", "gzip")
	h.Add("Vary", "Accept-Encoding")
	// gzip rewrites the body length; drop any precomputed value so the
	// client doesn't trust a stale number.
	h.Del("Content-Length")

	gz := gzipPool.Get().(*gzip.Writer)
	gz.Reset(w.ResponseWriter)
	w.gz = gz
}

func (w *gzipResponseWriter) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.decide()
	w.ResponseWriter.WriteHeader(code)
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		// Mirror net/http's implicit WriteHeader(200). Sniff the content
		// type from the first write if the handler didn't set one; that
		// way our gzippable-type check has something to look at.
		if w.ResponseWriter.Header().Get("Content-Type") == "" {
			w.ResponseWriter.Header().Set("Content-Type", http.DetectContentType(b))
		}
		w.WriteHeader(http.StatusOK)
	}
	if w.passThrough {
		return w.ResponseWriter.Write(b)
	}
	return w.gz.Write(b)
}

// Close returns any pooled gzip writer. No-op on the pass-through path.
func (w *gzipResponseWriter) Close() {
	if w.gz == nil {
		return
	}
	_ = w.gz.Close()
	gzipPool.Put(w.gz)
	w.gz = nil
}

// Gzip compresses responses for clients that accept gzip encoding.
// The compression decision is deferred until the handler sets its
// Content-Type so binary assets (PNG, JPEG, WOFF2) served through the
// same middleware are left untouched. See perf audit 2026-04-22 #7.
func Gzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		gzw := &gzipResponseWriter{ResponseWriter: w}
		defer gzw.Close()

		next.ServeHTTP(gzw, r)
	})
}

// RateLimiter provides per-IP rate limiting for sensitive endpoints like login/register.
// The client map is capped at maxClients entries to prevent memory exhaustion from
// distributed attacks. When the cap is hit, the oldest entry is evicted.
type RateLimiter struct {
	mu         sync.Mutex
	clients    map[string]*clientWindow
	limit      int
	window     time.Duration
	cleanTTL   time.Duration
	maxClients int

	// insertCount drives opportunistic cleanup: every Nth new-key insert,
	// we sweep stale entries out of the map instead of waiting for the
	// per-window ticker. Keeps the map from sitting at cap during a burst
	// of unique keys (login scanning, flaky X-Forwarded-For). See perf
	// audit 2026-04-22 #5. Memory-neutral — same entry count at cap,
	// different eviction cadence.
	insertCount uint64
}

// opportunisticCleanupEvery controls how often LimitByKey runs a mid-stream
// cleanup pass. One sweep per 128 new keys is cheap (a single map scan
// under the already-held lock) and prevents the 1-minute web window from
// letting the map sit near maxClients for a full minute after a burst.
const opportunisticCleanupEvery = 128

type clientWindow struct {
	count       int
	windowStart time.Time
}

const defaultMaxClients = 10_000

// NewRateLimiter creates a rate limiter: limit requests per window per IP.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		clients:    make(map[string]*clientWindow),
		limit:      limit,
		window:     window,
		cleanTTL:   window * 2,
		maxClients: defaultMaxClients,
	}

	// Background cleanup every window interval
	go func() {
		ticker := time.NewTicker(window)
		defer ticker.Stop()
		for range ticker.C {
			rl.cleanup()
		}
	}()

	return rl
}

func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.cleanupLocked(time.Now())
}

// cleanupLocked is the body of cleanup without the mutex dance. Caller
// must already hold rl.mu. Shared between the background ticker and the
// opportunistic-inline path in LimitByKey so we only have one eviction
// policy.
func (rl *RateLimiter) cleanupLocked(now time.Time) {
	for ip, cw := range rl.clients {
		if now.Sub(cw.windowStart) > rl.cleanTTL {
			delete(rl.clients, ip)
		}
	}
}

// evictOldest removes the entry with the oldest windowStart. Caller must hold rl.mu.
func (rl *RateLimiter) evictOldest() {
	var oldestIP string
	var oldestTime time.Time
	first := true
	for ip, cw := range rl.clients {
		if first || cw.windowStart.Before(oldestTime) {
			oldestIP = ip
			oldestTime = cw.windowStart
			first = false
		}
	}
	if oldestIP != "" {
		delete(rl.clients, oldestIP)
	}
}

// clientIP extracts the real client IP, preferring the value set by chi's
// RealIP middleware (X-Real-IP / X-Forwarded-For) over r.RemoteAddr which
// is often just the proxy IP when running behind Fly.io or similar.
func clientIP(r *http.Request) string {
	// chi's RealIP middleware sets X-Real-IP from X-Forwarded-For then
	// overwrites r.RemoteAddr. However, RemoteAddr keeps the port suffix
	// (e.g. "1.2.3.4:12345") so strip it for a clean map key.
	ip := r.RemoteAddr
	if i := strings.LastIndex(ip, ":"); i != -1 {
		ip = ip[:i]
	}
	return ip
}

// Limit returns middleware that rejects requests over the rate limit with 429.
// Keying is per client IP.
func (rl *RateLimiter) Limit(next http.Handler) http.Handler {
	return rl.LimitByKey(clientIP)(next)
}

// LimitByKey returns middleware that rate-limits based on a caller-supplied
// key function. Use this for per-user or per-resource limits that should not
// collapse setters behind the same gym IP into a single bucket.
//
// If keyFn returns "", the limiter lets the request through unthrottled — the
// caller is responsible for gating anonymous traffic with a separate IP-based
// limiter (or requiring auth before this middleware runs).
func (rl *RateLimiter) LimitByKey(keyFn func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := keyFn(r)
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}

			rl.mu.Lock()
			cw, exists := rl.clients[key]
			now := time.Now()

			if !exists || now.Sub(cw.windowStart) > rl.window {
				// Opportunistic cleanup: every Nth new key, sweep stale
				// entries inline. Under a unique-key burst this keeps
				// the map well below maxClients without waiting on the
				// background ticker. Only runs when we're actually
				// inserting a new row — existing keys just reset their
				// own window in place.
				if !exists {
					rl.insertCount++
					if rl.insertCount%opportunisticCleanupEvery == 0 {
						rl.cleanupLocked(now)
					}
					if rl.maxClients > 0 && len(rl.clients) >= rl.maxClients {
						rl.evictOldest()
					}
				}
				rl.clients[key] = &clientWindow{count: 1, windowStart: now}
				rl.mu.Unlock()
				next.ServeHTTP(w, r)
				return
			}

			cw.count++
			if cw.count > rl.limit {
				rl.mu.Unlock()
				// Retry-After is approximate — it reports the full window
				// length rather than the time remaining. Accurate enough
				// for clients that just want a sane backoff.
				w.Header().Set("Retry-After", retryAfterSeconds(rl.window))
				http.Error(w, `{"error":"too many requests"}`, http.StatusTooManyRequests)
				return
			}

			rl.mu.Unlock()
			next.ServeHTTP(w, r)
		})
	}
}

func retryAfterSeconds(d time.Duration) string {
	sec := int(d.Seconds())
	if sec < 1 {
		sec = 1
	}
	return strconv.Itoa(sec)
}

// RequestTimeout wraps each request context with a deadline so that all
// downstream work (DB queries, S3 calls, etc.) is automatically cancelled
// if the handler exceeds the given duration.
func RequestTimeout(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// LimitBody caps the size of the request body via http.MaxBytesReader so
// ParseForm, ParseMultipartForm, and json.Decoder all fail cleanly with
// 413 (Request Entity Too Large) instead of letting a slow-streaming
// caller pin a request goroutine while pushing the process toward OOM.
//
// Handlers that legitimately accept larger payloads (image upload endpoints
// today) are free to re-wrap r.Body with a higher MaxBytesReader before
// ParseMultipartForm — the later wrapper wins. See S3 in the 2026-04-22
// perf audit.
func LimitBody(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}
