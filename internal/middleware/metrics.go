package middleware

import (
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Metrics collects lightweight request metrics in-process. No external
// dependencies (Prometheus, StatsD, etc.) — just atomic counters and a
// fixed-size latency histogram that can be scraped via a JSON endpoint.
type Metrics struct {
	TotalRequests   atomic.Int64
	ActiveRequests  atomic.Int64
	TotalErrors     atomic.Int64 // 5xx responses
	TotalClientErrs atomic.Int64 // 4xx responses

	// Latency histogram: buckets in milliseconds.
	// Bucket[i] counts requests completing in <= bucketBounds[i] ms.
	buckets     [7]atomic.Int64
	startedAt   time.Time
	statusCodes sync.Map // map[int]*atomic.Int64
}

var bucketBounds = [7]int64{5, 10, 25, 50, 100, 250, 1000} // ms

// NewMetrics creates a Metrics collector.
func NewMetrics() *Metrics {
	return &Metrics{startedAt: time.Now()}
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

		// Record in latency histogram
		for i, bound := range bucketBounds {
			if elapsed <= bound {
				m.buckets[i].Add(1)
				break
			}
		}

		// Count status codes
		if rw.status >= 500 {
			m.TotalErrors.Add(1)
		} else if rw.status >= 400 {
			m.TotalClientErrs.Add(1)
		}
		m.incrementStatusCode(rw.status)
	})
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

// Handler returns an http.HandlerFunc that serves the metrics as JSON.
func (m *Metrics) Handler(w http.ResponseWriter, r *http.Request) {
	type statusCount struct {
		Code  int   `json:"code"`
		Count int64 `json:"count"`
	}
	type latencyBucket struct {
		Le    string `json:"le"`
		Count int64  `json:"count"`
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

	data := map[string]interface{}{
		"uptime_seconds":     int(time.Since(m.startedAt).Seconds()),
		"total_requests":     m.TotalRequests.Load(),
		"active_requests":    m.ActiveRequests.Load(),
		"total_5xx_errors":   m.TotalErrors.Load(),
		"total_4xx_errors":   m.TotalClientErrs.Load(),
		"status_codes":       statuses,
		"latency_histogram":  latency,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data) //nolint:errcheck
}

type metricsResponseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *metricsResponseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}
