package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shotwell-paddle/routewerk/internal/service"
)

type HealthHandler struct {
	db      *pgxpool.Pool
	storage *service.StorageService
}

func NewHealthHandler(db *pgxpool.Pool, storage *service.StorageService) *HealthHandler {
	return &HealthHandler{db: db, storage: storage}
}

// isInternalRequest returns true if the request comes from Fly.io's internal
// network (private 172.x, fdaa:: ranges) or localhost. Pool stats and other
// sensitive diagnostics are only returned for internal callers.
func isInternalRequest(r *http.Request) bool {
	ip := r.RemoteAddr
	if i := strings.LastIndex(ip, ":"); i != -1 {
		ip = ip[:i]
	}
	return strings.HasPrefix(ip, "172.") ||
		strings.HasPrefix(ip, "fdaa:") ||
		ip == "127.0.0.1" || ip == "::1"
}

func (h *HealthHandler) Check(w http.ResponseWriter, r *http.Request) {
	status := "ok"

	result := map[string]interface{}{
		"status":   status,
		"database": "ok",
	}

	if err := h.db.Ping(r.Context()); err != nil {
		status = "degraded"
		result["database"] = "error"
	}

	// Only expose pool details to internal callers (Fly health checks,
	// SSH console, etc.) — external clients see status only.
	if isInternalRequest(r) {
		poolStat := h.db.Stat()
		result["db_pool"] = map[string]interface{}{
			"total_conns":            poolStat.TotalConns(),
			"idle_conns":             poolStat.IdleConns(),
			"acquired_conns":         poolStat.AcquiredConns(),
			"constructing_conns":     poolStat.ConstructingConns(),
			"max_conns":              poolStat.MaxConns(),
			"empty_acquire_count":    poolStat.EmptyAcquireCount(),
			"canceled_acquire_count": poolStat.CanceledAcquireCount(),
		}
	}

	if h.storage != nil && h.storage.IsConfigured() {
		result["storage"] = "ok"
		if !h.storage.Healthy(r.Context()) {
			status = "degraded"
			result["storage"] = "error"
		}
	} else {
		result["storage"] = "not_configured"
	}

	result["status"] = status

	httpStatus := http.StatusOK
	if status == "degraded" {
		httpStatus = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(result)
}
