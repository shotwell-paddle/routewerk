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

	result := map[string]string{
		"status":   status,
		"database": "ok",
	}

	if err := h.db.Ping(r.Context()); err != nil {
		status = "degraded"
		result["database"] = "error"
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
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
