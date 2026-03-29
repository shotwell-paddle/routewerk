package middleware

import (
	"net/http"
	"sync"
	"time"
)

// SecureHeaders adds security-related HTTP headers to every response.
func SecureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "0") // modern best practice: disable legacy XSS filter
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")

		next.ServeHTTP(w, r)
	})
}

// RateLimiter provides per-IP rate limiting for sensitive endpoints like login/register.
type RateLimiter struct {
	mu       sync.Mutex
	clients  map[string]*clientWindow
	limit    int
	window   time.Duration
	cleanTTL time.Duration
}

type clientWindow struct {
	count    int
	windowStart time.Time
}

// NewRateLimiter creates a rate limiter: limit requests per window per IP.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		clients:  make(map[string]*clientWindow),
		limit:    limit,
		window:   window,
		cleanTTL: window * 2,
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

// Limit returns middleware that rejects requests over the rate limit with 429.
func (rl *RateLimiter) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr // chi's RealIP middleware normalizes this

		rl.mu.Lock()
		cw, exists := rl.clients[ip]
		now := time.Now()

		if !exists || now.Sub(cw.windowStart) > rl.window {
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
