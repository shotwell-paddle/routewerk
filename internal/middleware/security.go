package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
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

// SecureHeadersWeb adds security headers tuned for the HTML frontend.
// CSP allows self-hosted assets plus Google Fonts for the Inter typeface.
func SecureHeadersWeb(next http.Handler) http.Handler {
	csp := strings.Join([]string{
		"default-src 'none'",
		"script-src 'self'",
		"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com",
		"font-src 'self' https://fonts.gstatic.com",
		"img-src 'self' data:",
		"connect-src 'self'",
		"frame-ancestors 'none'",
		"base-uri 'self'",
		"form-action 'self'",
	}, "; ")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		commonSecurityHeaders(w)
		w.Header().Set("Content-Security-Policy", csp)
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
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

// Limit returns middleware that rejects requests over the rate limit with 429.
func (rl *RateLimiter) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr // chi's RealIP middleware normalizes this

		rl.mu.Lock()
		cw, exists := rl.clients[ip]
		now := time.Now()

		if !exists || now.Sub(cw.windowStart) > rl.window {
			// Evict oldest entry if at capacity
			if !exists && rl.maxClients > 0 && len(rl.clients) >= rl.maxClients {
				rl.evictOldest()
			}
			rl.clients[ip] = &clientWindow{count: 1, windowStart: now}
			rl.mu.Unlock()
			next.ServeHTTP(w, r)
			return
		}

		cw.count++
		if cw.count > rl.limit {
			rl.mu.Unlock()
			w.Header().Set("Retry-After", "60")
			http.Error(w, `{"error":"too many requests"}`, http.StatusTooManyRequests)
			return
		}

		rl.mu.Unlock()
		next.ServeHTTP(w, r)
	})
}
