package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
