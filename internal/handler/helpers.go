package handler

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// maxBodySize limits request body to 1MB to prevent memory exhaustion.
const maxBodySize = 1 << 20 // 1 MB

// JSON writes a JSON response with the given status code.
func JSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

// Error writes a JSON error response.
func Error(w http.ResponseWriter, status int, message string) {
	JSON(w, status, map[string]string{"error": message})
}

// InternalError writes the generic 500 JSON AND logs the underlying cause
// with request context. Handlers previously called Error(w, 500, msg) and
// discarded err across ~200 sites — half the API produced undiagnosable
// 500s. Always pass the real error; the client still only sees message.
func InternalError(w http.ResponseWriter, r *http.Request, message string, err error) {
	attrs := []any{
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path),
		slog.String("msg", message),
	}
	if reqID := chimw.GetReqID(r.Context()); reqID != "" {
		attrs = append(attrs, slog.String("request_id", reqID))
	}
	if err != nil {
		attrs = append(attrs, slog.Any("error", err))
	}
	slog.Error("internal error", attrs...)
	JSON(w, http.StatusInternalServerError, map[string]string{"error": message})
}

// Decode reads a JSON request body into the target with a size limit.
func Decode(r *http.Request, target interface{}) error {
	r.Body = http.MaxBytesReader(nil, r.Body, maxBodySize)
	dec := json.NewDecoder(r.Body)
	err := dec.Decode(target)

	// Reject bodies with multiple JSON values (prevents request smuggling)
	if err == nil {
		if dec.More() {
			return io.ErrUnexpectedEOF
		}
	}

	return err
}

// clampPage parses limit/offset query values into sane bounds: limit
// defaults to def when missing/invalid/zero and is capped at max; offset
// floors at 0. Repos apply their own defaults for zero limits, but an
// unbounded client-supplied ?limit=1000000 previously reached the SQL
// unchecked on every list endpoint.
func clampPage(r *http.Request, def, max int) (limit, offset int) {
	limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ = strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = def
	}
	if limit > max {
		limit = max
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

var nonAlphaNum = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = nonAlphaNum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}
