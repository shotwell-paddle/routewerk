package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime/debug"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// Recovery is a panic recovery middleware that logs the error and stack trace
// via slog, then returns a structured JSON error or a plain text error
// depending on the request's Accept header. It replaces chi's default
// Recoverer with structured logging.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rv := recover(); rv != nil {
				stack := string(debug.Stack())

				attrs := []slog.Attr{
					slog.Any("panic", rv),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.String("stack", stack),
				}
				if reqID := chimw.GetReqID(r.Context()); reqID != "" {
					attrs = append(attrs, slog.String("request_id", reqID))
				}
				if userID := GetUserID(r.Context()); userID != "" {
					attrs = append(attrs, slog.String("user_id", userID))
				}

				slog.LogAttrs(r.Context(), slog.LevelError, "panic recovered", attrs...)

				// Respond based on request type
				if isHTMXRequest(r) {
					w.Header().Set("Content-Type", "text/plain")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte("Something went wrong. Please try again.")) //nolint:errcheck
				} else if wantsJSON(r) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
						"error": "internal server error",
					})
				} else {
					http.Error(w, "Something went wrong. Please try again.", http.StatusInternalServerError)
				}
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func isHTMXRequest(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

func wantsJSON(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	path := r.URL.Path
	return accept == "application/json" || (len(path) >= 5 && path[:5] == "/api/")
}
