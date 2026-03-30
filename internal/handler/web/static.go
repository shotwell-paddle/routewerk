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
func StaticHandler() http.Handler {
	sub, err := fs.Sub(web.StaticFS, "static")
	if err != nil {
		panic("cannot access static FS: " + err.Error())
	}
	return http.StripPrefix("/static/", http.FileServer(http.FS(sub)))
}
