package middleware

import (
	"log/slog"
	"net/http"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
)

type responseWriter struct {
	http.ResponseWriter
	status int
	size   int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.size += n
	return n, err
}

// Logger logs each request as structured JSON via slog. It pulls the
// request ID from chi's middleware and the authenticated user ID (if present)
// from the context.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)

		duration := time.Since(start)

		attrs := []slog.Attr{
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rw.status),
			slog.Duration("duration", duration),
			slog.Int("bytes", rw.size),
			slog.String("ip", r.RemoteAddr),
		}

		if reqID := chimw.GetReqID(r.Context()); reqID != "" {
			attrs = append(attrs, slog.String("request_id", reqID))
		}

		if userID := GetUserID(r.Context()); userID != "" {
			attrs = append(attrs, slog.String("user_id", userID))
		}

		if q := r.URL.RawQuery; q != "" {
			attrs = append(attrs, slog.String("query", q))
		}

		level := slog.LevelInfo
		if rw.status >= 500 {
			level = slog.LevelError
		} else if rw.status >= 400 {
			level = slog.LevelWarn
		}

		slog.LogAttrs(r.Context(), level, "http request", attrs...)
	})
}
