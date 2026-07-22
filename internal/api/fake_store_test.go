package api

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/manavsingla/taskflow/internal/model"
	"github.com/manavsingla/taskflow/internal/store"
)

// fakeStore is a minimal in-memory store.Store used only to exercise the api package's
// routing, validation, and auth/rate-limit middleware without a real Postgres. Methods
// the api package never calls (leasing, run lifecycle writes) are trivial stubs.
type fakeStore struct {
	mu   sync.Mutex
	jobs map[string]*model.Job
	runs map[string][]*model.JobRun
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		jobs: make(map[string]*model.Job),
		runs: make(map[string][]*model.JobRun),
	}
}

func (s *fakeStore) CreateJob(ctx context.Context, in model.NewJobInput) (*model.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if in.IdempotencyKey != nil && *in.IdempotencyKey != "" {
		for _, j := range s.jobs {
			if j.IdempotencyKey != nil && *j.IdempotencyKey == *in.IdempotencyKey {
				return nil, store.ErrIdempotencyConflict
			}
		}
	}

	now := time.Now()
	job := &model.Job{
		ID:             uuid.NewString(),
		Name:           in.Name,
		Payload:        in.Payload,
		CronExpr:       in.CronExpr,
		Priority:       in.Priority,
		MaxAttempts:    in.MaxAttempts,
		TimeoutSeconds: in.TimeoutSeconds,
		Status:         model.JobStatusActive,
		IdempotencyKey: in.IdempotencyKey,
		DependsOn:      in.DependsOn,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	s.jobs[job.ID] = job
	return job, nil
}

func (s *fakeStore) GetJob(ctx context.Context, id string) (*model.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j, ok := s.jobs[id]; ok {
		return j, nil
	}
	return nil, store.ErrNotFound
}

func (s *fakeStore) GetJobByIdempotencyKey(ctx context.Context, key string) (*model.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, j := range s.jobs {
		if j.IdempotencyKey != nil && *j.IdempotencyKey == key {
			return j, nil
		}
	}
	return nil, store.ErrNotFound
}

func (s *fakeStore) ListJobs(ctx context.Context, status *model.JobStatus, limit, offset int) ([]*model.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*model.Job
	for _, j := range s.jobs {
		if status == nil || j.Status == *status {
			out = append(out, j)
		}
	}
	return out, nil
}

func (s *fakeStore) UpdateJobStatus(ctx context.Context, id string, status model.JobStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	if !ok {
		return store.ErrNotFound
	}
	j.Status = status
	j.UpdatedAt = time.Now()
	return nil
}

func (s *fakeStore) ListDependencies(ctx context.Context, jobID string) ([]string, error) {
	return nil, nil
}

func (s *fakeStore) LatestRunForJob(ctx context.Context, jobID string) (*model.JobRun, error) {
	return nil, store.ErrNotFound
}

func (s *fakeStore) LatestRunsForJobs(ctx context.Context, jobIDs []string) (map[string]*model.JobRun, error) {
	return map[string]*model.JobRun{}, nil
}

func (s *fakeStore) HasActiveRun(ctx context.Context, jobID string) (bool, error) {
	return false, nil
}

func (s *fakeStore) HasActiveRuns(ctx context.Context, jobIDs []string) (map[string]bool, error) {
	return map[string]bool{}, nil
}

func (s *fakeStore) CreateRun(ctx context.Context, jobID string, priority int16, scheduledAt time.Time) (*model.JobRun, error) {
	return nil, nil
}

func (s *fakeStore) LeaseNextRun(ctx context.Context, workerID string, leaseDuration time.Duration) (*model.JobRun, *model.Job, error) {
	return nil, nil, nil
}

func (s *fakeStore) ExtendLease(ctx context.Context, runID, workerID string, extend time.Duration) error {
	return nil
}

func (s *fakeStore) MarkRunning(ctx context.Context, runID string) error { return nil }

func (s *fakeStore) CompleteRun(ctx context.Context, runID string, result map[string]any) error {
	return nil
}

func (s *fakeStore) FailRun(ctx context.Context, runID string, errMsg string, requeue bool, backoff time.Duration) error {
	return nil
}

func (s *fakeStore) MarkDead(ctx context.Context, runID string, reason string) error { return nil }

func (s *fakeStore) ReclaimExpiredLeases(ctx context.Context) (int, error) { return 0, nil }

func (s *fakeStore) GetRun(ctx context.Context, id string) (*model.JobRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, runs := range s.runs {
		for _, r := range runs {
			if r.ID == id {
				return r, nil
			}
		}
	}
	return nil, store.ErrNotFound
}

func (s *fakeStore) ListJobRuns(ctx context.Context, jobID string, limit int) ([]*model.JobRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runs[jobID], nil
}

func (s *fakeStore) CountPendingRuns(ctx context.Context) (int, error) { return 0, nil }

func (s *fakeStore) UpsertWorkerHeartbeat(ctx context.Context, workerID, hostname string) error {
	return nil
}

func (s *fakeStore) ListWorkers(ctx context.Context) ([]*model.Worker, error) {
	return nil, nil
}

func (s *fakeStore) Close() {}
