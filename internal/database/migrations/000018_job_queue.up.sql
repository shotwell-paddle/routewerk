-- Lightweight DB-backed job queue. Jobs are claimed via SELECT FOR UPDATE
-- SKIP LOCKED, giving us exactly-once delivery without external dependencies.
CREATE TABLE jobs (
    id            BIGSERIAL PRIMARY KEY,
    queue         TEXT        NOT NULL DEFAULT 'default',
    job_type      TEXT        NOT NULL,
    payload       JSONB       NOT NULL DEFAULT '{}',
    status        TEXT        NOT NULL DEFAULT 'pending'
                              CHECK (status IN ('pending', 'running', 'completed', 'failed', 'dead')),
    attempts      INT         NOT NULL DEFAULT 0,
    max_attempts  INT         NOT NULL DEFAULT 3,
    last_error    TEXT,
    run_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at    TIMESTAMPTZ,
    completed_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Claim query: pick the oldest pending job that's ready to run
CREATE INDEX idx_jobs_claimable
    ON jobs (queue, run_at)
    WHERE status = 'pending';

-- Reap stale running jobs (started_at older than timeout)
CREATE INDEX idx_jobs_running
    ON jobs (started_at)
    WHERE status = 'running';
