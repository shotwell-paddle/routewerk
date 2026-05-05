//go:build !spa_embed

package spa

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStub_AssetServerReturns503(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/_app/foo.js", nil)
	AssetServer().ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "spa_embed") {
		t.Errorf("body should mention build tag; got: %q", rr.Body.String())
	}
}

func TestStub_FallbackReturns503(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/spa-test/", nil)
	FallbackHandler().ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}
}
