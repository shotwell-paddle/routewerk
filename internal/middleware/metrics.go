package middleware

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
)

// Metrics collects lightweight request metrics in-process. No external
// dependencies (Prometheus, StatsD, etc.) — just atomic counters and a
// fixed-size latency histogram that can be scraped via JSON or Prometheus
// text format endpoints.
type Metrics struct {
	TotalRequests   atomic.Int64
	ActiveRequests  atomic.Int64
	TotalErrors     atomic.Int64 // 5xx responses
	TotalClientErrs atomic.Int64 // 4xx responses

	// Latency histogram: buckets in milliseconds.
	// Bucket[i] counts requests completing in <= bucketBounds[i] ms.
	buckets   [7]atomic.Int64
	startedAt time.Time

	statusCodes sync.Map // map[int]*atomic.Int64

	// Per-route pattern metrics: map[pattern]*routeMetrics
	routeMetrics sync.Map

	// Latency samples for percentile calculation (rolling window).
	// We keep the last maxSamples values and compute percentiles on read.
	samplesMu sync.Mutex
	samples   []int64 // milliseconds
}

var bucketBounds = [7]int64{5, 10, 25, 50, 100, 250, 1000} // ms

const maxSamples = 10000

// routeMetrics tracks per-route-pattern latency and request counts.
type routeMetrics struct {
	requests atomic.Int64
	errors   atomic.Int64 // 5xx
	// Latency buckets (same bounds as global)
	buckets [7]atomic.Int64
}

// NewMetrics creates a Metrics collector.
func NewMetrics() *Metrics {
	return &Metrics{
		startedAt: time.Now(),
		samples:   make([]int64, 0, maxSamples),
	}
}

// Collect is Chi middleware that records request count, latency, and status codes.
func (m *Metrics) Collect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.ActiveRequests.Add(1)
		m.TotalRequests.Add(1)
		start := time.Now()

		rw := &metricsResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)

		m.ActiveRequests.Add(-1)
		elapsed := time.Since(start).Milliseconds()

		// Record in global latency histogram
		for i, bound := range bucketBounds {
			if elapsed <= bound {
				m.buckets[i].Add(1)
				break
			}
		}

		// Store sample for percentile calculation
		m.samplesMu.Lock()
		if len(m.samples) >= maxSamples {
			// Evict oldest quarter to amortise shifts
			m.samples = m.samples[maxSamples/4:]
		}
		m.samples = append(m.samples, elapsed)
		m.samplesMu.Unlock()

		// Count status codes
		if rw.status >= 500 {
			m.TotalErrors.Add(1)
		} else if rw.status >= 400 {
			m.TotalClientErrs.Add(1)
		}
		m.incrementStatusCode(rw.status)

		// Per-route pattern metrics (chi-specific)
		if rctx := chi.RouteContext(r.Context()); rctx != nil {
			pattern := rctx.RoutePattern()
			if pattern != "" {
				rm := m.getOrCreateRouteMetrics(pattern)
				rm.requests.Add(1)
				if rw.status >= 500 {
					rm.errors.Add(1)
				}
				for i, bound := range bucketBounds {
					if elapsed <= bound {
						rm.buckets[i].Add(1)
						break
					}
				}
			}
		}
	})
}

func (m *Metrics) getOrCreateRouteMetrics(pattern string) *routeMetrics {
	if v, ok := m.routeMetrics.Load(pattern); ok {
		return v.(*routeMetrics)
	}
	rm := &routeMetrics{}
	actual, _ := m.routeMetrics.LoadOrStore(pattern, rm)
	return actual.(*routeMetrics)
}

func (m *Metrics) incrementStatusCode(code int) {
	if v, ok := m.statusCodes.Load(code); ok {
		v.(*atomic.Int64).Add(1)
		return
	}
	counter := &atomic.Int64{}
	counter.Add(1)
	m.statusCodes.Store(code, counter)
}

// percentiles returns p50, p95, p99 from the current sample window.
func (m *Metrics) percentiles() (p50, p95, p99 float64) {
	m.samplesMu.Lock()
	n := len(m.samples)
	if n == 0 {
		m.samplesMu.Unlock()
		return 0, 0, 0
	}
	// Copy to avoid holding lock during sort
	cp := make([]int64, n)
	copy(cp, m.samples)
	m.samplesMu.Unlock()

	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })

	pct := func(p float64) float64 {
		idx := int(math.Ceil(p*float64(n))) - 1
		if idx < 0 {
			idx = 0
		}
		return float64(cp[idx])
	}
	return pct(0.50), pct(0.95), pct(0.99)
}

// ── Snapshot (exported data for templates) ──────────────────────

// MetricsSnapshot holds a point-in-time copy of all metrics, suitable for
// rendering in HTML templates or serialising to JSON.
type MetricsSnapshot struct {
	UptimeSeconds  int
	TotalRequests  int64
	ActiveRequests int64
	TotalErrors    int64
	ClientErrors   int64
	ErrorRate      float64 // percentage of 5xx / total
	P50            float64
	P95            float64
	P99            float64
	StatusCodes    []StatusCodeCount
	Latency        []LatencyBucket
	Routes         []RouteSnapshot
}

// StatusCodeCount is a (code, count, pct) tuple.
type StatusCodeCount struct {
	Code  int
	Count int64
	Pct   float64 // percentage of total requests
}

// LatencyBucket is a histogram bucket.
type LatencyBucket struct {
	Label string // e.g. "≤5ms"
	Count int64
	Pct   float64 // percentage of total
}

// RouteSnapshot holds per-route metrics.
type RouteSnapshot struct {
	Pattern  string
	Requests int64
	Errors   int64
	AvgMs    float64
}

// Snapshot returns a point-in-time copy of all collected metrics.
func (m *Metrics) Snapshot() MetricsSnapshot {
	total := m.TotalRequests.Load()
	errors := m.TotalErrors.Load()

	var errorRate float64
	if total > 0 {
		errorRate = float64(errors) / float64(total) * 100
	}

	p50, p95, p99 := m.percentiles()

	snap := MetricsSnapshot{
		UptimeSeconds:  int(time.Since(m.startedAt).Seconds()),
		TotalRequests:  total,
		ActiveRequests: m.ActiveRequests.Load(),
		TotalErrors:    errors,
		ClientErrors:   m.TotalClientErrs.Load(),
		ErrorRate:      errorRate,
		P50:            p50,
		P95:            p95,
		P99:            p99,
	}

	// Status codes
	m.statusCodes.Range(func(key, value interface{}) bool {
		count := value.(*atomic.Int64).Load()
		var pct float64
		if total > 0 {
			pct = float64(count) / float64(total) * 100
		}
		snap.StatusCodes = append(snap.StatusCodes, StatusCodeCount{
			Code:  key.(int),
			Count: count,
			Pct:   pct,
		})
		return true
	})
	sort.Slice(snap.StatusCodes, func(i, j int) bool {
		return snap.StatusCodes[i].Code < snap.StatusCodes[j].Code
	})

	// Latency histogram
	for i, bound := range bucketBounds {
		count := m.buckets[i].Load()
		var pct float64
		if total > 0 {
			pct = float64(count) / float64(total) * 100
		}
		snap.Latency = append(snap.Latency, LatencyBucket{
			Label: fmt.Sprintf("≤%dms", bound),
			Count: count,
			Pct:   pct,
		})
	}

	// Per-route
	m.routeMetrics.Range(func(key, value interface{}) bool {
		rm := value.(*routeMetrics)
		reqs := rm.requests.Load()
		// Estimate avg from bucket midpoints
		var totalMs int64
		var counted int64
		prevBound := int64(0)
		for i, bound := range bucketBounds {
			c := rm.buckets[i].Load()
			midpoint := (prevBound + bound) / 2
			totalMs += midpoint * c
			counted += c
			prevBound = bound
		}
		var avg float64
		if counted > 0 {
			avg = float64(totalMs) / float64(counted)
		}
		snap.Routes = append(snap.Routes, RouteSnapshot{
			Pattern:  key.(string),
			Requests: reqs,
			Errors:   rm.errors.Load(),
			AvgMs:    avg,
		})
		return true
	})
	sort.Slice(snap.Routes, func(i, j int) bool {
		return snap.Routes[i].Requests > snap.Routes[j].Requests
	})

	return snap
}

// UptimeFormatted returns a human-readable uptime string.
func (m *Metrics) UptimeFormatted() string {
	d := time.Since(m.startedAt)
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	return fmt.Sprintf("%dm", mins)
}

// ── JSON Handler ────────────────────────────────────────────────

// Handler returns an http.HandlerFunc that serves the metrics as JSON.
func (m *Metrics) Handler(w http.ResponseWriter, r *http.Request) {
	// Content negotiation: Prometheus text format if requested
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "text/plain") || r.URL.Query().Get("format") == "prometheus" {
		m.prometheusHandler(w, r)
		return
	}

	type statusCount struct {
		Code  int   `json:"code"`
		Count int64 `json:"count"`
	}
	type latencyBucket struct {
		Le    string `json:"le"`
		Count int64  `json:"count"`
	}
	type routeSummary struct {
		Pattern  string          `json:"pattern"`
		Requests int64           `json:"requests"`
		Errors   int64           `json:"errors"`
		Latency  []latencyBucket `json:"latency"`
	}

	var statuses []statusCount
	m.statusCodes.Range(func(key, value interface{}) bool {
		statuses = append(statuses, statusCount{
			Code:  key.(int),
			Count: value.(*atomic.Int64).Load(),
		})
		return true
	})

	var latency []latencyBucket
	for i, bound := range bucketBounds {
		latency = append(latency, latencyBucket{
			Le:    time.Duration(bound * int64(time.Millisecond)).String(),
			Count: m.buckets[i].Load(),
		})
	}

	p50, p95, p99 := m.percentiles()

	var routes []routeSummary
	m.routeMetrics.Range(func(key, value interface{}) bool {
		rm := value.(*routeMetrics)
		var rl []latencyBucket
		for i, bound := range bucketBounds {
			rl = append(rl, latencyBucket{
				Le:    time.Duration(bound * int64(time.Millisecond)).String(),
				Count: rm.buckets[i].Load(),
			})
		}
		routes = append(routes, routeSummary{
			Pattern:  key.(string),
			Requests: rm.requests.Load(),
			Errors:   rm.errors.Load(),
			Latency:  rl,
		})
		return true
	})

	data := map[string]interface{}{
		"uptime_seconds":    int(time.Since(m.startedAt).Seconds()),
		"total_requests":    m.TotalRequests.Load(),
		"active_requests":   m.ActiveRequests.Load(),
		"total_5xx_errors":  m.TotalErrors.Load(),
		"total_4xx_errors":  m.TotalClientErrs.Load(),
		"status_codes":      statuses,
		"latency_histogram": latency,
		"latency_percentiles": map[string]float64{
			"p50": p50,
			"p95": p95,
			"p99": p99,
		},
		"routes": routes,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data) //nolint:errcheck
}

// ── Prometheus Text Format ──────────────────────────────────────

func (m *Metrics) prometheusHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	var b strings.Builder

	// Uptime
	fmt.Fprintf(&b, "# HELP routewerk_uptime_seconds Time since process start.\n")
	fmt.Fprintf(&b, "# TYPE routewerk_uptime_seconds gauge\n")
	fmt.Fprintf(&b, "routewerk_uptime_seconds %d\n", int(time.Since(m.startedAt).Seconds()))

	// Request counters
	fmt.Fprintf(&b, "# HELP routewerk_http_requests_total Total HTTP requests.\n")
	fmt.Fprintf(&b, "# TYPE routewerk_http_requests_total counter\n")
	fmt.Fprintf(&b, "routewerk_http_requests_total %d\n", m.TotalRequests.Load())

	fmt.Fprintf(&b, "# HELP routewerk_http_requests_active Currently in-flight requests.\n")
	fmt.Fprintf(&b, "# TYPE routewerk_http_requests_active gauge\n")
	fmt.Fprintf(&b, "routewerk_http_requests_active %d\n", m.ActiveRequests.Load())

	fmt.Fprintf(&b, "# HELP routewerk_http_errors_total Total 5xx errors.\n")
	fmt.Fprintf(&b, "# TYPE routewerk_http_errors_total counter\n")
	fmt.Fprintf(&b, "routewerk_http_errors_total %d\n", m.TotalErrors.Load())

	fmt.Fprintf(&b, "# HELP routewerk_http_client_errors_total Total 4xx errors.\n")
	fmt.Fprintf(&b, "# TYPE routewerk_http_client_errors_total counter\n")
	fmt.Fprintf(&b, "routewerk_http_client_errors_total %d\n", m.TotalClientErrs.Load())

	// Status code breakdown
	fmt.Fprintf(&b, "# HELP routewerk_http_responses_total Responses by status code.\n")
	fmt.Fprintf(&b, "# TYPE routewerk_http_responses_total counter\n")
	m.statusCodes.Range(func(key, value interface{}) bool {
		fmt.Fprintf(&b, "routewerk_http_responses_total{code=\"%d\"} %d\n",
			key.(int), value.(*atomic.Int64).Load())
		return true
	})

	// Global latency histogram
	fmt.Fprintf(&b, "# HELP routewerk_http_request_duration_ms_bucket Latency histogram.\n")
	fmt.Fprintf(&b, "# TYPE routewerk_http_request_duration_ms_bucket histogram\n")
	var cumulative int64
	for i, bound := range bucketBounds {
		cumulative += m.buckets[i].Load()
		fmt.Fprintf(&b, "routewerk_http_request_duration_ms_bucket{le=\"%d\"} %d\n", bound, cumulative)
	}
	fmt.Fprintf(&b, "routewerk_http_request_duration_ms_bucket{le=\"+Inf\"} %d\n", m.TotalRequests.Load())

	// Percentiles
	p50, p95, p99 := m.percentiles()
	fmt.Fprintf(&b, "# HELP routewerk_http_request_duration_ms Latency percentiles.\n")
	fmt.Fprintf(&b, "# TYPE routewerk_http_request_duration_ms summary\n")
	fmt.Fprintf(&b, "routewerk_http_request_duration_ms{quantile=\"0.5\"} %.1f\n", p50)
	fmt.Fprintf(&b, "routewerk_http_request_duration_ms{quantile=\"0.95\"} %.1f\n", p95)
	fmt.Fprintf(&b, "routewerk_http_request_duration_ms{quantile=\"0.99\"} %.1f\n", p99)

	// Per-route metrics
	fmt.Fprintf(&b, "# HELP routewerk_route_requests_total Requests per route pattern.\n")
	fmt.Fprintf(&b, "# TYPE routewerk_route_requests_total counter\n")
	m.routeMetrics.Range(func(key, value interface{}) bool {
		rm := value.(*routeMetrics)
		pattern := key.(string)
		fmt.Fprintf(&b, "routewerk_route_requests_total{pattern=\"%s\"} %d\n", pattern, rm.requests.Load())
		fmt.Fprintf(&b, "routewerk_route_errors_total{pattern=\"%s\"} %d\n", pattern, rm.errors.Load())
		return true
	})

	w.Write([]byte(b.String())) //nolint:errcheck
}

type metricsResponseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *metricsResponseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}
