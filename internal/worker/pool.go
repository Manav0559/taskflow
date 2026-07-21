// Package worker implements the lease/execute/complete loop that turns pending
// job_runs into finished (or retried, or dead-lettered) ones, plus the
// heartbeat and lease-reclaim background loops that keep the fleet healthy.
package worker

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/manavsingla/taskflow/internal/metrics"
	"github.com/manavsingla/taskflow/internal/model"
	"github.com/manavsingla/taskflow/internal/store"
)

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

func (p *Pool) executeOne(ctx context.Context, run *model.JobRun, job *model.Job) {
	p.mu.RLock()
	h, ok := p.handlers[job.Name]
	p.mu.RUnlock()

	if !ok {
		msg := "no handler registered for job: " + job.Name
		if err := p.Store.FailRun(ctx, run.ID, msg, false, 0); err != nil {
			p.Logger.Error("fail run failed", "run_id", run.ID, "error", err)
		}
		if err := p.Store.MarkDead(ctx, run.ID, "no handler"); err != nil {
			p.Logger.Error("mark dead failed", "run_id", run.ID, "error", err)
		}
		metrics.RunsCompleted.WithLabelValues("dead").Inc()
		p.Logger.Error("job run dead: no handler registered",
			"run_id", run.ID, "job_id", job.ID, "job_name", job.Name)
		return
	}

	if err := p.Store.MarkRunning(ctx, run.ID); err != nil {
		p.Logger.Error("mark running failed", "run_id", run.ID, "error", err)
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
		if cerr := p.Store.CompleteRun(ctx, run.ID, result); cerr != nil {
			p.Logger.Error("complete run failed", "run_id", run.ID, "error", cerr)
		}
		metrics.RunsCompleted.WithLabelValues("succeeded").Inc()
		p.Logger.Info("job run succeeded", "run_id", run.ID, "job_id", job.ID, "job_name", job.Name)
		return
	}

	// run.Attempt already reflects this attempt (LeaseNextRun increments it),
	// so comparing it directly to MaxAttempts tells us if another try remains.
	if run.Attempt < job.MaxAttempts {
		backoff := time.Duration(1<<uint(run.Attempt)) * time.Second
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
		if ferr := p.Store.FailRun(ctx, run.ID, err.Error(), true, backoff); ferr != nil {
			p.Logger.Error("fail run failed", "run_id", run.ID, "error", ferr)
		}
		metrics.RunsCompleted.WithLabelValues("failed").Inc()
		p.Logger.Warn("job run failed, will retry",
			"run_id", run.ID, "job_id", job.ID, "job_name", job.Name,
			"attempt", run.Attempt, "max_attempts", job.MaxAttempts,
			"backoff", backoff, "error", err)
		return
	}

	if ferr := p.Store.FailRun(ctx, run.ID, err.Error(), false, 0); ferr != nil {
		p.Logger.Error("fail run failed", "run_id", run.ID, "error", ferr)
	}
	if derr := p.Store.MarkDead(ctx, run.ID, "max attempts exceeded"); derr != nil {
		p.Logger.Error("mark dead failed", "run_id", run.ID, "error", derr)
	}
	metrics.RunsCompleted.WithLabelValues("dead").Inc()
	p.Logger.Error("job run dead: max attempts exceeded",
		"run_id", run.ID, "job_id", job.ID, "job_name", job.Name,
		"attempt", run.Attempt, "max_attempts", job.MaxAttempts, "error", err)
}
