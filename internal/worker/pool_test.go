package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/manavsingla/taskflow/internal/model"
	"github.com/manavsingla/taskflow/internal/store"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestPool(fs *fakeStore) *Pool {
	p := NewPool(fs, "test-worker", 1, 0, 0, testLogger())
	return p
}

func TestExecuteOne_Success(t *testing.T) {
	fs := newFakeStore()
	p := newTestPool(fs)
	p.RegisterHandler("ok-job", func(ctx context.Context, job *model.Job, run *model.JobRun) (map[string]any, error) {
		return map[string]any{"done": true}, nil
	})

	job := &model.Job{ID: "job-1", Name: "ok-job", MaxAttempts: 3, TimeoutSeconds: 5}
	run := &model.JobRun{ID: "run-1", JobID: "job-1", Attempt: 1}

	p.executeOne(context.Background(), run, job)

	if len(fs.completeRunCalls) != 1 {
		t.Fatalf("expected CompleteRun to be called once, got %d", len(fs.completeRunCalls))
	}
	if fs.completeRunCalls[0].lease.RunID != "run-1" {
		t.Errorf("CompleteRun called with runID %q, want %q", fs.completeRunCalls[0].lease.RunID, "run-1")
	}
	if len(fs.failRunCalls) != 0 {
		t.Errorf("expected FailRun not to be called, got %d calls", len(fs.failRunCalls))
	}
	if len(fs.markDeadCalls) != 0 {
		t.Errorf("expected MarkDead not to be called, got %d calls", len(fs.markDeadCalls))
	}
	if len(fs.markRunningCalls) != 1 {
		t.Errorf("expected MarkRunning to be called once, got %d", len(fs.markRunningCalls))
	}
}

func TestExecuteOne_FailureWithAttemptsRemaining(t *testing.T) {
	fs := newFakeStore()
	p := newTestPool(fs)
	handlerErr := errors.New("boom")
	p.RegisterHandler("bad-job", func(ctx context.Context, job *model.Job, run *model.JobRun) (map[string]any, error) {
		return nil, handlerErr
	})

	// Attempt 1 of 3 max attempts: a retry should be scheduled (requeue=true).
	job := &model.Job{ID: "job-1", Name: "bad-job", MaxAttempts: 3, TimeoutSeconds: 5}
	run := &model.JobRun{ID: "run-1", JobID: "job-1", Attempt: 1}

	p.executeOne(context.Background(), run, job)

	if len(fs.completeRunCalls) != 0 {
		t.Errorf("expected CompleteRun not to be called, got %d calls", len(fs.completeRunCalls))
	}
	if len(fs.failRunCalls) != 1 {
		t.Fatalf("expected FailRun to be called once, got %d", len(fs.failRunCalls))
	}
	fc := fs.failRunCalls[0]
	if fc.lease.RunID != "run-1" {
		t.Errorf("FailRun called with runID %q, want %q", fc.lease.RunID, "run-1")
	}
	if !fc.requeue {
		t.Error("expected FailRun to be called with requeue=true when attempts remain")
	}
	if len(fs.markDeadCalls) != 0 {
		t.Errorf("expected MarkDead not to be called when attempts remain, got %d calls", len(fs.markDeadCalls))
	}
}

func TestExecuteOne_FailureAttemptsExhausted(t *testing.T) {
	fs := newFakeStore()
	p := newTestPool(fs)
	handlerErr := errors.New("boom")
	p.RegisterHandler("bad-job", func(ctx context.Context, job *model.Job, run *model.JobRun) (map[string]any, error) {
		return nil, handlerErr
	})

	// Attempt 3 of 3 max attempts: no retries left, run should be failed (requeue=false)
	// and then marked dead.
	job := &model.Job{ID: "job-1", Name: "bad-job", MaxAttempts: 3, TimeoutSeconds: 5}
	run := &model.JobRun{ID: "run-1", JobID: "job-1", Attempt: 3}

	p.executeOne(context.Background(), run, job)

	if len(fs.completeRunCalls) != 0 {
		t.Errorf("expected CompleteRun not to be called, got %d calls", len(fs.completeRunCalls))
	}
	if len(fs.failRunCalls) != 1 {
		t.Fatalf("expected FailRun to be called once, got %d", len(fs.failRunCalls))
	}
	fc := fs.failRunCalls[0]
	if fc.requeue {
		t.Error("expected FailRun to be called with requeue=false when attempts exhausted")
	}
	if len(fs.markDeadCalls) != 1 {
		t.Fatalf("expected MarkDead to be called once, got %d", len(fs.markDeadCalls))
	}
	if fs.markDeadCalls[0].lease.RunID != "run-1" {
		t.Errorf("MarkDead called with runID %q, want %q", fs.markDeadCalls[0].lease.RunID, "run-1")
	}
}

func TestExecuteOne_NoHandlerRegistered(t *testing.T) {
	fs := newFakeStore()
	p := newTestPool(fs)

	job := &model.Job{ID: "job-1", Name: "unregistered-job", MaxAttempts: 3, TimeoutSeconds: 5}
	run := &model.JobRun{ID: "run-1", JobID: "job-1", Attempt: 1}

	p.executeOne(context.Background(), run, job)

	if len(fs.failRunCalls) != 1 {
		t.Fatalf("expected FailRun to be called once for missing handler, got %d", len(fs.failRunCalls))
	}
	if fs.failRunCalls[0].requeue {
		t.Error("expected FailRun requeue=false for missing handler")
	}
	if len(fs.markDeadCalls) != 1 {
		t.Fatalf("expected MarkDead to be called once for missing handler, got %d", len(fs.markDeadCalls))
	}
	if len(fs.markRunningCalls) != 0 {
		t.Errorf("expected MarkRunning not to be called for missing handler, got %d calls", len(fs.markRunningCalls))
	}
}

func TestExecuteOne_PassesLeaseAsFencingToken(t *testing.T) {
	fs := newFakeStore()
	p := newTestPool(fs)
	p.RegisterHandler("ok-job", func(ctx context.Context, job *model.Job, run *model.JobRun) (map[string]any, error) {
		return nil, nil
	})

	job := &model.Job{ID: "job-1", Name: "ok-job", MaxAttempts: 3, TimeoutSeconds: 5}
	run := &model.JobRun{ID: "run-1", JobID: "job-1", Attempt: 2}

	p.executeOne(context.Background(), run, job)

	want := store.Lease{RunID: "run-1", WorkerID: "test-worker", Attempt: 2}
	if len(fs.markRunningCalls) != 1 || fs.markRunningCalls[0] != want {
		t.Errorf("MarkRunning leases = %+v, want [%+v]", fs.markRunningCalls, want)
	}
	if len(fs.completeRunCalls) != 1 || fs.completeRunCalls[0].lease != want {
		t.Errorf("CompleteRun leases = %+v, want [%+v]", fs.completeRunCalls, want)
	}
}

func TestExecuteOne_MarkRunningLeaseLostSkipsHandler(t *testing.T) {
	fs := newFakeStore()
	fs.markRunningErr = store.ErrLeaseLost
	p := newTestPool(fs)
	called := false
	p.RegisterHandler("ok-job", func(ctx context.Context, job *model.Job, run *model.JobRun) (map[string]any, error) {
		called = true
		return nil, nil
	})

	job := &model.Job{ID: "job-1", Name: "ok-job", MaxAttempts: 3, TimeoutSeconds: 5}
	run := &model.JobRun{ID: "run-1", JobID: "job-1", Attempt: 1}

	p.executeOne(context.Background(), run, job)

	if called {
		t.Error("handler must not run once the lease is lost")
	}
	if n := len(fs.completeRunCalls) + len(fs.failRunCalls) + len(fs.markDeadCalls); n != 0 {
		t.Errorf("expected no further writes after lease lost, got %d", n)
	}
}

func TestExecuteOne_CompleteLeaseLostIsDroppedNotRetried(t *testing.T) {
	fs := newFakeStore()
	fs.completeRunErr = fmt.Errorf("wrapped: %w", store.ErrLeaseLost)
	p := newTestPool(fs)
	p.RegisterHandler("ok-job", func(ctx context.Context, job *model.Job, run *model.JobRun) (map[string]any, error) {
		return map[string]any{"done": true}, nil
	})

	job := &model.Job{ID: "job-1", Name: "ok-job", MaxAttempts: 3, TimeoutSeconds: 5}
	run := &model.JobRun{ID: "run-1", JobID: "job-1", Attempt: 1}

	p.executeOne(context.Background(), run, job)

	if len(fs.completeRunCalls) != 1 {
		t.Errorf("expected exactly one CompleteRun attempt, got %d", len(fs.completeRunCalls))
	}
	if len(fs.failRunCalls) != 0 || len(fs.markDeadCalls) != 0 {
		t.Errorf("lease lost on complete must not fall through to FailRun/MarkDead, got fail=%d dead=%d",
			len(fs.failRunCalls), len(fs.markDeadCalls))
	}
}

func TestExecuteOne_FailLeaseLostSkipsMarkDead(t *testing.T) {
	fs := newFakeStore()
	fs.failRunErr = store.ErrLeaseLost
	p := newTestPool(fs)
	p.RegisterHandler("bad-job", func(ctx context.Context, job *model.Job, run *model.JobRun) (map[string]any, error) {
		return nil, errors.New("boom")
	})

	job := &model.Job{ID: "job-1", Name: "bad-job", MaxAttempts: 1, TimeoutSeconds: 5}
	run := &model.JobRun{ID: "run-1", JobID: "job-1", Attempt: 1}

	p.executeOne(context.Background(), run, job)

	if len(fs.failRunCalls) != 1 {
		t.Fatalf("expected exactly one FailRun attempt, got %d", len(fs.failRunCalls))
	}
	if len(fs.markDeadCalls) != 0 {
		t.Errorf("expected MarkDead to be skipped after lease lost, got %d calls", len(fs.markDeadCalls))
	}
}
