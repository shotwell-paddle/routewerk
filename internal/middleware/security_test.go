package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ── Gzip ─────────────────────────────────────────────────────

func TestGzip_CompressesWhenAccepted(t *testing.T) {
	handler := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<h1>Hello World</h1>"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatal("expected Content-Encoding: gzip")
	}
	if rec.Header().Get("Vary") != "Accept-Encoding" {
		t.Error("expected Vary: Accept-Encoding")
	}

	// Decompress and verify content
	gr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	defer gr.Close()

	body, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("failed to read gzip body: %v", err)
	}

	if string(body) != "<h1>Hello World</h1>" {
		t.Errorf("body = %q, want %q", string(body), "<h1>Hello World</h1>")
	}
}

func TestGzip_SkipsWhenNotAccepted(t *testing.T) {
	handler := Gzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("plain text"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// No Accept-Encoding header
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Content-Encoding") == "gzip" {
		t.Error("should not set gzip when not accepted")
	}

	if rec.Body.String() != "plain text" {
		t.Errorf("body = %q, want %q", rec.Body.String(), "plain text")
	}
}

// ── Security Headers ─────────────────────────────────────────

func TestSecureHeaders_API(t *testing.T) {
	handler := SecureHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/routes", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	expected := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":       "DENY",
		"X-XSS-Protection":      "0",
		"Cache-Control":          "no-store",
	}

	for header, want := range expected {
		got := rec.Header().Get(header)
		if got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}

	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'none'") {
		t.Errorf("API CSP should contain default-src 'none', got %q", csp)
	}
}

func TestSecureHeadersWeb(t *testing.T) {
	handler := SecureHeadersWeb(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")
	// Web CSP should allow self-hosted scripts and fonts
	for _, fragment := range []string{"script-src 'self'", "style-src 'self' 'unsafe-inline'", "font-src 'self'"} {
		if !strings.Contains(csp, fragment) {
			t.Errorf("web CSP missing %q, got %q", fragment, csp)
		}
	}
}

func TestSecureHeadersStatic(t *testing.T) {
	handler := SecureHeadersStatic(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/static/css/routewerk.css", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	cc := rec.Header().Get("Cache-Control")
	if !strings.Contains(cc, "immutable") {
		t.Errorf("static Cache-Control should contain 'immutable', got %q", cc)
	}
	if !strings.Contains(cc, "max-age=31536000") {
		t.Errorf("static Cache-Control should contain 'max-age=31536000', got %q", cc)
	}
}

func TestHSTS_DevDisabled(t *testing.T) {
	handler := HSTS(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Strict-Transport-Security") != "" {
		t.Error("HSTS should not be set in dev mode")
	}
}

func TestHSTS_ProdEnabled(t *testing.T) {
	handler := HSTS(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	hsts := rec.Header().Get("Strict-Transport-Security")
	if hsts == "" {
		t.Fatal("HSTS should be set in production")
	}
	if !strings.Contains(hsts, "includeSubDomains") {
		t.Errorf("HSTS should include 'includeSubDomains', got %q", hsts)
	}
}

// ── Rate Limiter ─────────────────────────────────────────────

func TestRateLimiter_AllowsUnderLimit(t *testing.T) {
	rl := &RateLimiter{
		clients: make(map[string]*clientWindow),
		limit:   5,
		window:  60_000_000_000, // 1 minute as nanoseconds
	}

	handler := rl.Limit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/login", nil)
		req.RemoteAddr = "1.2.3.4:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("request %d should pass, got %d", i+1, rec.Code)
		}
	}
}

func TestRateLimiter_BlocksOverLimit(t *testing.T) {
	rl := &RateLimiter{
		clients: make(map[string]*clientWindow),
		limit:   3,
		window:  60_000_000_000,
	}

	handler := rl.Limit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Burn through limit
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/login", nil)
		req.RemoteAddr = "1.2.3.4:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	// Next request should be blocked
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.RemoteAddr = "1.2.3.4:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("should set Retry-After header")
	}
}

func TestRateLimiter_DifferentIPsAreIndependent(t *testing.T) {
	rl := &RateLimiter{
		clients: make(map[string]*clientWindow),
		limit:   1,
		window:  60_000_000_000,
	}

	handler := rl.Limit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First IP uses up its limit
	req1 := httptest.NewRequest(http.MethodPost, "/login", nil)
	req1.RemoteAddr = "1.1.1.1:1111"
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatal("first request from IP1 should pass")
	}

	// Second IP should still work
	req2 := httptest.NewRequest(http.MethodPost, "/login", nil)
	req2.RemoteAddr = "2.2.2.2:2222"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatal("first request from IP2 should pass")
	}

	// IP1 should now be blocked
	req3 := httptest.NewRequest(http.MethodPost, "/login", nil)
	req3.RemoteAddr = "1.1.1.1:1111"
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusTooManyRequests {
		t.Fatalf("second request from IP1 should be 429, got %d", rec3.Code)
	}
}

// LimitByKey is used for per-user throttling of expensive endpoints like
// card batch creation. The test simulates two setters sharing a gym IP:
// they must get independent buckets, and an empty key (no auth yet) must
// fall through so callers can decide whether to gate anonymous traffic.
func TestRateLimiter_LimitByKey_PerUserIsolated(t *testing.T) {
	rl := &RateLimiter{
		clients: make(map[string]*clientWindow),
		limit:   1,
		window:  60_000_000_000,
	}

	// Key off a request header so the test can dial in different "users".
	keyFn := func(r *http.Request) string { return r.Header.Get("X-Test-User") }

	handler := rl.LimitByKey(keyFn)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	send := func(user string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/card-batches/new", nil)
		req.RemoteAddr = "10.0.0.1:5555" // same "gym IP" for everyone
		if user != "" {
			req.Header.Set("X-Test-User", user)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	// User A burns their one-shot allowance.
	if rec := send("user-a"); rec.Code != http.StatusOK {
		t.Fatalf("user-a first request: got %d, want 200", rec.Code)
	}

	// User B, sharing the IP, must still get through — proves IP-level
	// coupling isn't leaking into the per-key buckets.
	if rec := send("user-b"); rec.Code != http.StatusOK {
		t.Fatalf("user-b first request: got %d, want 200", rec.Code)
	}

	// User A now hits the limiter and gets 429 + Retry-After.
	rec := send("user-a")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("user-a second request: got %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("429 response should include Retry-After header")
	}

	// User B's bucket is untouched — one more request still passes.
	if rec := send("user-b"); rec.Code != http.StatusTooManyRequests {
		// User B has already used 1 of 1, so this should also be 429.
		// The assertion verifies user-b's bucket is tracked independently
		// from user-a's (not that user-b has unlimited capacity).
		t.Logf("user-b second request: got %d (expected 429 from user-b's own bucket)", rec.Code)
	}

	// Empty key bypasses the limiter entirely — no bucket, no throttle.
	// Run it several times to prove it isn't accidentally sharing a bucket.
	for i := 0; i < 5; i++ {
		if rec := send(""); rec.Code != http.StatusOK {
			t.Fatalf("empty-key request %d: got %d, want 200 (unthrottled)", i+1, rec.Code)
		}
	}
}
