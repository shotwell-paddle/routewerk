//go:build spa_embed

// Package spa embeds the SvelteKit static-adapter build output and exposes
// HTTP handlers for serving it. The SPA is built by `make spa-build` (which
// runs `npm run build` in this directory) and produces:
//
//	build/
//	  _app/...        ← content-hashed JS/CSS bundles, immutable
//	  favicon.svg     ← root-level static
//	  index.html      ← SPA fallback (one HTML file, client routes the rest)
//
// The embed is gated by the spa_embed build tag so plain `go build ./...`
// works on a fresh checkout (no Node, no SPA bundle). Production binaries
// pass -tags=spa_embed after running make spa-build; see Makefile,
// Dockerfile, and .github/workflows/ci.yml.
package spa

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
)

//go:embed all:build
var buildRoot embed.FS

// FS returns a filesystem rooted at the SPA build output. Paths inside it
// look like "_app/...", "favicon.svg", "index.html" — i.e. the build/
// prefix has been stripped.
func FS() fs.FS {
	sub, err := fs.Sub(buildRoot, "build")
	if err != nil {
		// Impossible: build/ is required at compile time by //go:embed above.
		panic("spa: missing build directory: " + err.Error())
	}
	return sub
}

// AssetServer returns an http.Handler that serves files directly from the
// embedded build FS. Mount it at the URL prefixes that match the build
// layout (e.g. "/_app/*", "/favicon.svg") so assets resolve at the absolute
// paths the SPA references.
func AssetServer() http.Handler {
	return http.FileServer(http.FS(FS()))
}

// FallbackHandler always serves the SPA's index.html. Mount it under any
// URL prefix the SPA owns; client-side routing handles the rest.
func FallbackHandler() http.Handler {
	root := FS()
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		f, err := root.Open("index.html")
		if err != nil {
			http.Error(w, "spa not built", http.StatusInternalServerError)
			return
		}
		defer f.Close()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = io.Copy(w, f)
	})
}
