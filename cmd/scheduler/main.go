// Command scheduler runs the leader-elected promoter loop: it finds jobs that are due
// (cron schedule elapsed, or a one-shot job that has never run) and whose dependencies
// are satisfied, and creates a job_run for them. Multiple replicas may run for
// availability, but only the elected leader promotes at any moment (see internal/lock).
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/manavsingla/taskflow/internal/config"
	"github.com/manavsingla/taskflow/internal/lock"
	"github.com/manavsingla/taskflow/internal/logger"
	"github.com/manavsingla/taskflow/internal/metrics"
	"github.com/manavsingla/taskflow/internal/scheduler"
	"github.com/manavsingla/taskflow/internal/store"
)

// promotionLockKey is an arbitrary fixed key identifying the "promoter leader" slot in
// pg_try_advisory_lock's global keyspace. It only needs to be distinct from any other
// advisory lock this system might introduce later.
const promotionLockKey = 727433001

func main() {
	log := logger.New("scheduler")

	cfg, err := config.Load()
	if err != nil {
		log.Error("load config", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	st, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("connect to database", "error", err)
		os.Exit(1)
	}
	defer st.Close()

	if err := store.RunMigrations(ctx, st.Pool(), "migrations"); err != nil {
		log.Error("run migrations", "error", err)
		os.Exit(1)
	}

	elector := lock.NewPostgresElector(st.Pool(), promotionLockKey)
	promoter := scheduler.NewPromoter(st, elector, log, cfg.PollInterval)

	metricsServer := &http.Server{Addr: cfg.MetricsAddr, Handler: metrics.Handler()}
	go func() {
		log.Info("metrics server listening", "addr", cfg.MetricsAddr)
		_ = metricsServer.ListenAndServe()
	}()

	log.Info("scheduler starting", "interval", cfg.PollInterval)
	runErr := promoter.Run(ctx)
	log.Info("scheduler stopped", "reason", runErr)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = metricsServer.Shutdown(shutdownCtx)
}
