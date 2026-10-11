// Package store defines the persistence contract used by the api, scheduler and worker
// services. The only production implementation is Postgres (internal/store/postgres.go),
// but code that only needs Store should depend on this interface, not the concrete type,
// so tests can substitute an in-memory fake.
package store

import (
	"context"
	"errors"
	"time"

	"github.com/manavsingla/taskflow/internal/model"
)

// ErrNotFound is returned by Get*/single-row lookups when nothing matches.
var ErrNotFound = errors.New("store: not found")

// ErrIdempotencyConflict is returned by CreateJob when the given idempotency key
// already exists for a different job.
var ErrIdempotencyConflict = errors.New("store: idempotency key already used")

// ErrLeaseLost is returned by the fenced run-lifecycle methods (ExtendLease,
// MarkRunning, CompleteRun, FailRun, MarkDead) when the run still exists but is no
// longer held by the caller's lease: the lease expired and the run was reclaimed,
// re-leased (possibly by the same worker ID, under a higher attempt), or already moved
// to a state the transition does not apply to. The write was not applied; the caller
// must drop the run rather than retry.
var ErrLeaseLost = errors.New("store: lease lost")

// Lease identifies one specific lease on a run and acts as its fencing token.
// LeaseNextRun increments job_runs.attempt in the same transaction that sets
// leased_by, so the (RunID, WorkerID, Attempt) triple is unique per lease: once the
// run is reclaimed or re-leased, writes carrying the old Lease match zero rows.
type Lease struct {
	RunID    string
	WorkerID string
	Attempt  int16
}

// LeaseOf returns the fencing token for run as leased by workerID.
func LeaseOf(run *model.JobRun, workerID string) Lease {
	return Lease{RunID: run.ID, WorkerID: workerID, Attempt: run.Attempt}
}

type Store interface {
	// --- Jobs ---
	CreateJob(ctx context.Context, in model.NewJobInput) (*model.Job, error)
	GetJob(ctx context.Context, id string) (*model.Job, error)
	// GetJobByIdempotencyKey returns ErrNotFound if no job has this key. Used by the API
	// to make POST /v1/jobs idempotent: a retried create with the same key returns the
	// original job instead of erroring or creating a duplicate.
	GetJobByIdempotencyKey(ctx context.Context, key string) (*model.Job, error)
	ListJobs(ctx context.Context, status *model.JobStatus, limit, offset int) ([]*model.Job, error)
	UpdateJobStatus(ctx context.Context, id string, status model.JobStatus) error
	ListDependencies(ctx context.Context, jobID string) ([]string, error)

	// --- Runs: scheduling / promotion ---
	// LatestRunForJob returns the most recently created run for a job, or ErrNotFound if none exists.
	LatestRunForJob(ctx context.Context, jobID string) (*model.JobRun, error)
	// HasActiveRun reports whether jobID has a run in pending/leased/running state
	// (used to avoid double-scheduling the same job concurrently).
	HasActiveRun(ctx context.Context, jobID string) (bool, error)
	// CreateRun inserts a new pending run for jobID.
	CreateRun(ctx context.Context, jobID string, priority int16, scheduledAt time.Time) (*model.JobRun, error)

	// --- Runs: worker lease lifecycle ---
	// LeaseNextRun atomically claims the highest-priority, oldest eligible pending run
	// (implemented via SELECT ... FOR UPDATE SKIP LOCKED) and marks it leased by workerID
	// until leaseDuration elapses. Returns (nil, nil, nil) if no run is available.
	LeaseNextRun(ctx context.Context, workerID string, leaseDuration time.Duration) (*model.JobRun, *model.Job, error)
	// The methods below are fenced by Lease: each UPDATE only applies while the row is
	// still leased by lease.WorkerID at lease.Attempt (and in a status the transition
	// applies to). They return ErrLeaseLost if the run exists but the lease no longer
	// matches, and ErrNotFound if the run does not exist.
	//
	// ExtendLease pushes lease_expires_at forward; called periodically by the worker
	// while a run is executing so a slow-but-alive worker isn't reaped.
	ExtendLease(ctx context.Context, lease Lease, extend time.Duration) error
	MarkRunning(ctx context.Context, lease Lease) error
	CompleteRun(ctx context.Context, lease Lease, result map[string]any) error
	// FailRun records an error. If requeue is true the run goes back to pending with
	// scheduled_at = now+backoff (the next LeaseNextRun increments attempt); otherwise
	// (attempts exhausted) the caller should follow up with MarkDead.
	FailRun(ctx context.Context, lease Lease, errMsg string, requeue bool, backoff time.Duration) error
	// MarkDead moves a run that this lease failed (status 'failed') to 'dead' and writes
	// a dead_letters row, in one transaction.
	MarkDead(ctx context.Context, lease Lease, reason string) error
	// ReclaimExpiredLeases resets any leased/running run whose lease has expired back
	// to pending (crash recovery for workers that died mid-execution). Returns the count reclaimed.
	ReclaimExpiredLeases(ctx context.Context) (int, error)

	GetRun(ctx context.Context, id string) (*model.JobRun, error)
	ListJobRuns(ctx context.Context, jobID string, limit int) ([]*model.JobRun, error)
	// CountPendingRuns is used to publish the taskflow_queue_depth gauge.
	CountPendingRuns(ctx context.Context) (int, error)

	// --- Workers ---
	UpsertWorkerHeartbeat(ctx context.Context, workerID, hostname string) error
	ListWorkers(ctx context.Context) ([]*model.Worker, error)

	Close()
}
