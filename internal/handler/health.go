package handler

import (
	"encoding/json"
	"net/http"

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

	// DB connection pool stats — useful for dashboards and alerting on
	// pool exhaustion before it causes request timeouts.
	poolStat := h.db.Stat()
	result["db_pool"] = map[string]interface{}{
		"total_conns":          poolStat.TotalConns(),
		"idle_conns":           poolStat.IdleConns(),
		"acquired_conns":       poolStat.AcquiredConns(),
		"constructing_conns":   poolStat.ConstructingConns(),
		"max_conns":            poolStat.MaxConns(),
		"empty_acquire_count":  poolStat.EmptyAcquireCount(),
		"canceled_acquire_count": poolStat.CanceledAcquireCount(),
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
