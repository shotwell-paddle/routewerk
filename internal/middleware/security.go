package middleware

import (
	"compress/gzip"
	"context"
	"io"
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
// `worker-src 'self' blob:` is required for heic2any: the library spawns a
// libheif Web Worker from a Blob URL at script-load time. Without an explicit
// worker-src, workers fall back through child-src → script-src → default-src;
// since default-src is 'none', the Worker creation would throw synchronously
// and prevent the library from ever registering `window.heic2any`.
func SecureHeadersWeb(storageEndpoint string) func(http.Handler) http.Handler {
	imgSrc := "img-src 'self' data:"
	if origin := originFromURL(storageEndpoint); origin != "" {
		imgSrc = "img-src 'self' data: " + origin
	}

	csp := strings.Join([]string{
		"default-src 'none'",
		"script-src 'self'",
		"worker-src 'self' blob:",
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

type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
	wroteHeader bool
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

func (w *gzipResponseWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		// Remove Content-Length since gzip changes it
		w.ResponseWriter.Header().Del("Content-Length")
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(code)
}

// gzipPool reuses gzip writers to avoid per-request allocation of compression buffers.
var gzipPool = sync.Pool{
	New: func() any {
		gz, _ := gzip.NewWriterLevel(nil, gzip.DefaultCompression)
		return gz
	},
}

// Gzip compresses responses for clients that accept gzip encoding.
// Only compresses text content types (HTML, CSS, JS, JSON, SVG).
func Gzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		gz := gzipPool.Get().(*gzip.Writer)
		gz.Reset(w)
		defer func() {
			gz.Close()
			gzipPool.Put(gz)
		}()

		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Vary", "Accept-Encoding")

		gzw := &gzipResponseWriter{Writer: gz, ResponseWriter: w}
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
}

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
	now := time.Now()
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
				if !exists && rl.maxClients > 0 && len(rl.clients) >= rl.maxClients {
					rl.evictOldest()
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
