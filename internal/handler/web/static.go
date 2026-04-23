package webhandler

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"sync"

	"github.com/shotwell-paddle/routewerk/web"
)

// assetHashes maps a static file path (e.g. "css/routewerk.css") to its
// content hash (first 8 hex characters of SHA-256). Computed once at init.
var (
	assetHashes     map[string]string
	assetHashesOnce sync.Once
)

// initAssetHashes walks the embedded static FS and computes a content hash
// for each file. Called lazily on first use.
func initAssetHashes() {
	assetHashesOnce.Do(func() {
		assetHashes = make(map[string]string)

		sub, err := fs.Sub(web.StaticFS, "static")
		if err != nil {
			return
		}

		fs.WalkDir(sub, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			data, readErr := fs.ReadFile(sub, path)
			if readErr != nil {
				return nil // skip unreadable files
			}
			h := sha256.Sum256(data)
			assetHashes[path] = fmt.Sprintf("%x", h[:4]) // 8 hex chars
			return nil
		})
	})
}

// StaticPath returns a versioned static asset URL. For example:
//
//	staticPath("css/routewerk.css") → "/static/css/routewerk.css?v=a1b2c3d4"
//
// If the file isn't found in the hash map (shouldn't happen), it returns
// the path without a version parameter.
func StaticPath(path string) string {
	initAssetHashes()
	path = strings.TrimPrefix(path, "/")
	if hash, ok := assetHashes[path]; ok {
		return "/static/" + path + "?v=" + hash
	}
	return "/static/" + path
}

// StaticHandler returns an http.Handler for /static/ files.
//
// Wraps http.FileServer on the embedded FS with ETag + If-None-Match
// handling so cache revalidations return 304 instead of re-sending the
// body. embed.FS has a zero modtime (FileServer emits no Last-Modified),
// so any shared proxy or private-mode reload currently full-downloads
// every asset. The hash lookup below reuses the content hashes already
// computed by initAssetHashes — constant-time per request, no extra I/O.
// See perf audit 2026-04-22 #6.
func StaticHandler() http.Handler {
	sub, err := fs.Sub(web.StaticFS, "static")
	if err != nil {
		panic("cannot access static FS: " + err.Error())
	}
	initAssetHashes()
	fileServer := http.StripPrefix("/static/", http.FileServer(http.FS(sub)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Path lookup key: "/static/css/routewerk.css" → "css/routewerk.css".
		key := strings.TrimPrefix(r.URL.Path, "/static/")
		if hash, ok := assetHashes[key]; ok {
			etag := `"` + hash + `"`
			w.Header().Set("ETag", etag)
			// RFC 7232 §3.2: If-None-Match wins over If-Modified-Since.
			// The incoming value can be a comma-separated list; our hashes
			// don't contain commas, so simple substring match is enough.
			if match := r.Header.Get("If-None-Match"); match != "" && strings.Contains(match, etag) {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}
		fileServer.ServeHTTP(w, r)
	})
}
