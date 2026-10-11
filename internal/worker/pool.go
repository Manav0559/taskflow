// Package worker implements the lease/execute/complete loop that turns pending
// job_runs into finished (or retried, or dead-lettered) ones, plus the
// heartbeat and lease-reclaim background loops that keep the fleet healthy.
package worker

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/manavsingla/taskflow/internal/metrics"
	"github.com/manavsingla/taskflow/internal/model"
	"github.com/manavsingla/taskflow/internal/store"
)

var tracer = otel.Tracer("taskflow/worker")

const maxBackoff = 5 * time.Minute

type Pool struct {
	Store         store.Store
	WorkerID      string
	Concurrency   int
	LeaseDuration time.Duration
	PollInterval  time.Duration
	Logger        *slog.Logger

	handlers map[string]Handler
	mu       sync.RWMutex
}

func NewPool(st store.Store, workerID string, concurrency int, leaseDuration, pollInterval time.Duration, log *slog.Logger) *Pool {
	return &Pool{
		Store:         st,
		WorkerID:      workerID,
		Concurrency:   concurrency,
		LeaseDuration: leaseDuration,
		PollInterval:  pollInterval,
		Logger:        log,
		handlers:      make(map[string]Handler),
	}
}

// RegisterHandler binds h to jobName; executeOne looks handlers up by model.Job.Name.
func (p *Pool) RegisterHandler(jobName string, h Handler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handlers[jobName] = h
}

// Run blocks until ctx is cancelled, running Concurrency lease/execute loops
// plus one heartbeat loop and one janitor (lease-reclaim) loop.
func (p *Pool) Run(ctx context.Context) error {
	var wg sync.WaitGroup

	for i := 0; i < p.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.leaseLoop(ctx)
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		p.heartbeatLoop(ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		p.janitorLoop(ctx)
	}()

	<-ctx.Done()
	wg.Wait()
	return ctx.Err()
}

// leaseLoop is the body run by each of the Concurrency worker goroutines.
func (p *Pool) leaseLoop(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}

		run, job, err := p.Store.LeaseNextRun(ctx, p.WorkerID, p.LeaseDuration)
		if err != nil {
			p.Logger.Error("lease next run failed", "error", err)
			if !sleepCtx(ctx, p.PollInterval) {
				return
			}
			continue
		}
		if run == nil {
			if !sleepCtx(ctx, p.PollInterval) {
				return
			}
			continue
		}

		p.executeOne(ctx, run, job)
	}
}

// sleepCtx sleeps for d or until ctx is done, whichever comes first, reporting
// which happened so callers can stop promptly on shutdown instead of finishing
// out a poll-interval sleep first.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// leaseLost logs and counts a fenced write that was rejected because this worker no
// longer holds the run's lease. The run belongs to whoever holds the current lease
// (or is pending again), so the only correct action is to drop it: retrying would
// fail the same way and could only produce a retry storm.
func (p *Pool) leaseLost(span trace.Span, op string, run *model.JobRun, job *model.Job) {
	metrics.LeasesLost.Inc()
	span.SetStatus(codes.Error, store.ErrLeaseLost.Error())
	p.Logger.Warn("lease lost, dropping run",
		"op", op, "run_id", run.ID, "job_id", job.ID, "job_name", job.Name, "attempt", run.Attempt)
}

// recordErr handles the error from a fenced store write. It returns true if the
// lease was lost, in which case the caller must stop processing the run.
func (p *Pool) recordErr(span trace.Span, op string, err error, run *model.JobRun, job *model.Job) (lost bool) {
	if err == nil {
		return false
	}
	if errors.Is(err, store.ErrLeaseLost) {
		p.leaseLost(span, op, run, job)
		return true
	}
	p.Logger.Error(op+" failed", "run_id", run.ID, "error", err)
	return false
}

func (p *Pool) executeOne(ctx context.Context, run *model.JobRun, job *model.Job) {
	ctx, span := tracer.Start(ctx, "worker.executeOne", trace.WithAttributes(
		attribute.String("job.id", job.ID),
		attribute.String("job.name", job.Name),
		attribute.String("run.id", run.ID),
		attribute.Int("run.attempt", int(run.Attempt)),
	))
	defer span.End()

	lease := store.LeaseOf(run, p.WorkerID)

	p.mu.RLock()
	h, ok := p.handlers[job.Name]
	p.mu.RUnlock()

	if !ok {
		msg := "no handler registered for job: " + job.Name
		if p.recordErr(span, "fail run", p.Store.FailRun(ctx, lease, msg, false, 0), run, job) {
			return
		}
		if p.recordErr(span, "mark dead", p.Store.MarkDead(ctx, lease, "no handler"), run, job) {
			return
		}
		metrics.RunsCompleted.WithLabelValues("dead").Inc()
		span.SetStatus(codes.Error, msg)
		p.Logger.Error("job run dead: no handler registered",
			"run_id", run.ID, "job_id", job.ID, "job_name", job.Name)
		return
	}

	if p.recordErr(span, "mark running", p.Store.MarkRunning(ctx, lease), run, job) {
		return
	}
	metrics.RunsLeased.WithLabelValues(p.WorkerID).Inc()

	runCtx, cancel := context.WithTimeout(ctx, time.Duration(job.TimeoutSeconds)*time.Second)
	defer cancel()

	start := time.Now()
	result, err := h(runCtx, job, run)
	metrics.RunDuration.Observe(time.Since(start).Seconds())

	// Defensive: a handler that ignores ctx and returns nil error on a timed-out
	// run should still be treated as a failure, not a success.
	if err == nil && runCtx.Err() != nil {
		err = runCtx.Err()
	}

	if err == nil {
		if p.recordErr(span, "complete run", p.Store.CompleteRun(ctx, lease, result), run, job) {
			return
		}
		metrics.RunsCompleted.WithLabelValues("succeeded").Inc()
		span.SetStatus(codes.Ok, "")
		p.Logger.Info("job run succeeded", "run_id", run.ID, "job_id", job.ID, "job_name", job.Name)
		return
	}

	span.RecordError(err)

	// run.Attempt already reflects this attempt (LeaseNextRun increments it),
	// so comparing it directly to MaxAttempts tells us if another try remains.
	if run.Attempt < job.MaxAttempts {
		backoff := time.Duration(1<<uint(run.Attempt)) * time.Second
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
		if p.recordErr(span, "fail run", p.Store.FailRun(ctx, lease, err.Error(), true, backoff), run, job) {
			return
		}
		metrics.RunsCompleted.WithLabelValues("failed").Inc()
		p.Logger.Warn("job run failed, will retry",
			"run_id", run.ID, "job_id", job.ID, "job_name", job.Name,
			"attempt", run.Attempt, "max_attempts", job.MaxAttempts,
			"backoff", backoff, "error", err)
		return
	}

	if p.recordErr(span, "fail run", p.Store.FailRun(ctx, lease, err.Error(), false, 0), run, job) {
		return
	}
	if p.recordErr(span, "mark dead", p.Store.MarkDead(ctx, lease, "max attempts exceeded"), run, job) {
		return
	}
	metrics.RunsCompleted.WithLabelValues("dead").Inc()
	span.SetStatus(codes.Error, err.Error())
	p.Logger.Error("job run dead: max attempts exceeded",
		"run_id", run.ID, "job_id", job.ID, "job_name", job.Name,
		"attempt", run.Attempt, "max_attempts", job.MaxAttempts, "error", err)
}
