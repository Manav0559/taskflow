// Package metrics defines the Prometheus collectors shared across services. Collectors
// are package-level singletons registered against the default registry exactly once
// here, so api/worker/scheduler can all import and increment them without risking a
// "duplicate metrics collector registration" panic from registering twice.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	JobsSubmitted = promauto.NewCounter(prometheus.CounterOpts{
		Name: "taskflow_jobs_submitted_total",
		Help: "Total number of jobs submitted via the API.",
	})

	RunsLeased = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "taskflow_runs_leased_total",
		Help: "Total number of job runs leased by a worker.",
	}, []string{"worker_id"})

	RunsCompleted = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "taskflow_runs_completed_total",
		Help: "Total number of job runs completed, by outcome.",
	}, []string{"outcome"}) // succeeded|failed|dead

	RunDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "taskflow_run_duration_seconds",
		Help:    "Duration of job run execution.",
		Buckets: prometheus.DefBuckets,
	})

	QueueDepth = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "taskflow_queue_depth",
		Help: "Number of runs currently in pending status.",
	})

	LeasesReclaimed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "taskflow_leases_reclaimed_total",
		Help: "Total number of runs reclaimed from a worker whose lease expired.",
	})

	HTTPRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "taskflow_http_request_duration_seconds",
		Help:    "HTTP request duration by route and status code.",
		Buckets: prometheus.DefBuckets,
	}, []string{"route", "method", "status"})

	IsLeader = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "taskflow_scheduler_is_leader",
		Help: "1 if this scheduler replica currently holds the promotion leader lock.",
	})
)

// Handler returns the /metrics HTTP handler for the default Prometheus registry.
func Handler() http.Handler {
	return promhttp.Handler()
}
