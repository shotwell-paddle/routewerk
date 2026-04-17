package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestCSRF_GETSetsTokenCookie(t *testing.T) {
	csrf := NewCSRFProtection(true) // isDev=true, Secure=false
	handler := csrf.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Should set CSRF cookie
	cookies := rec.Result().Cookies()
	var found bool
	for _, c := range cookies {
		if c.Name == csrfCookieName {
			found = true
			if c.Value == "" {
				t.Error("CSRF cookie value should not be empty")
			}
			if c.HttpOnly {
				t.Error("CSRF cookie should NOT be HttpOnly (JS needs to read it)")
			}
			if c.Secure {
				t.Error("CSRF cookie should not be Secure in dev mode")
			}
		}
	}
	if !found {
		t.Error("CSRF cookie was not set on GET request")
	}
}

func TestCSRF_GETPassesWithoutToken(t *testing.T) {
	csrf := NewCSRFProtection(true)
	handler := csrf.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// GET with no cookie — should still pass
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET should pass without token, got %d", rec.Code)
	}
}

func TestCSRF_POSTWithoutTokenFails(t *testing.T) {
	csrf := NewCSRFProtection(true)
	handler := csrf.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "abc123"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST without token should be 403, got %d", rec.Code)
	}
}

func TestCSRF_POSTWithMatchingHeaderPasses(t *testing.T) {
	token := "test-csrf-token-value"
	csrf := NewCSRFProtection(true)
	var called bool
	handler := csrf.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: token})
	req.Header.Set(csrfHeaderName, token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST with matching header should pass, got %d", rec.Code)
	}
	if !called {
		t.Error("handler should have been called")
	}
}

func TestCSRF_POSTWithMatchingFormFieldPasses(t *testing.T) {
	token := "test-csrf-token-value"
	csrf := NewCSRFProtection(true)
	var called bool
	handler := csrf.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	form := url.Values{}
	form.Set(csrfFormField, token)
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: token})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST with matching form field should pass, got %d", rec.Code)
	}
	if !called {
		t.Error("handler should have been called")
	}
}

func TestCSRF_POSTWithMismatchedTokenFails(t *testing.T) {
	csrf := NewCSRFProtection(true)
	handler := csrf.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "real-token"})
	req.Header.Set(csrfHeaderName, "fake-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST with mismatched token should be 403, got %d", rec.Code)
	}
}

func TestCSRF_PUT_DELETE_PATCH_RequireToken(t *testing.T) {
	csrf := NewCSRFProtection(true)
	handler := csrf.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("%s should not reach handler without token", r.Method)
	}))

	for _, method := range []string{http.MethodPut, http.MethodDelete, http.MethodPatch} {
		req := httptest.NewRequest(method, "/resource", nil)
		req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "token"})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("%s without token should be 403, got %d", method, rec.Code)
		}
	}
}

func TestCSRF_TokenFromRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "my-token"})

	got := TokenFromRequest(req)
	if got != "my-token" {
		t.Errorf("TokenFromRequest = %q, want %q", got, "my-token")
	}

	// Without cookie
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	got2 := TokenFromRequest(req2)
	if got2 != "" {
		t.Errorf("TokenFromRequest with no cookie = %q, want empty", got2)
	}
}

func TestCSRF_ProductionSecureFlag(t *testing.T) {
	csrf := NewCSRFProtection(false) // isDev=false → Secure=true
	handler := csrf.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	for _, c := range rec.Result().Cookies() {
		if c.Name == csrfCookieName && !c.Secure {
			t.Error("CSRF cookie should have Secure flag in production")
		}
	}
}

// ── RotateCSRFToken ───────────────────────────────────────────────

func TestRotateCSRFToken_WritesNewCookie(t *testing.T) {
	rec := httptest.NewRecorder()

	token, err := RotateCSRFToken(rec, false)
	if err != nil {
		t.Fatalf("RotateCSRFToken returned error: %v", err)
	}
	if token == "" {
		t.Fatal("RotateCSRFToken returned empty token")
	}

	var found bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == csrfCookieName {
			found = true
			if c.Value != token {
				t.Errorf("cookie value = %q, want %q (matches returned token)", c.Value, token)
			}
			if c.HttpOnly {
				t.Error("rotated CSRF cookie must NOT be HttpOnly — JS reads it for HTMX headers")
			}
			if c.Secure {
				t.Error("rotated CSRF cookie should not be Secure when called with secure=false")
			}
			if c.SameSite != http.SameSiteStrictMode {
				t.Errorf("SameSite = %v, want Strict", c.SameSite)
			}
			if c.Path != "/" {
				t.Errorf("Path = %q, want \"/\"", c.Path)
			}
		}
	}
	if !found {
		t.Error("RotateCSRFToken did not emit a CSRF cookie on the response")
	}
}

func TestRotateCSRFToken_ProductionSetsSecure(t *testing.T) {
	rec := httptest.NewRecorder()

	if _, err := RotateCSRFToken(rec, true); err != nil {
		t.Fatalf("RotateCSRFToken returned error: %v", err)
	}

	for _, c := range rec.Result().Cookies() {
		if c.Name == csrfCookieName && !c.Secure {
			t.Error("rotated CSRF cookie should have Secure flag when called with secure=true")
		}
	}
}

func TestRotateCSRFToken_InvalidatesOldToken(t *testing.T) {
	// Simulate a session-fixation scenario: an attacker knows the victim's
	// pre-login CSRF token. After login, Rotate is called — the cookie now
	// carries a new value and the old one no longer matches. Downstream
	// Protect() middleware runs ConstantTimeCompare against the cookie, so
	// submitting the attacker's old token would fail.
	rec := httptest.NewRecorder()

	oldToken := "attacker-known-token"
	newToken, err := RotateCSRFToken(rec, false)
	if err != nil {
		t.Fatalf("RotateCSRFToken: %v", err)
	}

	if newToken == oldToken {
		t.Error("rotated token equals attacker-known token — rotation didn't occur")
	}

	// Verify the cookie written is the new token, not the old one.
	cookies := rec.Result().Cookies()
	var got string
	for _, c := range cookies {
		if c.Name == csrfCookieName {
			got = c.Value
		}
	}
	if got != newToken {
		t.Errorf("cookie value %q != new token %q", got, newToken)
	}
	if got == oldToken {
		t.Error("cookie still contains the old (pre-rotation) token")
	}
}

func TestRotateCSRFToken_EachCallDistinct(t *testing.T) {
	// Rotation must produce a fresh random token every call.
	rec1 := httptest.NewRecorder()
	t1, err := RotateCSRFToken(rec1, false)
	if err != nil {
		t.Fatalf("call 1: %v", err)
	}
	rec2 := httptest.NewRecorder()
	t2, err := RotateCSRFToken(rec2, false)
	if err != nil {
		t.Fatalf("call 2: %v", err)
	}
	if t1 == t2 {
		t.Errorf("two Rotate calls returned identical tokens %q — entropy source broken?", t1)
	}
	// silence unused imports if refactored
	_ = url.QueryEscape
	_ = strings.Contains
}
