package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestTrustedClientIP verifies that only Fly-Client-IP (which Fly's proxy
// controls) can rewrite RemoteAddr — the client-forgeable X-Forwarded-For
// and X-Real-IP headers must be ignored, and garbage header values must
// leave the TCP peer address in place.
func TestTrustedClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		headers    map[string]string
		want       string
	}{
		{
			name:       "no headers leaves RemoteAddr untouched",
			remoteAddr: "10.0.0.1:4321",
			want:       "10.0.0.1:4321",
		},
		{
			name:       "Fly-Client-IP IPv4 honored",
			remoteAddr: "172.19.0.1:4321", // Fly proxy peer
			headers:    map[string]string{"Fly-Client-IP": "203.0.113.7"},
			want:       "203.0.113.7",
		},
		{
			name:       "Fly-Client-IP IPv6 honored and canonicalized",
			remoteAddr: "172.19.0.1:4321",
			headers:    map[string]string{"Fly-Client-IP": "2001:DB8:0:0:0:0:0:1"},
			want:       "2001:db8::1",
		},
		{
			name:       "Fly-Client-IP with surrounding whitespace honored",
			remoteAddr: "172.19.0.1:4321",
			headers:    map[string]string{"Fly-Client-IP": "  203.0.113.7  "},
			want:       "203.0.113.7",
		},
		{
			name:       "forged X-Forwarded-For ignored",
			remoteAddr: "198.51.100.9:55555",
			headers:    map[string]string{"X-Forwarded-For": "127.0.0.1"},
			want:       "198.51.100.9:55555",
		},
		{
			name:       "forged X-Real-IP ignored",
			remoteAddr: "198.51.100.9:55555",
			headers:    map[string]string{"X-Real-IP": "172.16.0.1"},
			want:       "198.51.100.9:55555",
		},
		{
			name:       "Fly-Client-IP wins over forged XFF",
			remoteAddr: "172.19.0.1:4321",
			headers: map[string]string{
				"Fly-Client-IP":   "203.0.113.7",
				"X-Forwarded-For": "127.0.0.1",
			},
			want: "203.0.113.7",
		},
		{
			name:       "garbage Fly-Client-IP ignored",
			remoteAddr: "198.51.100.9:55555",
			headers:    map[string]string{"Fly-Client-IP": "not-an-ip"},
			want:       "198.51.100.9:55555",
		},
		{
			name:       "XFF-style list in Fly-Client-IP ignored",
			remoteAddr: "198.51.100.9:55555",
			headers:    map[string]string{"Fly-Client-IP": "1.2.3.4, 5.6.7.8"},
			want:       "198.51.100.9:55555",
		},
		{
			name:       "empty Fly-Client-IP ignored",
			remoteAddr: "198.51.100.9:55555",
			headers:    map[string]string{"Fly-Client-IP": ""},
			want:       "198.51.100.9:55555",
		},
		{
			name:       "IP with port in Fly-Client-IP ignored",
			remoteAddr: "198.51.100.9:55555",
			headers:    map[string]string{"Fly-Client-IP": "203.0.113.7:80"},
			want:       "198.51.100.9:55555",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			h := TrustedClientIP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.RemoteAddr
			}))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tc.remoteAddr
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			h.ServeHTTP(httptest.NewRecorder(), req)

			if got != tc.want {
				t.Errorf("RemoteAddr = %q, want %q", got, tc.want)
			}
		})
	}
}
