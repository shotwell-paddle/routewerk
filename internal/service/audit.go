package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/shotwell-paddle/routewerk/internal/jobs"
	"github.com/shotwell-paddle/routewerk/internal/middleware"
	"github.com/shotwell-paddle/routewerk/internal/repository"
)

// auditJobType is the job.JobType for queued audit writes. Keep the
// identifier stable — renaming it orphans pending jobs in the DB.
const auditJobType = "audit.record"

// AuditService provides a high-level interface for recording audit events.
// Every write operation in the system that modifies org/location/user data
// should call one of these methods.
//
// When a *jobs.Queue is attached (via AttachJobQueue or NewAuditService),
// Record enqueues the audit row and a background worker persists it. That
// keeps the request path off the audit_logs insert even when the table is
// under lock contention. Durability is preserved because the jobs table
// itself is part of the same Postgres DB, so the audit entry is written to
// stable storage before the enqueue call returns. If the queue is nil (used
// in tests, or as a conservative fallback when Enqueue errors), Record
// falls through to a synchronous insert. See S6 in the 2026-04-22 perf
// audit.
type AuditService struct {
	repo  *repository.AuditRepo
	queue *jobs.Queue
}

// NewAuditService creates the service. Pass a non-nil queue to route audit
// writes through the job queue (recommended in production). Pass nil in
// tests to keep writes inline.
func NewAuditService(repo *repository.AuditRepo, queue *jobs.Queue) *AuditService {
	s := &AuditService{repo: repo, queue: queue}
	if queue != nil {
		queue.Register(auditJobType, s.handleAuditJob)
	}
	return s
}

// Record logs an audit event, extracting the actor from the request
// context. Extra key-value pairs are stored as metadata. When a job queue
// is attached the insert is deferred to a background worker. A structured
// log line is always emitted inline so log aggregators see the event even
// if the DB write ultimately drops it.
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

	// Structured log line is independent of the DB write. Emit it first so
	// a slow or failing DB doesn't delay the log aggregator too.
	slog.Info("audit",
		"actor_id", actorID,
		"action", action,
		"resource", resource,
		"resource_id", resourceID,
		"org_id", orgID,
		"ip", r.RemoteAddr,
	)

	if s.queue != nil {
		payload, err := json.Marshal(entry)
		if err != nil {
			// Marshal failure is non-recoverable for the queue path — log
			// and fall through to the sync insert so we don't silently
			// drop the entry. Use a fresh background context with a short
			// timeout so the fallback doesn't inherit a cancelled request
			// context (client disconnect is a common trigger for this
			// branch; reusing the request context would double-fail).
			slog.Error("audit: marshal failed, falling back to sync",
				"error", err, "action", action)
			fallbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			s.repo.Log(fallbackCtx, entry)
			return
		}
		if _, err := s.queue.Enqueue(r.Context(), jobs.EnqueueParams{
			JobType: auditJobType,
			Payload: payload,
		}); err != nil {
			// Same rationale as above: the usual reason Enqueue fails is
			// the request context being cancelled/expired, so the sync
			// retry needs its own context.
			slog.Error("audit: enqueue failed, falling back to sync",
				"error", err, "action", action)
			fallbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			s.repo.Log(fallbackCtx, entry)
		}
		return
	}

	// No queue attached (tests, or NewAuditService caller passed nil) —
	// insert inline. Matches the pre-S6 behaviour.
	s.repo.Log(r.Context(), entry)
}

// handleAuditJob is the job handler that persists a queued audit entry.
// Returns nil on success so the job completes; returns an error to let the
// queue retry with backoff. AuditRepo.Log swallows DB errors into slog, so
// we detect failure by... we don't — this handler always returns nil, and
// any DB failure is surfaced through slog. That matches the pre-S6 "audit
// is best-effort, never blocks business ops" contract.
func (s *AuditService) handleAuditJob(ctx context.Context, job jobs.Job) error {
	var entry repository.AuditEntry
	if err := json.Unmarshal(job.Payload, &entry); err != nil {
		// Unrecoverable — malformed payload won't succeed on retry.
		// Log and return nil so the job completes rather than looping.
		slog.Error("audit: unmarshal payload failed, dropping",
			"error", err, "job_id", job.ID)
		return nil
	}
	s.repo.Log(ctx, entry)
	return nil
}

// ── Convenience constants for action names ──────────────────────

const (
	AuditOrgUpdate         = "org.update"
	AuditLocationCreate    = "location.create"
	AuditLocationUpdate    = "location.update"
	AuditWallCreate        = "wall.create"
	AuditWallUpdate        = "wall.update"
	AuditWallDelete        = "wall.delete"
	AuditRouteCreate       = "route.create"
	AuditRouteUpdate       = "route.update"
	AuditRouteStatusChange = "route.status_change"
	AuditRouteBulkArchive  = "route.bulk_archive"
	AuditSessionCreate     = "session.create"
	AuditSessionUpdate     = "session.update"
	AuditSessionAssign     = "session.assign"
	AuditTagCreate         = "tag.create"
	AuditTagDelete         = "tag.delete"
	AuditMemberAdd         = "member.add"
	AuditMemberRemove      = "member.remove"
	AuditMemberRoleChange  = "member.role_change"
	AuditLoginSuccess      = "auth.login"
	AuditLoginFailed       = "auth.login_failed"
	AuditAccountLocked     = "auth.account_locked"
	AuditCardBatchCreate   = "card_batch.create"
	AuditCardBatchDelete   = "card_batch.delete"
)
