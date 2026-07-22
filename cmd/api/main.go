// Command api serves taskflow's HTTP API (job/run submission and inspection) plus a
// dedicated Prometheus metrics endpoint on a second port, matching the worker and
// scheduler binaries so all three are scraped the same way in docker-compose/k8s.
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/manavsingla/taskflow/internal/api"
	"github.com/manavsingla/taskflow/internal/cache"
	"github.com/manavsingla/taskflow/internal/config"
	"github.com/manavsingla/taskflow/internal/logger"
	"github.com/manavsingla/taskflow/internal/metrics"
	"github.com/manavsingla/taskflow/internal/store"
	"github.com/manavsingla/taskflow/internal/tracing"
)

func main() {
	log := logger.New("api")

	cfg, err := config.Load()
	if err != nil {
		log.Error("load config", "error", err)
		os.Exit(1)
	}
	if cfg.JWTSecret == "" {
		log.Error("JWT_SECRET must be set (refusing to start with an empty admin signing secret)")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// fatal runs the deferred stop() before exiting (a bare os.Exit here would skip it),
	// since we're past the point where stop is registered.
	fatal := func(msg string, err error) {
		log.Error(msg, "error", err)
		stop()
		os.Exit(1)
	}

	shutdownTracing, err := tracing.Init(ctx, "api", cfg.OTLPEndpoint)
	if err != nil {
		fatal("init tracing", err)
	}
	defer func() { _ = shutdownTracing(context.Background()) }()

	pgStore, err := store.New(ctx, cfg.DatabaseURL, cfg.DBMaxConns, cfg.DBMinConns)
	if err != nil {
		fatal("connect to database", err)
	}

	if err := store.RunMigrations(ctx, pgStore.Pool(), "migrations"); err != nil {
		fatal("run migrations", err)
	}

	// Read-replica routing is opt-in: with REPLICA_DATABASE_URL unset, reads stay on
	// the primary pool, same as before this feature existed.
	if cfg.ReplicaURL != "" {
		if err := pgStore.EnableReadReplica(ctx, cfg.ReplicaURL, cfg.DBMaxConns, cfg.DBMinConns); err != nil {
			fatal("connect to read replica", err)
		}
		log.Info("read replica enabled for job/run reads")
	}

	// Caching is opt-in: with REDIS_ADDR unset, svc is just pgStore and every read
	// goes straight to Postgres, same as before this feature existed.
	var svc store.Store = pgStore
	if cfg.RedisAddr != "" {
		svc = cache.Wrap(pgStore, cfg.RedisAddr)
		log.Info("job/run read cache enabled", "redis_addr", cfg.RedisAddr)
	}
	defer svc.Close()

	router := api.NewRouter(svc, log, cfg.JWTSecret, cfg.RateLimitRPS, cfg.RateLimitBurst, cfg.CORSAllowedOrigins)
	tracedRouter := otelhttp.NewHandler(router, "taskflow-api")

	apiServer := &http.Server{Addr: cfg.HTTPAddr, Handler: tracedRouter}
	metricsServer := &http.Server{Addr: cfg.MetricsAddr, Handler: metrics.Handler()}

	errCh := make(chan error, 2)
	go func() {
		log.Info("api server listening", "addr", cfg.HTTPAddr)
		if err := apiServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	go func() {
		log.Info("metrics server listening", "addr", cfg.MetricsAddr)
		if err := metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-errCh:
		log.Error("server error, shutting down", "error", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = apiServer.Shutdown(shutdownCtx)
	_ = metricsServer.Shutdown(shutdownCtx)
}
