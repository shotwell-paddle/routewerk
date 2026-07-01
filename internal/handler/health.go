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

// healthStatus computes the body "status" field and the HTTP status code
// from the component checks.
//
// The HTTP status code reflects ONLY process liveness + database
// reachability. fly.toml's [[http_service.checks]] treats any non-200 as
// machine-unhealthy and pulls it from routing; with min_machines_running = 1
// a transient storage (Tigris) blip would otherwise pull the only machine
// and turn a degraded-but-working app into a full outage. Storage health is
// still checked and reported in the JSON body for operators and monitoring,
// but it never changes the status code.
func healthStatus(dbOK, storageConfigured, storageOK bool) (string, int) {
	if !dbOK {
		return "degraded", http.StatusServiceUnavailable
	}
	if storageConfigured && !storageOK {
		return "degraded", http.StatusOK
	}
	return "ok", http.StatusOK
}

func (h *HealthHandler) Check(w http.ResponseWriter, r *http.Request) {
	result := map[string]interface{}{
		"database": "ok",
	}

	dbOK := true
	if err := h.db.Ping(r.Context()); err != nil {
		dbOK = false
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

	// Storage is checked and reported, but never affects the HTTP status —
	// see healthStatus for why.
	storageConfigured := h.storage != nil && h.storage.IsConfigured()
	storageOK := true
	if storageConfigured {
		result["storage"] = "ok"
		if !h.storage.Healthy(r.Context()) {
			storageOK = false
			result["storage"] = "degraded"
		}
	} else {
		result["storage"] = "not_configured"
	}

	status, httpStatus := healthStatus(dbOK, storageConfigured, storageOK)
	result["status"] = status

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(result)
}
