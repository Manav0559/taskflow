// Package lock provides leader election so exactly one scheduler replica promotes
// DAG-ready jobs into runs at a time (multiple replicas may run for availability,
// but promotion must not double-fire). Backed by Postgres advisory locks rather than
// a separate consensus system (etcd/ZooKeeper): the scheduler already depends on
// Postgres as its system of record, so this avoids introducing a second failure domain
// purely for leader election. Trade-off discussed in docs/ARCHITECTURE.md.
package lock

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Elector attempts to acquire/renew a single named leadership slot.
type Elector interface {
	// TryAcquire attempts to become (or remain) leader. It is safe to call repeatedly
	// on a poll loop; a session-scoped advisory lock is held for as long as the
	// underlying connection is alive, and released automatically if the process dies.
	TryAcquire(ctx context.Context) (bool, error)
	// Release gives up leadership immediately (graceful shutdown).
	Release(ctx context.Context) error
}

// NewPostgresElector returns an Elector using pg_try_advisory_lock(lockKey) on a
// dedicated held connection from pool.
func NewPostgresElector(pool *pgxpool.Pool, lockKey int64) Elector {
	return newPostgresElector(pool, lockKey)
}
