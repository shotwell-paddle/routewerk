package webhandler

import (
	"strings"
	"testing"
)

func TestStaticPath_KnownFile(t *testing.T) {
	got := StaticPath("css/routewerk.css")
	if !strings.HasPrefix(got, "/static/css/routewerk.css?v=") {
		t.Errorf("StaticPath() = %q, expected /static/css/routewerk.css?v=<hash>", got)
	}
	// Hash should be 8 hex chars
	parts := strings.Split(got, "?v=")
	if len(parts) != 2 || len(parts[1]) != 8 {
		t.Errorf("expected 8-char hash, got %q", got)
	}
}

func TestStaticPath_Deterministic(t *testing.T) {
	a := StaticPath("css/routewerk.css")
	b := StaticPath("css/routewerk.css")
	if a != b {
		t.Error("StaticPath should return the same value for the same file")
	}
}

func TestStaticPath_DifferentFilesGetDifferentHashes(t *testing.T) {
	css := StaticPath("css/routewerk.css")
	js := StaticPath("js/app.js")
	cssHash := strings.Split(css, "?v=")[1]
	jsHash := strings.Split(js, "?v=")[1]
	if cssHash == jsHash {
		t.Error("different files should produce different hashes")
	}
}

func TestStaticPath_UnknownFileNoHash(t *testing.T) {
	got := StaticPath("nonexistent/file.xyz")
	if strings.Contains(got, "?v=") {
		t.Errorf("unknown file should not get a hash: %q", got)
	}
	if got != "/static/nonexistent/file.xyz" {
		t.Errorf("unexpected path: %q", got)
	}
}

func TestStaticPath_StripsLeadingSlash(t *testing.T) {
	a := StaticPath("/css/routewerk.css")
	b := StaticPath("css/routewerk.css")
	if a != b {
		t.Errorf("leading slash should be stripped: %q != %q", a, b)
	}
}

func TestFuncMapIncludesStaticPath(t *testing.T) {
	if funcMap["staticPath"] == nil {
		t.Error("funcMap should include staticPath")
	}
}
