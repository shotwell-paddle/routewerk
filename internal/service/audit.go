package service

import (
	"context"
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

// Record logs an audit event. It extracts the actor from the request context.
// Extra key-value pairs are stored as metadata.
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

	// Write to DB (non-blocking — fire and forget on a goroutine so we
	// don't add latency to the request). Use a background context because
	// the request context may be cancelled after the response is sent.
	go func() {
		s.repo.Log(context.Background(), entry)
	}()

	// Also emit a structured log line so audit events are searchable in
	// log aggregators even if the DB write fails.
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
