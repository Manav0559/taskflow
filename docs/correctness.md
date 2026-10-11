# Correctness: leases, fencing, SKIP LOCKED, leader failover

This page describes what taskflow guarantees about who may change a `job_runs` row,
and where those guarantees stop. Everything here refers to code in
`internal/store/postgres.go`, `internal/worker/pool.go` and `internal/lock/`.

## Leases

A run is executed under a **lease**: a claim by one worker, recorded on the row itself.

| column             | meaning                                                      |
|--------------------|--------------------------------------------------------------|
| `leased_by`        | worker ID holding the lease (`WORKER_ID`)                    |
| `attempt`          | incremented every time the run is leased                     |
| `lease_expires_at` | the janitor may reclaim the run after this time              |

`LeaseNextRun` claims a run in one transaction:

1. `SELECT id, job_id FROM job_runs WHERE status = 'pending' AND scheduled_at <= now()
   ORDER BY priority DESC, scheduled_at ASC LIMIT 1 FOR UPDATE SKIP LOCKED`
2. `UPDATE job_runs SET status = 'leased', leased_by = $worker, leased_at = now(),
   lease_expires_at = now() + $lease, attempt = attempt + 1 WHERE id = $id`

While the handler runs, the worker calls `ExtendLease` every `LEASE_DURATION / 3`.
Renewal stops when the handler returns or when its `timeout_seconds` context ends, so a
handler that ignores cancellation loses its lease one `LEASE_DURATION` later instead of
holding it forever.

Every worker also runs a janitor (`ReclaimExpiredLeases`, every `5 * POLL_INTERVAL`)
that moves any `leased`/`running` row whose `lease_expires_at` has passed back to
`pending` and clears `leased_by`. That is how work held by a crashed worker is
recovered.

## SKIP LOCKED

`FOR UPDATE` takes a row lock on the selected candidate; `SKIP LOCKED` makes a
concurrent `LeaseNextRun` skip rows another transaction has locked instead of waiting
on them. Two workers polling at the same time therefore claim different runs, and
neither blocks on the other. The lock is held until the leasing transaction commits,
after which the row is no longer `pending` and drops out of the candidate set.

`SKIP LOCKED` only protects the claim. It says nothing about what happens after the
lease ends, which is what fencing covers.

## Fencing

A lease is time-based, and a worker cannot know that its lease has expired: a GC pause,
a network partition or a slow handler can let the janitor reclaim the run and a second
worker lease it while the first is still running. Without fencing, the first worker's
later `CompleteRun` or `FailRun` would overwrite the second worker's state (a lost
update).

The fencing token is the pair `(leased_by, attempt)`, carried in `store.Lease` and built
with `store.LeaseOf(run, workerID)` from the row `LeaseNextRun` returned. Because
`attempt` is incremented in the same transaction that sets `leased_by`, the pair is
unique per lease, and the same worker ID re-leasing the run gets a new token.

Every lifecycle write adds the token to its `WHERE` clause, plus a status guard:

| method                 | applies only when                                                   |
|------------------------|---------------------------------------------------------------------|
| `ExtendLease`          | `leased_by = $w AND attempt = $a AND status IN ('leased','running')` |
| `MarkRunning`          | `leased_by = $w AND attempt = $a AND status = 'leased'`              |
| `CompleteRun`          | `leased_by = $w AND attempt = $a AND status IN ('leased','running')` |
| `FailRun`              | `leased_by = $w AND attempt = $a AND status IN ('leased','running')` |
| `MarkDead`             | `leased_by = $w AND attempt = $a AND status = 'failed'` (`SELECT ... FOR UPDATE` in the dead-letter transaction) |

If the `UPDATE` affects zero rows the store checks whether the run exists and returns
`store.ErrNotFound` if it does not, otherwise `store.ErrLeaseLost`.

The janitor's reclaim and a worker's fenced write both update the same row, so Postgres
serializes them on the row lock. Either the write lands first (the run finishes and is
no longer `leased`/`running`, so the reclaim skips it) or the reclaim lands first
(`leased_by` is now `NULL`, so the write matches nothing). There is no interleaving in
which both take effect.

The fenced writes do not compare `lease_expires_at` with `now()`. A worker that finishes
just after its lease expired, but before anyone reclaimed the run, still records its
result. Nobody else held the run at that point, so this is safe and saves a re-run.

### What the worker does with `ErrLeaseLost`

`ErrLeaseLost` means the run belongs to someone else (or is pending again). The worker
logs a warning, increments `taskflow_leases_lost_total`, and stops processing that run.
It does not retry the write, which would fail the same way, and it does not fall
through to `FailRun`/`MarkDead`. If renewal is what discovers the loss, the handler's
context is cancelled and no result is written.

### What fencing does not cover

Fencing protects the `job_runs` row. It does not undo side effects a stale handler has
already performed outside the database. Execution is **at-least-once**: after a lease
expiry, two workers may both run the handler, and only the current lease holder's
result is recorded. Handlers with external side effects should be idempotent, for
example keyed on `run.ID` or `job.IdempotencyKey`.

## Leader failover

Promotion (turning due jobs into `pending` runs) is done by one scheduler replica at a
time. The leader holds a session-scoped Postgres advisory lock
(`pg_try_advisory_lock(727433001)`) on a connection it keeps checked out of the pool.
`Promoter.Run` calls `TryAcquire` on every tick before promoting. `TryAcquire` pings
that connection first and drops leadership if the ping fails. If the leader process
dies or its connection is closed, Postgres releases the lock and the next replica to
call `TryAcquire` takes over. The measured failover is recorded in
[VERIFICATION.md](VERIFICATION.md#leader-election-failover).

Promotion is not fenced the way run writes are. `PromoteOnce` checks `HasActiveRun` and
then calls `CreateRun` on a pooled connection, not on the lock-holding connection, and
there is no unique constraint on "one active run per job". If the old leader's session
is lost in the middle of a `PromoteOnce` pass, it can finish that pass while a new
leader starts one, and both may create a run for the same job. The window is at most
one pass. Closing it would need either a partial unique index on
`job_runs (job_id) WHERE status IN ('pending','leased','running')` or doing the check
and insert under the advisory-lock connection in one transaction.

## Known gaps

- **Duplicate promotion during failover**, described above.
- **`FailRun` + `MarkDead` are two transactions.** If a worker crashes between them, the
  run stays `failed` with no `dead_letters` row. The janitor only reclaims
  `leased`/`running` rows, so nothing repairs it automatically.
- **At-least-once execution.** Fencing keeps the stored result consistent. It does not
  stop a handler from running twice.
