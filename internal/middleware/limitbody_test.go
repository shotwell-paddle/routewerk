package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestLimitBodyByContentType verifies the split cap: JSON requests are held
// to the default limit while multipart uploads get the higher one. The
// handler reads the full body, which is where MaxBytesReader trips.
func TestLimitBodyByContentType(t *testing.T) {
	const defaultMax = 1 << 10   // 1 KB
	const multipartMax = 4 << 10 // 4 KB

	tests := []struct {
		name        string
		contentType string
		bodySize    int
		wantReadErr bool
	}{
		{"json under default cap", "application/json", 512, false},
		{"json over default cap", "application/json", 2 << 10, true},
		{"multipart under multipart cap", "multipart/form-data; boundary=xyz", 2 << 10, false},
		{"multipart over multipart cap", "multipart/form-data; boundary=xyz", 8 << 10, true},
		{"no content type over default cap", "", 2 << 10, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var readErr error
			handler := LimitBodyByContentType(defaultMax, multipartMax)(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, readErr = io.ReadAll(r.Body)
				}),
			)

			body := bytes.Repeat([]byte("a"), tt.bodySize)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/test", bytes.NewReader(body))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			handler.ServeHTTP(httptest.NewRecorder(), req)

			if tt.wantReadErr && readErr == nil {
				t.Errorf("expected body read to fail over cap, got nil")
			}
			if !tt.wantReadErr && readErr != nil {
				t.Errorf("expected body read to succeed, got %v", readErr)
			}
		})
	}
}
