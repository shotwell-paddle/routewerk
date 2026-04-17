package service

import (
	"log/slog"
	"net/http"

	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

// AuditService provides a high-level interface for recording audit events.
// Every write operation in the system that modifies org/location/user data
// should call one of these methods.
type AuditService struct {
	repo *repository.AuditRepo
}

func NewAuditService(repo *repository.AuditRepo) *AuditService {
	return &AuditService{repo: repo}
}

// Record logs an audit event synchronously. It extracts the actor from the
// request context. Extra key-value pairs are stored as metadata.
//
// The DB write happens inline on the request context so:
//   - Its deadline is bound to the HTTP request (can't leak past shutdown).
//   - Audit writes complete before the caller sees success, so we can't
//     ack a state change without a durable audit trail.
//   - Failures are still swallowed (logged, not returned) so a transient
//     audit-table outage doesn't cascade into user-visible errors.
//
// The trade-off vs. the previous fire-and-forget goroutine is a small (~1
// round-trip) latency bump per state-changing request. Ratings and other
// read-only endpoints don't Record, so this stays off the hot path.
func (s *AuditService) Record(r *http.Request, action, resource, resourceID, orgID string, meta map[string]interface{}) {
	actorID := middleware.GetUserID(r.Context())
	if actorID == "" {
		actorID = "system"
	}

	entry := repository.AuditEntry{
		ActorID:    actorID,
		Action:     action,
		Resource:   resource,
		ResourceID: resourceID,
		OrgID:      orgID,
		Metadata:   meta,
		IPAddress:  r.RemoteAddr,
	}

	// Synchronous insert. AuditRepo.Log swallows its own errors into slog
	// so a DB hiccup doesn't turn a successful business op into a 500.
	s.repo.Log(r.Context(), entry)

	// Structured log line as a second, independent audit surface. Even if
	// the DB write was dropped (e.g. audit_logs locked), log aggregators
	// still see the event.
	slog.Info("audit",
		"actor_id", actorID,
		"action", action,
		"resource", resource,
		"resource_id", resourceID,
		"org_id", orgID,
		"ip", r.RemoteAddr,
	)
}

// ── Convenience constants for action names ──────────────────────

const (
	AuditOrgUpdate        = "org.update"
	AuditLocationCreate   = "location.create"
	AuditLocationUpdate   = "location.update"
	AuditWallCreate       = "wall.create"
	AuditWallUpdate       = "wall.update"
	AuditWallDelete       = "wall.delete"
	AuditRouteCreate      = "route.create"
	AuditRouteUpdate      = "route.update"
	AuditRouteStatusChange = "route.status_change"
	AuditRouteBulkArchive = "route.bulk_archive"
	AuditSessionCreate    = "session.create"
	AuditSessionUpdate    = "session.update"
	AuditSessionAssign    = "session.assign"
	AuditTagCreate        = "tag.create"
	AuditTagDelete        = "tag.delete"
	AuditMemberAdd        = "member.add"
	AuditMemberRemove     = "member.remove"
	AuditMemberRoleChange = "member.role_change"
	AuditLoginSuccess     = "auth.login"
	AuditLoginFailed      = "auth.login_failed"
	AuditAccountLocked    = "auth.account_locked"
)
