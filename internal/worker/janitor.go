package worker

import (
	"context"
	"os"
	"time"

	"github.com/manavsingla/taskflow/internal/metrics"
)

// heartbeatLoop reuses PollInterval so a worker's liveness signal refreshes at
// the same cadence it polls for work, rather than adding a second tunable.
func (p *Pool) heartbeatLoop(ctx context.Context) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	ticker := time.NewTicker(p.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.Store.UpsertWorkerHeartbeat(ctx, p.WorkerID, hostname); err != nil {
				p.Logger.Error("heartbeat failed", "error", err)
			}
		}
	}
}

// janitorLoop runs at 5x PollInterval: expired leases only need to be reclaimed
// after LeaseDuration has already elapsed, so polling as fast as work-polling
// would just add DB load without shortening reclaim latency meaningfully.
func (p *Pool) janitorLoop(ctx context.Context) {
	ticker := time.NewTicker(p.PollInterval * 5)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := p.Store.ReclaimExpiredLeases(ctx)
			if err != nil {
				p.Logger.Error("reclaim expired leases failed", "error", err)
				continue
			}
			if n > 0 {
				metrics.LeasesReclaimed.Add(float64(n))
				p.Logger.Warn("reclaimed expired leases", "count", n)
			}
		}
	}
}
