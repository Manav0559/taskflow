// Package scheduler determines which jobs are due to run and promotes them into
// job_runs. Only the elected leader replica should drive promotion (see Promoter),
// but the pure scheduling logic in this file (NextRunDue, DependenciesSatisfied) has
// no leader dependency and can be called/tested independently.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/manavsingla/taskflow/internal/model"
	"github.com/manavsingla/taskflow/internal/store"
)

// NextRunDue reports whether a job is currently due to have a new run created for it.
//
// For one-shot jobs (cronExpr == nil), "due" simply means it has never had a run
// created (lastScheduledAt == nil); one-shots never re-fire.
//
// For recurring jobs, the cron schedule is anchored to lastScheduledAt if the job has
// run before, or to createdAt if it hasn't (so a freshly created recurring job doesn't
// need to wait a full period before its first run is computed from schedule creation
// time rather than some arbitrary epoch).
func NextRunDue(cronExpr *string, lastScheduledAt *time.Time, createdAt time.Time, now time.Time) (bool, error) {
	if cronExpr == nil {
		return lastScheduledAt == nil, nil
	}

	schedule, err := cron.ParseStandard(*cronExpr)
	if err != nil {
		return false, fmt.Errorf("scheduler: parse cron expression %q: %w", *cronExpr, err)
	}

	base := createdAt
	if lastScheduledAt != nil {
		base = *lastScheduledAt
	}

	next := schedule.Next(base)
	return !next.After(now), nil
}

// DependenciesSatisfied reports whether every dependency of jobID has most recently
// succeeded. A dependency that has never run, or whose latest run did not succeed,
// blocks promotion of jobID.
func DependenciesSatisfied(ctx context.Context, st store.Store, jobID string) (bool, error) {
	deps, err := st.ListDependencies(ctx, jobID)
	if err != nil {
		return false, fmt.Errorf("scheduler: list dependencies for job %s: %w", jobID, err)
	}
	if len(deps) == 0 {
		return true, nil
	}

	for _, depID := range deps {
		run, err := st.LatestRunForJob(ctx, depID)
		if errors.Is(err, store.ErrNotFound) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("scheduler: latest run for dependency job %s: %w", depID, err)
		}
		if run.Status != model.RunStatusSucceeded {
			return false, nil
		}
	}

	return true, nil
}
