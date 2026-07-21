package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/manavsingla/taskflow/internal/lock"
	"github.com/manavsingla/taskflow/internal/metrics"
	"github.com/manavsingla/taskflow/internal/model"
	"github.com/manavsingla/taskflow/internal/store"
)

// listActiveJobsLimit bounds the single ListJobs page the promoter scans per pass.
// A real deployment with more active jobs than this would silently stop promoting
// the overflow; the correct fix is paginating through ListJobs here, but that's out
// of scope for this MVP.
const listActiveJobsLimit = 10000

// Promoter is the leader-only loop that turns due, dependency-satisfied jobs into
// pending job_runs. Only one replica's Promoter should be actively promoting at a
// time; that's enforced by Elector, not by this type itself.
type Promoter struct {
	Store    store.Store
	Elector  lock.Elector
	Logger   *slog.Logger
	Interval time.Duration
}

// NewPromoter constructs a Promoter ready to Run.
func NewPromoter(st store.Store, el lock.Elector, log *slog.Logger, interval time.Duration) *Promoter {
	return &Promoter{
		Store:    st,
		Elector:  el,
		Logger:   log,
		Interval: interval,
	}
}

// PromoteOnce scans active jobs and creates a run for each one that is due and whose
// dependencies are satisfied. It is safe to call directly (e.g. from tests or an
// admin "promote now" endpoint) without going through Run/Elector.
func (p *Promoter) PromoteOnce(ctx context.Context) (int, error) {
	activeStatus := model.JobStatusActive
	jobs, err := p.Store.ListJobs(ctx, &activeStatus, listActiveJobsLimit, 0)
	if err != nil {
		return 0, err
	}

	promoted := 0
	now := time.Now()

	for _, job := range jobs {
		hasActive, err := p.Store.HasActiveRun(ctx, job.ID)
		if err != nil {
			p.Logger.Error("check active run failed", "job_id", job.ID, "error", err)
			continue
		}
		if hasActive {
			continue
		}

		var lastScheduledAt *time.Time
		lastRun, err := p.Store.LatestRunForJob(ctx, job.ID)
		switch {
		case errors.Is(err, store.ErrNotFound):
			lastScheduledAt = nil
		case err != nil:
			p.Logger.Error("latest run lookup failed", "job_id", job.ID, "error", err)
			continue
		default:
			lastScheduledAt = &lastRun.ScheduledAt
		}

		due, err := NextRunDue(job.CronExpr, lastScheduledAt, job.CreatedAt, now)
		if err != nil {
			p.Logger.Error("next run due check failed", "job_id", job.ID, "error", err)
			continue
		}
		if !due {
			continue
		}

		satisfied, err := DependenciesSatisfied(ctx, p.Store, job.ID)
		if err != nil {
			p.Logger.Error("dependency check failed", "job_id", job.ID, "error", err)
			continue
		}
		if !satisfied {
			continue
		}

		if _, err := p.Store.CreateRun(ctx, job.ID, job.Priority, now); err != nil {
			p.Logger.Error("create run failed", "job_id", job.ID, "error", err)
			continue
		}

		promoted++
		p.Logger.Info("promoted job", "job_id", job.ID, "name", job.Name)
	}

	if depth, err := p.Store.CountPendingRuns(ctx); err != nil {
		p.Logger.Warn("count pending runs failed", "error", err)
	} else {
		metrics.QueueDepth.Set(float64(depth))
	}

	return promoted, nil
}

// Run drives the promotion loop until ctx is cancelled. Leadership is re-checked on
// every tick (not just once at startup) because leadership can change hands at any
// time - e.g. this replica's connection holding the advisory lock drops, or another
// replica's TryAcquire races in - so staying leader-aware only at Run() startup would
// let a demoted replica keep promoting.
func (p *Promoter) Run(ctx context.Context) error {
	ticker := time.NewTicker(p.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			if err := p.Elector.Release(ctx); err != nil {
				p.Logger.Warn("release leadership failed", "error", err)
			}
			return ctx.Err()

		case <-ticker.C:
			isLeader, err := p.Elector.TryAcquire(ctx)
			if err != nil {
				p.Logger.Error("leader election attempt failed", "error", err)
				metrics.IsLeader.Set(0)
				continue
			}

			if isLeader {
				metrics.IsLeader.Set(1)
				if _, err := p.PromoteOnce(ctx); err != nil {
					p.Logger.Error("promote once failed", "error", err)
				}
			} else {
				metrics.IsLeader.Set(0)
			}
		}
	}
}
