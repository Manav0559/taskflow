package scheduler

import (
	"context"
	"sync"
	"time"

	"github.com/manavsingla/taskflow/internal/model"
	"github.com/manavsingla/taskflow/internal/store"
)

// fakeStore is a minimal in-memory implementation of store.Store used to unit test
// scheduler logic (NextRunDue, DependenciesSatisfied, PromoteOnce) without a real
// Postgres. Only the behavior the scheduler package actually exercises is real;
// everything else is a zero-value stub so the type satisfies store.Store.
type fakeStore struct {
	mu sync.Mutex

	jobs      map[string]*model.Job
	dependsOn map[string][]string // jobID -> depends on these jobIDs

	runs         []*model.JobRun
	activeRunJob map[string]bool // jobID -> has an active (pending/leased/running) run
	nextRunID    int
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		jobs:         make(map[string]*model.Job),
		dependsOn:    make(map[string][]string),
		activeRunJob: make(map[string]bool),
	}
}

// addJob registers a job (and its dependency list) directly into the fake store,
// bypassing CreateJob's idempotency-key bookkeeping which scheduler tests don't need.
func (f *fakeStore) addJob(job *model.Job, dependsOn ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.jobs[job.ID] = job
	if len(dependsOn) > 0 {
		f.dependsOn[job.ID] = dependsOn
	}
}

// --- Jobs ---

func (f *fakeStore) CreateJob(ctx context.Context, in model.NewJobInput) (*model.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	id := "job-" + in.Name
	job := &model.Job{
		ID:             id,
		Name:           in.Name,
		Payload:        in.Payload,
		CronExpr:       in.CronExpr,
		Priority:       in.Priority,
		MaxAttempts:    in.MaxAttempts,
		TimeoutSeconds: in.TimeoutSeconds,
		Status:         model.JobStatusActive,
		IdempotencyKey: in.IdempotencyKey,
		DependsOn:      in.DependsOn,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	f.jobs[id] = job
	if len(in.DependsOn) > 0 {
		f.dependsOn[id] = in.DependsOn
	}
	return job, nil
}

func (f *fakeStore) GetJob(ctx context.Context, id string) (*model.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	job, ok := f.jobs[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	return job, nil
}

func (f *fakeStore) GetJobByIdempotencyKey(ctx context.Context, key string) (*model.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, job := range f.jobs {
		if job.IdempotencyKey != nil && *job.IdempotencyKey == key {
			return job, nil
		}
	}
	return nil, store.ErrNotFound
}

func (f *fakeStore) ListJobs(ctx context.Context, status *model.JobStatus, limit, offset int) ([]*model.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var out []*model.Job
	for _, job := range f.jobs {
		if status != nil && job.Status != *status {
			continue
		}
		out = append(out, job)
	}
	return out, nil
}

func (f *fakeStore) UpdateJobStatus(ctx context.Context, id string, status model.JobStatus) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	job, ok := f.jobs[id]
	if !ok {
		return store.ErrNotFound
	}
	job.Status = status
	return nil
}

func (f *fakeStore) ListDependencies(ctx context.Context, jobID string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.dependsOn[jobID], nil
}

// --- Runs: scheduling / promotion ---

func (f *fakeStore) LatestRunForJob(ctx context.Context, jobID string) (*model.JobRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var latest *model.JobRun
	for _, run := range f.runs {
		if run.JobID != jobID {
			continue
		}
		if latest == nil || run.CreatedAt.After(latest.CreatedAt) {
			latest = run
		}
	}
	if latest == nil {
		return nil, store.ErrNotFound
	}
	return latest, nil
}

func (f *fakeStore) LatestRunsForJobs(ctx context.Context, jobIDs []string) (map[string]*model.JobRun, error) {
	result := make(map[string]*model.JobRun, len(jobIDs))
	for _, id := range jobIDs {
		run, err := f.LatestRunForJob(ctx, id)
		if err == store.ErrNotFound {
			continue
		}
		if err != nil {
			return nil, err
		}
		result[id] = run
	}
	return result, nil
}

func (f *fakeStore) HasActiveRun(ctx context.Context, jobID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.activeRunJob[jobID], nil
}

func (f *fakeStore) HasActiveRuns(ctx context.Context, jobIDs []string) (map[string]bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := make(map[string]bool, len(jobIDs))
	for _, id := range jobIDs {
		if f.activeRunJob[id] {
			result[id] = true
		}
	}
	return result, nil
}

func (f *fakeStore) CreateRun(ctx context.Context, jobID string, priority int16, scheduledAt time.Time) (*model.JobRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.nextRunID++
	run := &model.JobRun{
		ID:          "run-" + jobIDSuffix(f.nextRunID),
		JobID:       jobID,
		Status:      model.RunStatusPending,
		Attempt:     1,
		Priority:    priority,
		ScheduledAt: scheduledAt,
		CreatedAt:   time.Now(),
	}
	f.runs = append(f.runs, run)
	f.activeRunJob[jobID] = true
	return run, nil
}

// setRunStatus is a test helper to move a run (and its job's "active" bookkeeping)
// into a terminal or non-terminal state, mirroring what CompleteRun/FailRun/MarkDead
// would do in the real store.
func (f *fakeStore) setRunStatus(runID string, status model.RunStatus) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, run := range f.runs {
		if run.ID == runID {
			run.Status = status
			switch status {
			case model.RunStatusSucceeded, model.RunStatusFailed, model.RunStatusDead:
				f.activeRunJob[run.JobID] = false
			default:
				f.activeRunJob[run.JobID] = true
			}
			return
		}
	}
}

func jobIDSuffix(n int) string {
	digits := []byte{}
	if n == 0 {
		return "0"
	}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// --- Runs: worker lease lifecycle (unused by scheduler; minimal stubs) ---

func (f *fakeStore) LeaseNextRun(ctx context.Context, workerID string, leaseDuration time.Duration) (*model.JobRun, *model.Job, error) {
	return nil, nil, nil
}

func (f *fakeStore) ExtendLease(ctx context.Context, runID string, workerID string, extend time.Duration) error {
	return nil
}

func (f *fakeStore) MarkRunning(ctx context.Context, runID string) error {
	return nil
}

func (f *fakeStore) CompleteRun(ctx context.Context, runID string, result map[string]any) error {
	f.setRunStatus(runID, model.RunStatusSucceeded)
	return nil
}

func (f *fakeStore) FailRun(ctx context.Context, runID string, errMsg string, requeue bool, backoff time.Duration) error {
	if requeue {
		f.setRunStatus(runID, model.RunStatusPending)
	} else {
		f.setRunStatus(runID, model.RunStatusFailed)
	}
	return nil
}

func (f *fakeStore) MarkDead(ctx context.Context, runID string, reason string) error {
	f.setRunStatus(runID, model.RunStatusDead)
	return nil
}

func (f *fakeStore) ReclaimExpiredLeases(ctx context.Context) (int, error) {
	return 0, nil
}

func (f *fakeStore) GetRun(ctx context.Context, id string) (*model.JobRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, run := range f.runs {
		if run.ID == id {
			return run, nil
		}
	}
	return nil, store.ErrNotFound
}

func (f *fakeStore) ListJobRuns(ctx context.Context, jobID string, limit int) ([]*model.JobRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*model.JobRun
	for _, run := range f.runs {
		if run.JobID == jobID {
			out = append(out, run)
		}
	}
	return out, nil
}

func (f *fakeStore) CountPendingRuns(ctx context.Context) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, run := range f.runs {
		if run.Status == model.RunStatusPending {
			n++
		}
	}
	return n, nil
}

// --- Workers (unused by scheduler; minimal stubs) ---

func (f *fakeStore) UpsertWorkerHeartbeat(ctx context.Context, workerID, hostname string) error {
	return nil
}

func (f *fakeStore) ListWorkers(ctx context.Context) ([]*model.Worker, error) {
	return nil, nil
}

func (f *fakeStore) Close() {}

var _ store.Store = (*fakeStore)(nil)
