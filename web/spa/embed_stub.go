//go:build !spa_embed

// Stub for builds that don't embed the SPA. Lets `go build ./...` work on a
// fresh checkout without Node installed and without running `make spa-build`.
// All handlers respond with a 503 + actionable message.
package spa

import (
	"io/fs"
	"net/http"
	"testing/fstest"
)

const stubMessage = "spa not embedded — build with -tags=spa_embed after running make spa-build"

// FS returns an empty in-memory filesystem.
func FS() fs.FS {
	return fstest.MapFS{}
}

// AssetServer returns a handler that always responds 503 with a hint.
func AssetServer() http.Handler {
	return http.HandlerFunc(stubResponder)
}

// FallbackHandler returns a handler that always responds 503 with a hint.
func FallbackHandler() http.Handler {
	return http.HandlerFunc(stubResponder)
}

func stubResponder(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, stubMessage, http.StatusServiceUnavailable)
}
