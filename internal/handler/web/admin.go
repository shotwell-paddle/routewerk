package webhandler

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"time"

	"github.com/shotwell-paddle/routewerk/internal/jobs"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
)

// AdminDeps bundles the dependencies the admin dashboard pages need that
// aren't part of the main web Handler (metrics, job queue, etc.).
type AdminDeps struct {
	Metrics  *middleware.Metrics
	JobQueue *jobs.Queue
}

// AdminHealthPage renders the /admin/health dashboard with dependency checks.
func (h *Handler) AdminHealthPage(deps *AdminDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		var checks []HealthCheckResult
		overall := "healthy"

		// 1. Database
		dbCheck := HealthCheckResult{Name: "Database", Status: "ok", Details: "Connected"}
		if err := h.db.Ping(ctx); err != nil {
			dbCheck.Status = "error"
			dbCheck.Details = "Connection failed"
			overall = "degraded"
		} else {
			stat := h.db.Stat()
			dbCheck.Details = formatPoolStats(stat.TotalConns(), stat.IdleConns(), stat.AcquiredConns(), stat.MaxConns())
		}
		checks = append(checks, dbCheck)

		// 2. Storage (S3)
		storageCheck := HealthCheckResult{Name: "Storage (S3)", Status: "ok", Details: "Reachable"}
		if h.storageService == nil || !h.storageService.IsConfigured() {
			storageCheck.Status = "not_configured"
			storageCheck.Details = "Not configured"
		} else if !h.storageService.Healthy(ctx) {
			storageCheck.Status = "error"
			storageCheck.Details = "Unreachable"
			overall = "degraded"
		}
		checks = append(checks, storageCheck)

		// 3. Job queue
		jobCheck := HealthCheckResult{Name: "Job Queue", Status: "ok", Details: "Running"}
		if deps.JobQueue != nil {
			stats, err := deps.JobQueue.Stats(ctx)
			if err != nil {
				jobCheck.Status = "error"
				jobCheck.Details = "Query failed"
				if overall == "healthy" {
					overall = "degraded"
				}
			} else {
				pending := stats["pending"]
				running := stats["running"]
				failed := stats["failed"]
				jobCheck.Details = formatJobStats(pending, running, failed)
				if failed > 10 {
					jobCheck.Status = "error"
					overall = "degraded"
				}
			}
		}
		checks = append(checks, jobCheck)

		// 4. Runtime info
		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)
		runtimeCheck := HealthCheckResult{
			Name:    "Runtime",
			Status:  "ok",
			Details: formatRuntimeStats(memStats.Alloc, runtime.NumGoroutine()),
		}
		checks = append(checks, runtimeCheck)

		data := &PageData{
			TemplateData:  templateDataFromContext(r, "admin-health"),
			HealthChecks:  checks,
			OverallHealth: overall,
			Uptime:        deps.Metrics.UptimeFormatted(),
		}
		h.render(w, r, "admin/health.html", data)
	}
}

// AdminMetricsPage renders the /admin/metrics dashboard.
func (h *Handler) AdminMetricsPage(deps *AdminDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snap := deps.Metrics.Snapshot()

		var jobStats map[string]int64
		if deps.JobQueue != nil {
			var err error
			jobStats, err = deps.JobQueue.Stats(r.Context())
			if err != nil {
				slog.Error("failed to load job stats for admin dashboard", "error", err)
			}
		}

		data := &PageData{
			TemplateData: templateDataFromContext(r, "admin-metrics"),
			MetricsData:  &snap,
			Uptime:       deps.Metrics.UptimeFormatted(),
			JobStats:     jobStats,
		}
		h.render(w, r, "admin/metrics.html", data)
	}
}

func formatPoolStats(total, idle, acquired, max int32) string {
	return fmt.Sprintf("%d/%d conns (%d idle, %d acquired)", total, max, idle, acquired)
}

func formatJobStats(pending, running, failed int64) string {
	return fmt.Sprintf("%d pending, %d running, %d failed", pending, running, failed)
}

func formatRuntimeStats(allocBytes uint64, goroutines int) string {
	mb := float64(allocBytes) / 1024 / 1024
	return fmt.Sprintf("%.1f MB heap, %d goroutines", mb, goroutines)
}
