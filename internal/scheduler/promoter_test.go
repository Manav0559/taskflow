package scheduler

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/manavsingla/taskflow/internal/model"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newPromoter(fs *fakeStore) *Promoter {
	// PromoteOnce doesn't touch p.Elector, so a nil Elector is fine for these tests
	// (only Promoter.Run's leader-election path needs a real lock.Elector).
	return &Promoter{
		Store:  fs,
		Logger: testLogger(),
	}
}

func TestPromoteOnce_NoDepsNoPriorRun(t *testing.T) {
	fs := newFakeStore()
	fs.addJob(&model.Job{
		ID:        "j1",
		Name:      "job-one",
		Status:    model.JobStatusActive,
		CreatedAt: time.Now().Add(-time.Hour),
	})

	p := newPromoter(fs)
	promoted, err := p.PromoteOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if promoted != 1 {
		t.Fatalf("expected 1 job promoted, got %d", promoted)
	}

	runs, _ := fs.ListJobRuns(context.Background(), "j1", 10)
	if len(runs) != 1 {
		t.Fatalf("expected 1 run created for j1, got %d", len(runs))
	}
}

func TestPromoteOnce_ActiveRunNotDoublePromoted(t *testing.T) {
	fs := newFakeStore()
	fs.addJob(&model.Job{
		ID:        "j1",
		Name:      "job-one",
		Status:    model.JobStatusActive,
		CreatedAt: time.Now().Add(-time.Hour),
	})

	p := newPromoter(fs)

	// First pass promotes it and leaves an active (pending) run outstanding.
	promoted, err := p.PromoteOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if promoted != 1 {
		t.Fatalf("expected 1 job promoted on first pass, got %d", promoted)
	}

	// Second pass: the run from the first pass is still active (pending), so this
	// one-shot job must not be scheduled again.
	promoted, err = p.PromoteOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if promoted != 0 {
		t.Fatalf("expected 0 jobs promoted on second pass (active run outstanding), got %d", promoted)
	}

	runs, _ := fs.ListJobRuns(context.Background(), "j1", 10)
	if len(runs) != 1 {
		t.Fatalf("expected still only 1 run for j1, got %d", len(runs))
	}
}

func TestPromoteOnce_DependencyNotSatisfied(t *testing.T) {
	fs := newFakeStore()
	fs.addJob(&model.Job{
		ID:        "dep",
		Name:      "dependency",
		Status:    model.JobStatusActive,
		CreatedAt: time.Now().Add(-time.Hour),
	})
	fs.addJob(&model.Job{
		ID:        "j1",
		Name:      "dependent",
		Status:    model.JobStatusActive,
		CreatedAt: time.Now().Add(-time.Hour),
	}, "dep")

	p := newPromoter(fs)
	promoted, err := p.PromoteOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// "dep" has no dependencies and no prior run, so it should promote. "j1" depends
	// on "dep", which has never run (let alone succeeded), so it must not promote.
	if promoted != 1 {
		t.Fatalf("expected exactly 1 job (the dependency) promoted, got %d", promoted)
	}

	runs, _ := fs.ListJobRuns(context.Background(), "j1", 10)
	if len(runs) != 0 {
		t.Fatalf("expected dependent job j1 to have no runs, got %d", len(runs))
	}

	satisfied, err := DependenciesSatisfied(context.Background(), fs, "j1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if satisfied {
		t.Fatal("expected DependenciesSatisfied to report false when dependency has never run")
	}
}

func TestPromoteOnce_DependencySucceededThenPromotes(t *testing.T) {
	fs := newFakeStore()
	fs.addJob(&model.Job{
		ID:        "dep",
		Name:      "dependency",
		Status:    model.JobStatusActive,
		CreatedAt: time.Now().Add(-time.Hour),
	})
	fs.addJob(&model.Job{
		ID:        "j1",
		Name:      "dependent",
		Status:    model.JobStatusActive,
		CreatedAt: time.Now().Add(-time.Hour),
	}, "dep")

	p := newPromoter(fs)

	// First pass promotes "dep" (no deps of its own); "j1" stays blocked.
	if _, err := p.PromoteOnce(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	depRuns, _ := fs.ListJobRuns(context.Background(), "dep", 10)
	if len(depRuns) != 1 {
		t.Fatalf("expected 1 run for dep, got %d", len(depRuns))
	}

	// Simulate the worker completing dep's run successfully.
	fs.setRunStatus(depRuns[0].ID, model.RunStatusSucceeded)

	satisfied, err := DependenciesSatisfied(context.Background(), fs, "j1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !satisfied {
		t.Fatal("expected DependenciesSatisfied to report true once dependency succeeded")
	}

	// Second pass: dep is no longer due (one-shot, already ran) so it won't promote
	// again; j1 should now promote since its dependency succeeded.
	promoted, err := p.PromoteOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if promoted != 1 {
		t.Fatalf("expected exactly 1 job (j1) promoted on second pass, got %d", promoted)
	}

	j1Runs, _ := fs.ListJobRuns(context.Background(), "j1", 10)
	if len(j1Runs) != 1 {
		t.Fatalf("expected 1 run created for j1, got %d", len(j1Runs))
	}
}
