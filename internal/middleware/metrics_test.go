package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetrics_CountsRequests(t *testing.T) {
	m := NewMetrics()
	handler := m.Collect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	if got := m.TotalRequests.Load(); got != 5 {
		t.Errorf("TotalRequests = %d, want 5", got)
	}
	if got := m.ActiveRequests.Load(); got != 0 {
		t.Errorf("ActiveRequests = %d, want 0 (all completed)", got)
	}
}

func TestMetrics_Counts5xxErrors(t *testing.T) {
	m := NewMetrics()
	handler := m.Collect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	req := httptest.NewRequest(http.MethodGet, "/fail", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := m.TotalErrors.Load(); got != 1 {
		t.Errorf("TotalErrors = %d, want 1", got)
	}
}

func TestMetrics_Counts4xxErrors(t *testing.T) {
	m := NewMetrics()
	handler := m.Collect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := m.TotalClientErrs.Load(); got != 1 {
		t.Errorf("TotalClientErrs = %d, want 1", got)
	}
}

func TestMetrics_HandlerReturnsJSON(t *testing.T) {
	m := NewMetrics()

	// Send a request through the collector first
	collector := m.Collect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	collector.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	// Now hit the metrics handler
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	m.Handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse metrics JSON: %v", err)
	}

	if result["total_requests"].(float64) != 1 {
		t.Errorf("total_requests = %v, want 1", result["total_requests"])
	}
	if result["uptime_seconds"] == nil {
		t.Error("uptime_seconds should be present")
	}
	if result["latency_histogram"] == nil {
		t.Error("latency_histogram should be present")
	}
}

func TestMetrics_LatencyHistogramPopulated(t *testing.T) {
	m := NewMetrics()
	handler := m.Collect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Fast request should land in a low bucket
	req := httptest.NewRequest(http.MethodGet, "/fast", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	// At least one bucket should have a count > 0
	hasCount := false
	for i := range m.buckets {
		if m.buckets[i].Load() > 0 {
			hasCount = true
			break
		}
	}
	if !hasCount {
		t.Error("at least one latency bucket should be populated")
	}
}

func TestMetrics_Percentiles(t *testing.T) {
	m := NewMetrics()

	// Manually inject samples instead of going through HTTP
	m.samplesMu.Lock()
	for i := int64(1); i <= 100; i++ {
		m.samples = append(m.samples, i)
	}
	m.samplesMu.Unlock()

	p50, p95, p99 := m.percentiles()

	if p50 != 50 {
		t.Errorf("p50 = %.1f, want 50", p50)
	}
	if p95 != 95 {
		t.Errorf("p95 = %.1f, want 95", p95)
	}
	if p99 != 99 {
		t.Errorf("p99 = %.1f, want 99", p99)
	}
}

func TestMetrics_PercentilesEmpty(t *testing.T) {
	m := NewMetrics()
	p50, p95, p99 := m.percentiles()
	if p50 != 0 || p95 != 0 || p99 != 0 {
		t.Errorf("empty percentiles should all be 0, got p50=%.1f p95=%.1f p99=%.1f", p50, p95, p99)
	}
}

func TestMetrics_JSONIncludesPercentiles(t *testing.T) {
	m := NewMetrics()

	// Inject samples
	m.samplesMu.Lock()
	for i := int64(1); i <= 50; i++ {
		m.samples = append(m.samples, i)
	}
	m.samplesMu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	m.Handler(rec, req)

	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("parse JSON: %v", err)
	}

	pctls, ok := result["latency_percentiles"].(map[string]interface{})
	if !ok {
		t.Fatal("latency_percentiles missing from JSON response")
	}

	for _, key := range []string{"p50", "p95", "p99"} {
		if _, exists := pctls[key]; !exists {
			t.Errorf("percentile %q missing", key)
		}
	}
}

func TestMetrics_PrometheusFormat(t *testing.T) {
	m := NewMetrics()

	collector := m.Collect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	collector.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	// Request Prometheus format via Accept header
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Accept", "text/plain")
	rec := httptest.NewRecorder()
	m.Handler(rec, req)

	body := rec.Body.String()

	if !strings.Contains(rec.Header().Get("Content-Type"), "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", rec.Header().Get("Content-Type"))
	}

	// Check for key metric lines
	expected := []string{
		"routewerk_uptime_seconds",
		"routewerk_http_requests_total",
		"routewerk_http_requests_active",
		"routewerk_http_errors_total",
		"routewerk_http_request_duration_ms_bucket",
		"routewerk_http_request_duration_ms{quantile=",
	}
	for _, exp := range expected {
		if !strings.Contains(body, exp) {
			t.Errorf("Prometheus output missing %q", exp)
		}
	}
}

func TestMetrics_PrometheusFormatQueryParam(t *testing.T) {
	m := NewMetrics()

	req := httptest.NewRequest(http.MethodGet, "/metrics?format=prometheus", nil)
	rec := httptest.NewRecorder()
	m.Handler(rec, req)

	if !strings.Contains(rec.Header().Get("Content-Type"), "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", rec.Header().Get("Content-Type"))
	}
}

func TestMetrics_SampleEviction(t *testing.T) {
	m := NewMetrics()

	// Fill beyond maxSamples
	m.samplesMu.Lock()
	for i := 0; i < maxSamples+100; i++ {
		m.samples = append(m.samples, int64(i))
	}
	m.samplesMu.Unlock()

	// Trigger eviction via Collect
	handler := m.Collect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	m.samplesMu.Lock()
	n := len(m.samples)
	m.samplesMu.Unlock()

	if n > maxSamples {
		t.Errorf("samples = %d, should be <= %d after eviction", n, maxSamples)
	}
}

func TestMetrics_StatusCodeTracking(t *testing.T) {
	m := NewMetrics()

	handler := m.Collect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.WriteHeader(200)
		case "/created":
			w.WriteHeader(201)
		case "/notfound":
			w.WriteHeader(404)
		case "/error":
			w.WriteHeader(500)
		}
	}))

	for _, path := range []string{"/ok", "/ok", "/created", "/notfound", "/error"} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}

	// Verify counts via JSON output
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	m.Handler(rec, req)

	var result map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result["total_5xx_errors"].(float64) != 1 {
		t.Errorf("5xx errors = %v, want 1", result["total_5xx_errors"])
	}
	if result["total_4xx_errors"].(float64) != 1 {
		t.Errorf("4xx errors = %v, want 1", result["total_4xx_errors"])
	}
}
