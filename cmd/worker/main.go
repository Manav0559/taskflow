// Command worker leases pending job runs from Postgres and executes them via a small
// built-in handler registry (echo/sleep/http_call). A real deployment would register
// domain-specific handlers here instead of (or in addition to) the examples.
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/manavsingla/taskflow/internal/config"
	"github.com/manavsingla/taskflow/internal/logger"
	"github.com/manavsingla/taskflow/internal/metrics"
	"github.com/manavsingla/taskflow/internal/store"
	"github.com/manavsingla/taskflow/internal/tracing"
	"github.com/manavsingla/taskflow/internal/worker"
)

func main() {
	log := logger.New("worker")

	cfg, err := config.Load()
	if err != nil {
		log.Error("load config", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTracing, err := tracing.Init(ctx, "worker", cfg.OTLPEndpoint)
	if err != nil {
		log.Error("init tracing", "error", err)
		os.Exit(1)
	}
	defer shutdownTracing(context.Background())

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

	pool := worker.NewPool(st, cfg.WorkerID, cfg.Concurrency, cfg.LeaseDuration, cfg.PollInterval, log)
	pool.RegisterHandler("echo", worker.EchoHandler)
	pool.RegisterHandler("sleep", worker.SleepHandler)
	pool.RegisterHandler("http_call", worker.HTTPCallHandler)

	metricsServer := &http.Server{Addr: cfg.MetricsAddr, Handler: metrics.Handler()}
	go func() {
		log.Info("metrics server listening", "addr", cfg.MetricsAddr)
		_ = metricsServer.ListenAndServe()
	}()

	log.Info("worker starting", "worker_id", cfg.WorkerID, "concurrency", cfg.Concurrency)
	runErr := pool.Run(ctx)
	log.Info("worker stopped", "reason", runErr)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = metricsServer.Shutdown(shutdownCtx)
}
