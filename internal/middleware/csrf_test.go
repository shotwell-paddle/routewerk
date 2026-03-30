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
