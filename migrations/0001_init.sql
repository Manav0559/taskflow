-- taskflow core schema
-- Design notes:
--   * job_runs.status transitions: pending -> leased -> running -> (succeeded|failed) -> (dead if attempts exhausted)
--   * Leasing uses SELECT ... FOR UPDATE SKIP LOCKED (see internal/worker) instead of a separate broker.
--     This is a deliberate CP-leaning choice for the scheduling metadata path: we already need a
--     transactional store for DAG state, so reusing Postgres as the queue avoids a second consistency
--     domain. Throughput ceiling and the Kafka/SQS alternative are discussed in docs/ARCHITECTURE.md.
--   * lease_expires_at + heartbeats let another worker reclaim a run if its owner dies mid-execution
--     (crash recovery), while idempotency_key lets job handlers dedupe a run that is executed twice.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS jobs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT NOT NULL,
    payload         JSONB NOT NULL DEFAULT '{}'::jsonb,
    cron_expr       TEXT,                -- NULL for one-shot jobs, e.g. "*/5 * * * *" for recurring
    priority        SMALLINT NOT NULL DEFAULT 0,  -- higher runs first
    max_attempts    SMALLINT NOT NULL DEFAULT 5,
    timeout_seconds INT NOT NULL DEFAULT 300,
    status          TEXT NOT NULL DEFAULT 'active', -- active | paused | archived
    idempotency_key TEXT UNIQUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS job_dependencies (
    job_id          UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    depends_on_id   UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    PRIMARY KEY (job_id, depends_on_id)
);

CREATE TABLE IF NOT EXISTS job_runs (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id            UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    status            TEXT NOT NULL DEFAULT 'pending', -- pending|leased|running|succeeded|failed|dead
    attempt           SMALLINT NOT NULL DEFAULT 0,
    priority          SMALLINT NOT NULL DEFAULT 0,
    scheduled_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    leased_by         TEXT,
    leased_at         TIMESTAMPTZ,
    lease_expires_at  TIMESTAMPTZ,
    started_at        TIMESTAMPTZ,
    finished_at       TIMESTAMPTZ,
    result            JSONB,
    error             TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_job_runs_lease_queue
    ON job_runs (status, priority DESC, scheduled_at)
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_job_runs_job_id ON job_runs (job_id);
CREATE INDEX IF NOT EXISTS idx_job_runs_lease_expiry ON job_runs (lease_expires_at) WHERE status IN ('leased', 'running');

CREATE TABLE IF NOT EXISTS workers (
    id              TEXT PRIMARY KEY,
    hostname        TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'alive', -- alive|dead
    last_heartbeat  TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS dead_letters (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_run_id  UUID NOT NULL REFERENCES job_runs(id) ON DELETE CASCADE,
    reason      TEXT NOT NULL,
    payload     JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
