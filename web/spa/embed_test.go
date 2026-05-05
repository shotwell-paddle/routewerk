//go:build spa_embed

package spa

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFS_HasIndexHTML(t *testing.T) {
	f, err := FS().Open("index.html")
	if err != nil {
		t.Fatalf("index.html missing from embedded build: %v", err)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	if len(b) == 0 {
		t.Fatal("index.html is empty")
	}
	if !strings.Contains(strings.ToLower(string(b)), "<!doctype html>") {
		clip := len(b)
		if clip > 200 {
			clip = 200
		}
		t.Errorf("index.html doesn't look like HTML; first %d bytes: %q", clip, b[:clip])
	}
}

func TestFallbackHandler_ServesIndex(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/anything/at/all", nil)
	FallbackHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Errorf("Content-Type = %q, want text/html...", got)
	}
	if got := rr.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", got)
	}
}

func TestAssetServer_ServesEmbedFiles(t *testing.T) {
	root := FS()
	srv := AssetServer()

	var checked int
	err := fs.WalkDir(root, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || path == "index.html" {
			return nil
		}
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/"+path, nil)
		srv.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("GET /%s → %d, want 200", path, rr.Code)
		}
		checked++
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if checked == 0 {
		t.Fatal("no embedded asset files found to test (placeholder mode?)")
	}
}
