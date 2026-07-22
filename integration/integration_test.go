// Package integration exercises the real Postgres-backed store, the HTTP API, the
// scheduler's promotion logic, and the worker pool together against a live database —
// unlike the unit tests in internal/scheduler and internal/worker, which use an
// in-memory fake store. It requires a reachable Postgres and is skipped otherwise
// (CI provides one via a service container; locally, `docker compose up -d postgres`
// plus DATABASE_URL=postgres://taskflow:taskflow@localhost:5432/taskflow?sslmode=disable).
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/manavsingla/taskflow/internal/api"
	"github.com/manavsingla/taskflow/internal/model"
	"github.com/manavsingla/taskflow/internal/scheduler"
	"github.com/manavsingla/taskflow/internal/store"
	"github.com/manavsingla/taskflow/internal/worker"
)

const testSecret = "integration-test-secret"

func TestJobLifecycleEndToEnd(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test (requires a real Postgres)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st, err := store.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect to database: %v", err)
	}
	defer st.Close()

	// Tests run with the package directory as the working directory, so migrations/
	// (at the repo root) is one level up.
	if err := store.RunMigrations(ctx, st.Pool(), "../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	router := api.NewRouter(st, log, testSecret, 1000, 1000, []string{"*"})
	srv := httptest.NewServer(router)
	defer srv.Close()

	token, err := api.MintToken(testSecret, "integration-test", time.Hour)
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}

	job := createJob(t, srv.URL, token, map[string]any{
		"name":            "echo",
		"payload":         map[string]any{"hello": "integration"},
		"max_attempts":    2,
		"timeout_seconds": 5,
	})

	promoter := &scheduler.Promoter{Store: st, Logger: log}
	promoted, err := promoter.PromoteOnce(ctx)
	if err != nil {
		t.Fatalf("PromoteOnce: %v", err)
	}
	if promoted != 1 {
		t.Fatalf("expected exactly 1 run promoted, got %d", promoted)
	}

	pool := worker.NewPool(st, "integration-worker", 1, 30*time.Second, 100*time.Millisecond, log)
	pool.RegisterHandler("echo", worker.EchoHandler)

	workerCtx, workerCancel := context.WithTimeout(ctx, 3*time.Second)
	done := make(chan error, 1)
	go func() { done <- pool.Run(workerCtx) }()

	status := waitForTerminalRun(t, ctx, st, job.ID, 3*time.Second)
	workerCancel()
	<-done

	if status != model.RunStatusSucceeded {
		t.Fatalf("expected run to succeed, got status %q", status)
	}
}

func TestDeadLetterOnUnregisteredHandler(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test (requires a real Postgres)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st, err := store.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect to database: %v", err)
	}
	defer st.Close()

	if err := store.RunMigrations(ctx, st.Pool(), "../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	router := api.NewRouter(st, log, testSecret, 1000, 1000, []string{"*"})
	srv := httptest.NewServer(router)
	defer srv.Close()

	token, err := api.MintToken(testSecret, "integration-test", time.Hour)
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}

	job := createJob(t, srv.URL, token, map[string]any{
		"name":            "no_such_handler",
		"max_attempts":    2,
		"timeout_seconds": 5,
	})

	promoter := &scheduler.Promoter{Store: st, Logger: log}
	if _, err := promoter.PromoteOnce(ctx); err != nil {
		t.Fatalf("PromoteOnce: %v", err)
	}

	// No handler registered for "no_such_handler" — the pool has zero handlers.
	pool := worker.NewPool(st, "integration-worker-2", 1, 30*time.Second, 100*time.Millisecond, log)

	workerCtx, workerCancel := context.WithTimeout(ctx, 3*time.Second)
	done := make(chan error, 1)
	go func() { done <- pool.Run(workerCtx) }()

	status := waitForTerminalRun(t, ctx, st, job.ID, 3*time.Second)
	workerCancel()
	<-done

	if status != model.RunStatusDead {
		t.Fatalf("expected run to be dead-lettered, got status %q", status)
	}
}

func createJob(t *testing.T, baseURL, token string, body map[string]any) *model.Job {
	t.Helper()

	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/jobs", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/jobs: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /v1/jobs: expected 201, got %d", resp.StatusCode)
	}

	var job model.Job
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		t.Fatalf("decode job response: %v", err)
	}
	return &job
}

func waitForTerminalRun(t *testing.T, ctx context.Context, st store.Store, jobID string, timeout time.Duration) model.RunStatus {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		runs, err := st.ListJobRuns(ctx, jobID, 1)
		if err == nil && len(runs) == 1 {
			switch runs[0].Status {
			case model.RunStatusSucceeded, model.RunStatusFailed, model.RunStatusDead:
				return runs[0].Status
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("run for job %s did not reach a terminal state within %s", jobID, timeout)
	return ""
}
