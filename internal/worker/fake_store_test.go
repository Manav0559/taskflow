package worker

import (
	"context"
	"sync"
	"time"

	"github.com/manavsingla/taskflow/internal/model"
	"github.com/manavsingla/taskflow/internal/store"
)

// fakeStore is a minimal in-memory store.Store implementation used to drive
// Pool.executeOne without a real Postgres. It records which lifecycle methods were
// called (and with what arguments) so tests can assert executeOne's retry/dead
// decisions without caring about persistence details.
type fakeStore struct {
	mu sync.Mutex

	markRunningCalls []string
	completeRunCalls []completeRunCall
	failRunCalls     []failRunCall
	markDeadCalls    []markDeadCall
}

type completeRunCall struct {
	runID  string
	result map[string]any
}

type failRunCall struct {
	runID   string
	errMsg  string
	requeue bool
	backoff time.Duration
}

type markDeadCall struct {
	runID  string
	reason string
}

func newFakeStore() *fakeStore {
	return &fakeStore{}
}

// --- Jobs (unused by pool tests; minimal stubs) ---

func (f *fakeStore) CreateJob(ctx context.Context, in model.NewJobInput) (*model.Job, error) {
	return nil, nil
}
func (f *fakeStore) GetJob(ctx context.Context, id string) (*model.Job, error) { return nil, nil }
func (f *fakeStore) GetJobByIdempotencyKey(ctx context.Context, key string) (*model.Job, error) {
	return nil, nil
}
func (f *fakeStore) ListJobs(ctx context.Context, status *model.JobStatus, limit, offset int) ([]*model.Job, error) {
	return nil, nil
}
func (f *fakeStore) UpdateJobStatus(ctx context.Context, id string, status model.JobStatus) error {
	return nil
}
func (f *fakeStore) ListDependencies(ctx context.Context, jobID string) ([]string, error) {
	return nil, nil
}

// --- Runs: scheduling / promotion (unused by pool tests; minimal stubs) ---

func (f *fakeStore) LatestRunForJob(ctx context.Context, jobID string) (*model.JobRun, error) {
	return nil, store.ErrNotFound
}
func (f *fakeStore) HasActiveRun(ctx context.Context, jobID string) (bool, error) {
	return false, nil
}
func (f *fakeStore) CreateRun(ctx context.Context, jobID string, priority int16, scheduledAt time.Time) (*model.JobRun, error) {
	return nil, nil
}

// --- Runs: worker lease lifecycle ---

func (f *fakeStore) LeaseNextRun(ctx context.Context, workerID string, leaseDuration time.Duration) (*model.JobRun, *model.Job, error) {
	return nil, nil, nil
}

func (f *fakeStore) ExtendLease(ctx context.Context, runID string, workerID string, extend time.Duration) error {
	return nil
}

func (f *fakeStore) MarkRunning(ctx context.Context, runID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.markRunningCalls = append(f.markRunningCalls, runID)
	return nil
}

func (f *fakeStore) CompleteRun(ctx context.Context, runID string, result map[string]any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.completeRunCalls = append(f.completeRunCalls, completeRunCall{runID: runID, result: result})
	return nil
}

func (f *fakeStore) FailRun(ctx context.Context, runID string, errMsg string, requeue bool, backoff time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failRunCalls = append(f.failRunCalls, failRunCall{runID: runID, errMsg: errMsg, requeue: requeue, backoff: backoff})
	return nil
}

func (f *fakeStore) MarkDead(ctx context.Context, runID string, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.markDeadCalls = append(f.markDeadCalls, markDeadCall{runID: runID, reason: reason})
	return nil
}

func (f *fakeStore) ReclaimExpiredLeases(ctx context.Context) (int, error) {
	return 0, nil
}

func (f *fakeStore) GetRun(ctx context.Context, id string) (*model.JobRun, error) {
	return nil, store.ErrNotFound
}

func (f *fakeStore) ListJobRuns(ctx context.Context, jobID string, limit int) ([]*model.JobRun, error) {
	return nil, nil
}

func (f *fakeStore) CountPendingRuns(ctx context.Context) (int, error) {
	return 0, nil
}

// --- Workers (unused by pool tests; minimal stubs) ---

func (f *fakeStore) UpsertWorkerHeartbeat(ctx context.Context, workerID, hostname string) error {
	return nil
}

func (f *fakeStore) ListWorkers(ctx context.Context) ([]*model.Worker, error) {
	return nil, nil
}

func (f *fakeStore) Close() {}

var _ store.Store = (*fakeStore)(nil)
