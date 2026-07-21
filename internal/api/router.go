// Package api implements taskflow's HTTP surface: job/run/worker CRUD-ish endpoints
// behind JWT auth and a per-IP rate limiter, plus unauthenticated /healthz and /metrics.
package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/manavsingla/taskflow/internal/metrics"
	"github.com/manavsingla/taskflow/internal/store"
)

// NewRouter wires the full HTTP API. jwtSecret is the shared HS256 secret used to
// verify Authorization: Bearer tokens on /v1/*; rateRPS/rateBurst configure the
// per-client-IP token bucket applied to the same routes.
func NewRouter(st store.Store, log *slog.Logger, jwtSecret string, rateRPS float64, rateBurst int) http.Handler {
	r := chi.NewRouter()

	r.Get("/healthz", healthzHandler)
	r.Handle("/metrics", metrics.Handler())

	h := &handler{store: st, log: log}
	limiter := newRateLimiter(rateRPS, rateBurst)

	r.Route("/v1", func(r chi.Router) {
		r.Use(chimiddleware.Recoverer)
		r.Use(requestLogger(log))
		r.Use(limiter.middleware)
		r.Use(jwtAuth(jwtSecret))

		r.Post("/jobs", h.createJob)
		r.Get("/jobs", h.listJobs)
		r.Get("/jobs/{id}", h.getJob)
		r.Post("/jobs/{id}/pause", h.pauseJob)
		r.Post("/jobs/{id}/resume", h.resumeJob)
		r.Get("/jobs/{id}/runs", h.listJobRuns)
		r.Get("/runs/{id}", h.getRun)
		r.Get("/workers", h.listWorkers)
	})

	return r
}

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
