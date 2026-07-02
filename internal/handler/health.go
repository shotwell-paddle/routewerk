package handler

import (
	"encoding/json"
	"net"
	"net/http"
	"net/netip"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shotwell-paddle/routewerk/internal/service"
)

type HealthHandler struct {
	db      *pgxpool.Pool
	storage *service.StorageService
	// backup is nil when server-side backups are disabled/unconfigured.
	backup *service.BackupService
}

func NewHealthHandler(db *pgxpool.Pool, storage *service.StorageService, backup *service.BackupService) *HealthHandler {
	return &HealthHandler{db: db, storage: storage, backup: backup}
}

// internalNetworks are the source networks allowed to see sensitive
// diagnostics (pool stats) on /health: the RFC1918 private ranges plus
// Fly's private 6PN network (fdaa::/16). Loopback is handled separately
// via Addr.IsLoopback in isInternalRequest.
var internalNetworks = []netip.Prefix{
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("fdaa::/16"),
}

// isInternalRequest returns true if the request comes from a private
// network (RFC1918, Fly's 6PN fdaa::/16) or loopback. Pool stats and
// other sensitive diagnostics are only returned for internal callers.
//
// The previous string-prefix check ("172.") matched ANY 172.x address —
// 172.5.0.0 is publicly routable; only 172.16.0.0/12 is RFC1918. Proper
// CIDR containment via net/netip closes that gap. RemoteAddr comes from
// the TCP peer or the TrustedClientIP middleware (Fly-Client-IP), never
// from a client-forgeable header.
func isInternalRequest(r *http.Request) bool {
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	// Drop any IPv6 zone (Prefix.Contains rejects zoned addrs) and unmap
	// IPv4-in-IPv6 (::ffff:10.0.0.1) so the IPv4 prefixes match either
	// representation.
	addr = addr.WithZone("").Unmap()
	if addr.IsLoopback() {
		return true
	}
	for _, network := range internalNetworks {
		if network.Contains(addr) {
			return true
		}
	}
	return false
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

	// Backup freshness is informational only (never affects status code):
	// a silently failing nightly backup was the main weakness of every
	// prior backup mode, so surface when the last one succeeded.
	if h.backup != nil {
		if ts, ok := h.backup.LastSuccess(); ok {
			result["last_backup"] = ts.Format("2006-01-02T15:04:05Z")
		} else {
			// Seeded from the bucket at scheduler start, so this means "no
			// backup exists at all" (or the seed hasn't run/failed) — not
			// merely "none since the last deploy".
			result["last_backup"] = "none"
		}
	}

	status, httpStatus := healthStatus(dbOK, storageConfigured, storageOK)
	result["status"] = status

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(result)
}
