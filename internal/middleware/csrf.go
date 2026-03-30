package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"sync"
)

const (
	csrfTokenLength = 32
	csrfCookieName  = "_csrf"
	csrfHeaderName  = "X-CSRF-Token"
	csrfFormField   = "_csrf_token"
)

// CSRFProtection provides double-submit cookie CSRF protection.
// For every request it ensures a CSRF cookie exists, and for state-changing
// methods (POST, PUT, PATCH, DELETE) it validates the token from either the
// form body or the X-CSRF-Token header matches the cookie value.
//
// Templates should include the token as a hidden field:
//
//	<input type="hidden" name="_csrf_token" value="{{.CSRFToken}}">
//
// HTMX requests can use the header instead via hx-headers.
type CSRFProtection struct {
	secure bool // set cookie Secure flag (true in production)
}

// NewCSRFProtection creates CSRF middleware. Pass isDev=true to skip Secure flag.
func NewCSRFProtection(isDev bool) *CSRFProtection {
	return &CSRFProtection{secure: !isDev}
}

// csrfTokenPool avoids allocations on hot path.
var csrfTokenPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, csrfTokenLength)
		return &b
	},
}

func generateCSRFToken() (string, error) {
	bp := csrfTokenPool.Get().(*[]byte)
	defer csrfTokenPool.Put(bp)
	b := *bp
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Protect returns middleware that enforces CSRF on state-changing methods.
func (c *CSRFProtection) Protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Ensure CSRF cookie exists
		cookie, err := r.Cookie(csrfCookieName)
		if err != nil || cookie.Value == "" {
			token, genErr := generateCSRFToken()
			if genErr != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			http.SetCookie(w, &http.Cookie{
				Name:     csrfCookieName,
				Value:    token,
				Path:     "/",
				HttpOnly: false, // JS needs to read it for HTMX headers
				Secure:   c.secure,
				SameSite: http.SameSiteStrictMode,
			})
			cookie = &http.Cookie{Name: csrfCookieName, Value: token}
		}

		// Safe methods — no validation needed
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
			next.ServeHTTP(w, r)
			return
		}

		// State-changing method — validate token
		submitted := r.Header.Get(csrfHeaderName)
		if submitted == "" {
			submitted = r.FormValue(csrfFormField)
		}

		if submitted == "" || subtle.ConstantTimeCompare([]byte(submitted), []byte(cookie.Value)) != 1 {
			http.Error(w, "CSRF validation failed", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// TokenFromRequest extracts the current CSRF token from the request cookie.
// Used by handlers to inject the token into template data.
func TokenFromRequest(r *http.Request) string {
	cookie, err := r.Cookie(csrfCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}
