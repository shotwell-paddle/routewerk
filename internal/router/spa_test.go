//go:build spa_embed

package router

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/shotwell-paddle/routewerk/web/spa"
)

// TestSPARoutes verifies the chi wiring for the Phase 0 SPA mount points.
// The full router needs a DB pool, so this test builds a slim mux with just
// the SPA handlers registered the same way router.New() registers them.
// Keep this in lockstep with the SPA section in router.go.
func TestSPARoutes(t *testing.T) {
	r := chi.NewRouter()
	r.Handle("/_app/*", spa.AssetServer())
	r.Get("/favicon.svg", func(w http.ResponseWriter, req *http.Request) {
		spa.AssetServer().ServeHTTP(w, req)
	})
	r.Handle("/spa-test", http.RedirectHandler("/spa-test/", http.StatusMovedPermanently))
	r.Handle("/spa-test/*", spa.FallbackHandler())

	cases := []struct {
		name        string
		path        string
		wantStatus  int
		wantInBody  string
		wantHeader  string
		wantHeaderV string
	}{
		{
			name:        "spa fallback root serves html",
			path:        "/spa-test/",
			wantStatus:  http.StatusOK,
			wantInBody:  "<!DOCTYPE html",
			wantHeader:  "Content-Type",
			wantHeaderV: "text/html; charset=utf-8",
		},
		{
			name:        "spa fallback nested path serves same html",
			path:        "/spa-test/some/deep/client/route",
			wantStatus:  http.StatusOK,
			wantInBody:  "<!DOCTYPE html",
			wantHeader:  "Cache-Control",
			wantHeaderV: "no-cache",
		},
		{
			name:       "favicon resolves",
			path:       "/favicon.svg",
			wantStatus: http.StatusOK,
			wantInBody: "svg",
		},
		{
			name:       "spa-test without trailing slash redirects",
			path:       "/spa-test",
			wantStatus: http.StatusMovedPermanently,
		},
		{
			name:       "unknown root path is not handled by SPA",
			path:       "/unknown-root-path",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body: %q)", rr.Code, tc.wantStatus, rr.Body.String())
			}
			if tc.wantInBody != "" {
				body, _ := io.ReadAll(rr.Body)
				if !strings.Contains(string(body), tc.wantInBody) {
					t.Errorf("body missing %q; got: %q", tc.wantInBody, body)
				}
			}
			if tc.wantHeader != "" && rr.Header().Get(tc.wantHeader) != tc.wantHeaderV {
				t.Errorf("%s = %q, want %q", tc.wantHeader, rr.Header().Get(tc.wantHeader), tc.wantHeaderV)
			}
		})
	}
}

// TestSPAAssetMimeType verifies that asset responses get sensible content
// types via http.FileServer's automatic detection.
func TestSPAAssetMimeType(t *testing.T) {
	r := chi.NewRouter()
	r.Handle("/_app/*", spa.AssetServer())

	// Find one JS file in the embedded build to test against.
	jsPath := ""
	_ = fs.WalkDir(spa.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || jsPath != "" {
			return err
		}
		if strings.HasSuffix(p, ".js") {
			jsPath = p
		}
		return nil
	})
	if jsPath == "" {
		t.Skip("no JS file found in embedded build (placeholder mode?)")
	}

	req := httptest.NewRequest(http.MethodGet, "/"+jsPath, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "javascript") && !strings.Contains(ct, "text/plain") {
		t.Errorf("Content-Type for .js = %q, want javascript or text/plain", ct)
	}
}
