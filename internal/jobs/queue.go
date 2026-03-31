// Package jobs provides a lightweight, Postgres-backed job queue.
//
// Design:
//   - Jobs are stored in a `jobs` table and claimed via SELECT FOR UPDATE SKIP LOCKED.
//   - Workers poll at a configurable interval (default 5s).
//   - Failed jobs are retried with exponential backoff up to max_attempts.
//   - After max_attempts, the job status becomes "dead" for manual inspection.
//   - No external dependencies — just pgxpool.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Job represents a queued job.
type Job struct {
	ID          int64           `json:"id"`
	Queue       string          `json:"queue"`
	JobType     string          `json:"job_type"`
	Payload     json.RawMessage `json:"payload"`
	Status      string          `json:"status"`
	Attempts    int             `json:"attempts"`
	MaxAttempts int             `json:"max_attempts"`
	LastError   *string         `json:"last_error,omitempty"`
	RunAt       time.Time       `json:"run_at"`
	StartedAt   *time.Time      `json:"started_at,omitempty"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}

// Handler processes a single job. Return nil on success, an error to retry.
type Handler func(ctx context.Context, job Job) error

// EnqueueParams defines parameters for enqueuing a new job.
type EnqueueParams struct {
	Queue       string          // defaults to "default"
	JobType     string          // required
	Payload     json.RawMessage // JSON payload
	MaxAttempts int             // defaults to 3
	RunAt       time.Time       // defaults to now
}

// Queue is a Postgres-backed job queue.
type Queue struct {
	db       *pgxpool.Pool
	handlers map[string]Handler
	mu       sync.RWMutex
	pollInterval time.Duration
	staleTimeout time.Duration
}

// NewQueue creates a new job queue backed by the given database pool.
func NewQueue(db *pgxpool.Pool) *Queue {
	return &Queue{
		db:           db,
		handlers:     make(map[string]Handler),
		pollInterval: 5 * time.Second,
		staleTimeout: 10 * time.Minute,
	}
}

// Register adds a handler for a given job type.
func (q *Queue) Register(jobType string, h Handler) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.handlers[jobType] = h
}

// Enqueue inserts a new job into the queue.
func (q *Queue) Enqueue(ctx context.Context, p EnqueueParams) (int64, error) {
	if p.JobType == "" {
		return 0, errors.New("jobs: job_type is required")
	}
	if p.Queue == "" {
		p.Queue = "default"
	}
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = 3
	}
	if p.Payload == nil {
		p.Payload = json.RawMessage("{}")
	}
	if p.RunAt.IsZero() {
		p.RunAt = time.Now()
	}

	var id int64
	err := q.db.QueryRow(ctx,
		`INSERT INTO jobs (queue, job_type, payload, max_attempts, run_at)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id`,
		p.Queue, p.JobType, p.Payload, p.MaxAttempts, p.RunAt,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("jobs: enqueue %s: %w", p.JobType, err)
	}
	return id, nil
}

// Start begins polling for jobs in the background. Call the returned
// cancel function to stop the worker gracefully.
func (q *Queue) Start(ctx context.Context) context.CancelFunc {
	ctx, cancel := context.WithCancel(ctx)

	go q.pollLoop(ctx)
	go q.reapLoop(ctx)

	slog.Info("job queue started", "poll_interval", q.pollInterval, "stale_timeout", q.staleTimeout)
	return cancel
}

func (q *Queue) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(q.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for {
				// Process jobs in a tight loop until the queue is drained
				processed, err := q.processOne(ctx)
				if err != nil {
					if !errors.Is(err, context.Canceled) {
						slog.Error("job processing error", "error", err)
					}
					break
				}
				if !processed {
					break
				}
			}
		}
	}
}

// processOne claims and processes a single job. Returns true if a job
// was processed, false if the queue was empty.
func (q *Queue) processOne(ctx context.Context) (bool, error) {
	tx, err := q.db.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var job Job
	err = tx.QueryRow(ctx, `
		SELECT id, queue, job_type, payload, status, attempts, max_attempts,
		       last_error, run_at, started_at, completed_at, created_at
		FROM jobs
		WHERE status = 'pending' AND run_at <= NOW()
		ORDER BY run_at ASC
		LIMIT 1
		FOR UPDATE SKIP LOCKED
	`).Scan(
		&job.ID, &job.Queue, &job.JobType, &job.Payload, &job.Status,
		&job.Attempts, &job.MaxAttempts, &job.LastError,
		&job.RunAt, &job.StartedAt, &job.CompletedAt, &job.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim job: %w", err)
	}

	// Mark as running
	now := time.Now()
	_, err = tx.Exec(ctx,
		`UPDATE jobs SET status = 'running', started_at = $1, attempts = attempts + 1 WHERE id = $2`,
		now, job.ID,
	)
	if err != nil {
		return false, fmt.Errorf("mark running: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit claim: %w", err)
	}

	job.Attempts++
	job.Status = "running"

	// Look up handler
	q.mu.RLock()
	handler, ok := q.handlers[job.JobType]
	q.mu.RUnlock()

	if !ok {
		errMsg := fmt.Sprintf("no handler registered for job type %q", job.JobType)
		q.failJob(ctx, job, errMsg)
		slog.Warn("unhandled job type", "job_type", job.JobType, "job_id", job.ID)
		return true, nil
	}

	// Execute handler with a timeout
	execCtx, execCancel := context.WithTimeout(ctx, q.staleTimeout)
	defer execCancel()

	if err := handler(execCtx, job); err != nil {
		q.failJob(ctx, job, err.Error())
		slog.Warn("job failed", "job_type", job.JobType, "job_id", job.ID, "attempt", job.Attempts, "error", err)
	} else {
		q.completeJob(ctx, job)
		slog.Debug("job completed", "job_type", job.JobType, "job_id", job.ID)
	}

	return true, nil
}

func (q *Queue) completeJob(ctx context.Context, job Job) {
	_, err := q.db.Exec(ctx,
		`UPDATE jobs SET status = 'completed', completed_at = NOW() WHERE id = $1`,
		job.ID,
	)
	if err != nil {
		slog.Error("failed to mark job completed", "job_id", job.ID, "error", err)
	}
}

func (q *Queue) failJob(ctx context.Context, job Job, errMsg string) {
	if job.Attempts >= job.MaxAttempts {
		// Dead — no more retries
		_, err := q.db.Exec(ctx,
			`UPDATE jobs SET status = 'dead', last_error = $1, completed_at = NOW() WHERE id = $2`,
			errMsg, job.ID,
		)
		if err != nil {
			slog.Error("failed to mark job dead", "job_id", job.ID, "error", err)
		}
		return
	}

	// Exponential backoff: 30s, 2m, 8m, ...
	delay := time.Duration(math.Pow(4, float64(job.Attempts))) * 30 * time.Second
	if delay > 1*time.Hour {
		delay = 1 * time.Hour
	}
	runAt := time.Now().Add(delay)

	_, err := q.db.Exec(ctx,
		`UPDATE jobs SET status = 'pending', last_error = $1, run_at = $2 WHERE id = $3`,
		errMsg, runAt, job.ID,
	)
	if err != nil {
		slog.Error("failed to reschedule job", "job_id", job.ID, "error", err)
	}
}

// reapLoop finds stale "running" jobs that have exceeded the timeout and
// resets them to "pending" for retry.
func (q *Queue) reapLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cutoff := time.Now().Add(-q.staleTimeout)
			tag, err := q.db.Exec(ctx,
				`UPDATE jobs SET status = 'pending', last_error = 'stale: exceeded timeout'
				 WHERE status = 'running' AND started_at < $1`,
				cutoff,
			)
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					slog.Error("reap stale jobs failed", "error", err)
				}
				continue
			}
			if n := tag.RowsAffected(); n > 0 {
				slog.Warn("reaped stale jobs", "count", n)
			}
		}
	}
}

// Stats returns current job queue statistics.
func (q *Queue) Stats(ctx context.Context) (map[string]int64, error) {
	rows, err := q.db.Query(ctx,
		`SELECT status, COUNT(*) FROM jobs GROUP BY status`)
	if err != nil {
		return nil, fmt.Errorf("jobs: stats: %w", err)
	}
	defer rows.Close()

	stats := make(map[string]int64)
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		stats[status] = count
	}
	return stats, rows.Err()
}
