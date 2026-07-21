package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/manavsingla/taskflow/internal/model"
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
	if fs.completeRunCalls[0].runID != "run-1" {
		t.Errorf("CompleteRun called with runID %q, want %q", fs.completeRunCalls[0].runID, "run-1")
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
	if fc.runID != "run-1" {
		t.Errorf("FailRun called with runID %q, want %q", fc.runID, "run-1")
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
	if fs.markDeadCalls[0].runID != "run-1" {
		t.Errorf("MarkDead called with runID %q, want %q", fs.markDeadCalls[0].runID, "run-1")
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
