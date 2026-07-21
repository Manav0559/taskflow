package lock

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// postgresElector holds a single dedicated connection (checked out of pool for the
// elector's lifetime) because pg_try_advisory_lock is session-scoped: if we ran it
// through the pool normally, the connection could be handed to another goroutine
// between calls and the "lock" would mean nothing.
type postgresElector struct {
	pool    *pgxpool.Pool
	lockKey int64

	mu      sync.Mutex
	conn    *pgxpool.Conn
	isLeader bool
}

func newPostgresElector(pool *pgxpool.Pool, lockKey int64) *postgresElector {
	return &postgresElector{pool: pool, lockKey: lockKey}
}

func (e *postgresElector) TryAcquire(ctx context.Context) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.conn == nil {
		conn, err := e.pool.Acquire(ctx)
		if err != nil {
			return false, err
		}
		e.conn = conn
	}

	// Ping to detect a dead connection (e.g. DB restart) and recover by re-acquiring.
	if err := e.conn.Ping(ctx); err != nil {
		e.conn.Release()
		e.conn = nil
		e.isLeader = false
		conn, aerr := e.pool.Acquire(ctx)
		if aerr != nil {
			return false, aerr
		}
		e.conn = conn
	}

	if e.isLeader {
		return true, nil
	}

	var acquired bool
	err := e.conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", e.lockKey).Scan(&acquired)
	if err != nil {
		return false, err
	}
	e.isLeader = acquired
	return acquired, nil
}

func (e *postgresElector) Release(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.conn == nil || !e.isLeader {
		return nil
	}
	_, err := e.conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", e.lockKey)
	e.isLeader = false
	if e.conn != nil {
		e.conn.Release()
		e.conn = nil
	}
	if err == pgx.ErrNoRows {
		return nil
	}
	return err
}
