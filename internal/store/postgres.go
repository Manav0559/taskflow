package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/manavsingla/taskflow/internal/model"
)

const jobColumns = `id, name, payload, cron_expr, priority, max_attempts, timeout_seconds, status, idempotency_key, created_at, updated_at`

const runColumns = `id, job_id, status, attempt, priority, scheduled_at, leased_by, leased_at, lease_expires_at, started_at, finished_at, result, error, created_at`

// PostgresStore is the production store.Store implementation backed by Postgres.
type PostgresStore struct {
	pool *pgxpool.Pool
}

var _ Store = (*PostgresStore)(nil)

// querier is satisfied by both *pgxpool.Pool and pgx.Tx, letting helpers run either
// standalone or as part of a caller-managed transaction (e.g. LeaseNextRun, MarkDead).
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// rowScanner is satisfied by both pgx.Row and pgx.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func New(ctx context.Context, databaseURL string) (*PostgresStore, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("create pgx pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return &PostgresStore{pool: pool}, nil
}

func (s *PostgresStore) Close() {
	s.pool.Close()
}

// Pool exposes the underlying connection pool so callers (e.g. cmd/* main.go) can run
// RunMigrations against the same pool the store uses, rather than opening a second one.
func (s *PostgresStore) Pool() *pgxpool.Pool {
	return s.pool
}

func scanJobRow(ctx context.Context, q querier, row rowScanner) (*model.Job, error) {
	var job model.Job
	var payloadRaw []byte
	if err := row.Scan(&job.ID, &job.Name, &payloadRaw, &job.CronExpr, &job.Priority, &job.MaxAttempts,
		&job.TimeoutSeconds, &job.Status, &job.IdempotencyKey, &job.CreatedAt, &job.UpdatedAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(payloadRaw, &job.Payload); err != nil {
		return nil, fmt.Errorf("unmarshal payload: %w", err)
	}
	deps, err := fetchDependencies(ctx, q, job.ID)
	if err != nil {
		return nil, err
	}
	job.DependsOn = deps
	return &job, nil
}

func fetchJob(ctx context.Context, q querier, id string) (*model.Job, error) {
	row := q.QueryRow(ctx, `SELECT `+jobColumns+` FROM jobs WHERE id = $1`, id)
	return scanJobRow(ctx, q, row)
}

func fetchJobByIdempotencyKey(ctx context.Context, q querier, key string) (*model.Job, error) {
	row := q.QueryRow(ctx, `SELECT `+jobColumns+` FROM jobs WHERE idempotency_key = $1`, key)
	return scanJobRow(ctx, q, row)
}

func fetchDependencies(ctx context.Context, q querier, jobID string) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT depends_on_id FROM job_dependencies WHERE job_id = $1`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deps []string
	for rows.Next() {
		var dep string
		if err := rows.Scan(&dep); err != nil {
			return nil, err
		}
		deps = append(deps, dep)
	}
	return deps, rows.Err()
}

func scanRun(row rowScanner) (*model.JobRun, error) {
	var run model.JobRun
	var resultRaw []byte
	if err := row.Scan(&run.ID, &run.JobID, &run.Status, &run.Attempt, &run.Priority, &run.ScheduledAt,
		&run.LeasedBy, &run.LeasedAt, &run.LeaseExpiresAt, &run.StartedAt, &run.FinishedAt, &resultRaw,
		&run.Error, &run.CreatedAt); err != nil {
		return nil, err
	}
	if resultRaw != nil {
		if err := json.Unmarshal(resultRaw, &run.Result); err != nil {
			return nil, fmt.Errorf("unmarshal result: %w", err)
		}
	}
	return &run, nil
}

func fetchRun(ctx context.Context, q querier, id string) (*model.JobRun, error) {
	row := q.QueryRow(ctx, `SELECT `+runColumns+` FROM job_runs WHERE id = $1`, id)
	return scanRun(row)
}

func (s *PostgresStore) CreateJob(ctx context.Context, in model.NewJobInput) (*model.Job, error) {
	payloadRaw, err := json.Marshal(in.Payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var job model.Job
	var outRaw []byte
	row := tx.QueryRow(ctx, `
		INSERT INTO jobs (name, payload, cron_expr, priority, max_attempts, timeout_seconds, idempotency_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+jobColumns, in.Name, payloadRaw, in.CronExpr, in.Priority, in.MaxAttempts, in.TimeoutSeconds, in.IdempotencyKey)

	if err := row.Scan(&job.ID, &job.Name, &outRaw, &job.CronExpr, &job.Priority, &job.MaxAttempts,
		&job.TimeoutSeconds, &job.Status, &job.IdempotencyKey, &job.CreatedAt, &job.UpdatedAt); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrIdempotencyConflict
		}
		return nil, fmt.Errorf("insert job: %w", err)
	}

	if err := json.Unmarshal(outRaw, &job.Payload); err != nil {
		return nil, fmt.Errorf("unmarshal payload: %w", err)
	}

	for _, dep := range in.DependsOn {
		if _, err := tx.Exec(ctx, `INSERT INTO job_dependencies (job_id, depends_on_id) VALUES ($1, $2)`, job.ID, dep); err != nil {
			return nil, fmt.Errorf("insert dependency: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	job.DependsOn = in.DependsOn
	return &job, nil
}

func (s *PostgresStore) GetJob(ctx context.Context, id string) (*model.Job, error) {
	job, err := fetchJob(ctx, s.pool, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return job, nil
}

func (s *PostgresStore) GetJobByIdempotencyKey(ctx context.Context, key string) (*model.Job, error) {
	job, err := fetchJobByIdempotencyKey(ctx, s.pool, key)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return job, nil
}

func (s *PostgresStore) ListJobs(ctx context.Context, status *model.JobStatus, limit, offset int) ([]*model.Job, error) {
	var rows pgx.Rows
	var err error
	if status != nil {
		rows, err = s.pool.Query(ctx, `SELECT `+jobColumns+` FROM jobs WHERE status = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
			*status, limit, offset)
	} else {
		rows, err = s.pool.Query(ctx, `SELECT `+jobColumns+` FROM jobs ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
			limit, offset)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*model.Job
	for rows.Next() {
		var job model.Job
		var payloadRaw []byte
		if err := rows.Scan(&job.ID, &job.Name, &payloadRaw, &job.CronExpr, &job.Priority, &job.MaxAttempts,
			&job.TimeoutSeconds, &job.Status, &job.IdempotencyKey, &job.CreatedAt, &job.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(payloadRaw, &job.Payload); err != nil {
			return nil, fmt.Errorf("unmarshal payload: %w", err)
		}
		jobs = append(jobs, &job)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, j := range jobs {
		deps, err := fetchDependencies(ctx, s.pool, j.ID)
		if err != nil {
			return nil, err
		}
		j.DependsOn = deps
	}
	return jobs, nil
}

func (s *PostgresStore) UpdateJobStatus(ctx context.Context, id string, status model.JobStatus) error {
	tag, err := s.pool.Exec(ctx, `UPDATE jobs SET status = $1, updated_at = now() WHERE id = $2`, status, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) ListDependencies(ctx context.Context, jobID string) ([]string, error) {
	return fetchDependencies(ctx, s.pool, jobID)
}

func (s *PostgresStore) LatestRunForJob(ctx context.Context, jobID string) (*model.JobRun, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+runColumns+` FROM job_runs WHERE job_id = $1 ORDER BY created_at DESC LIMIT 1`, jobID)
	run, err := scanRun(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return run, nil
}

func (s *PostgresStore) HasActiveRun(ctx context.Context, jobID string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM job_runs WHERE job_id = $1 AND status IN ('pending','leased','running'))
	`, jobID).Scan(&exists)
	return exists, err
}

func (s *PostgresStore) CreateRun(ctx context.Context, jobID string, priority int16, scheduledAt time.Time) (*model.JobRun, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO job_runs (job_id, priority, scheduled_at)
		VALUES ($1, $2, $3)
		RETURNING `+runColumns, jobID, priority, scheduledAt)
	return scanRun(row)
}

func (s *PostgresStore) LeaseNextRun(ctx context.Context, workerID string, leaseDuration time.Duration) (*model.JobRun, *model.Job, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx)

	var runID, jobID string
	err = tx.QueryRow(ctx, `
		SELECT id, job_id FROM job_runs
		WHERE status = 'pending' AND scheduled_at <= now()
		ORDER BY priority DESC, scheduled_at ASC
		LIMIT 1 FOR UPDATE SKIP LOCKED
	`).Scan(&runID, &jobID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE job_runs
		SET status = 'leased', leased_by = $1, leased_at = now(), lease_expires_at = now() + $2, attempt = attempt + 1
		WHERE id = $3
	`, workerID, leaseDuration, runID); err != nil {
		return nil, nil, err
	}

	run, err := fetchRun(ctx, tx, runID)
	if err != nil {
		return nil, nil, err
	}

	job, err := fetchJob(ctx, tx, jobID)
	if err != nil {
		return nil, nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}

	return run, job, nil
}

func (s *PostgresStore) ExtendLease(ctx context.Context, runID string, workerID string, extend time.Duration) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE job_runs SET lease_expires_at = now() + $1 WHERE id = $2 AND leased_by = $3
	`, extend, runID, workerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) MarkRunning(ctx context.Context, runID string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE job_runs SET status = 'running', started_at = now() WHERE id = $1`, runID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) CompleteRun(ctx context.Context, runID string, result map[string]any) error {
	raw, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE job_runs SET status = 'succeeded', finished_at = now(), result = $1 WHERE id = $2
	`, raw, runID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) FailRun(ctx context.Context, runID string, errMsg string, requeue bool, backoff time.Duration) error {
	var tag pgconn.CommandTag
	var err error
	if requeue {
		tag, err = s.pool.Exec(ctx, `
			UPDATE job_runs
			SET status = 'pending', scheduled_at = now() + $1, error = $2,
			    leased_by = NULL, leased_at = NULL, lease_expires_at = NULL
			WHERE id = $3
		`, backoff, errMsg, runID)
	} else {
		tag, err = s.pool.Exec(ctx, `
			UPDATE job_runs SET status = 'failed', finished_at = now(), error = $1 WHERE id = $2
		`, errMsg, runID)
	}
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) MarkDead(ctx context.Context, runID string, reason string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	run, err := fetchRun(ctx, tx, runID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}

	payloadRaw, err := json.Marshal(map[string]any{"result": run.Result, "error": run.Error})
	if err != nil {
		return fmt.Errorf("marshal dead letter payload: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO dead_letters (job_run_id, reason, payload) VALUES ($1, $2, $3)
	`, runID, reason, payloadRaw); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `UPDATE job_runs SET status = 'dead' WHERE id = $1`, runID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (s *PostgresStore) ReclaimExpiredLeases(ctx context.Context) (int, error) {
	rows, err := s.pool.Query(ctx, `
		UPDATE job_runs
		SET status = 'pending', leased_by = NULL, leased_at = NULL, lease_expires_at = NULL
		WHERE status IN ('leased', 'running') AND lease_expires_at < now()
		RETURNING id
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *PostgresStore) GetRun(ctx context.Context, id string) (*model.JobRun, error) {
	run, err := fetchRun(ctx, s.pool, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return run, nil
}

func (s *PostgresStore) ListJobRuns(ctx context.Context, jobID string, limit int) ([]*model.JobRun, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+runColumns+` FROM job_runs WHERE job_id = $1 ORDER BY created_at DESC LIMIT $2`,
		jobID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []*model.JobRun
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (s *PostgresStore) CountPendingRuns(ctx context.Context) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM job_runs WHERE status = 'pending'`).Scan(&count)
	return count, err
}

func (s *PostgresStore) UpsertWorkerHeartbeat(ctx context.Context, workerID, hostname string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO workers (id, hostname, last_heartbeat, status)
		VALUES ($1, $2, now(), 'alive')
		ON CONFLICT (id) DO UPDATE SET hostname = EXCLUDED.hostname, last_heartbeat = now(), status = 'alive'
	`, workerID, hostname)
	return err
}

func (s *PostgresStore) ListWorkers(ctx context.Context) ([]*model.Worker, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, hostname, status, last_heartbeat, started_at FROM workers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workers []*model.Worker
	for rows.Next() {
		var w model.Worker
		if err := rows.Scan(&w.ID, &w.Hostname, &w.Status, &w.LastHeartbeat, &w.StartedAt); err != nil {
			return nil, err
		}
		workers = append(workers, &w)
	}
	return workers, rows.Err()
}
