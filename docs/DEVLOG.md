# Devlog

taskflow was built by parallelizing implementation across multiple independent AI
agents working against a pre-fixed set of shared interfaces, rather than one agent
building the system linearly front to back.

Before any feature code was written, the contracts that cross package boundaries were
nailed down first: `store.Store` (every method the API, scheduler, and worker need from
persistence — job/run CRUD, `LeaseNextRun`, `ReclaimExpiredLeases`, and so on),
`lock.Elector` (`TryAcquire`/`Release`, independent of *how* leadership is implemented),
and the `model` package's types (`Job`, `JobRun`, `Worker`, `NewJobInput`), which
deliberately has zero dependencies on anything else so nothing importing it could create
a cycle.

With those fixed, five workstreams went on in parallel with no shared mutable state
between them:

- **Postgres storage** — the schema (`migrations/0001_init.sql`) and the concrete
  `store.Store` implementation, including the `SELECT ... FOR UPDATE SKIP LOCKED` lease
  claim.
- **Scheduler / DAG logic** — cron-due calculation, dependency-satisfaction checks, and
  the leader-elected promotion loop, built and testable against the `store.Store`
  interface alone.
- **Worker pool** — the lease/execute/complete loop, retry/backoff, lease-extension and
  janitor reclaim, again built against `store.Store` without caring which database sat
  behind it.
- **HTTP API** — routing, request validation, JWT auth, and the rate limiter, built
  against `store.Store` and the `model` types.
- **Infra** — Dockerfiles, docker-compose, Kubernetes manifests, Terraform, and CI,
  built against the *shape* of the three binaries (three services, each exposing a main
  port plus a metrics port) before all of their internals existed.

Each of these could be implemented, and largely tested, without knowing the internal
details of the others — only the interfaces. That's what let them proceed concurrently
instead of serially.

The one phase that couldn't be parallelized was integration: wiring the three real
`cmd/api`, `cmd/scheduler`, `cmd/worker` binaries together, making sure `config.Load`
covered every environment variable each service actually needed, confirming the
migration runner behaved correctly when three processes raced to apply it on cold start,
and confirming the promoter and worker pool actually agreed on run semantics end to end
against a real Postgres instance. This was a manual pass, not an agent handoff — someone
has to actually run the system and watch a job go from `pending` to `succeeded` to
believe the interfaces were right, not just internally consistent.

The broader point this project is meant to demonstrate: defining stable interfaces up
front is what makes parallel work — across multiple engineers, or across multiple
agents — merge cleanly instead of producing integration hell. The `store.Store`
interface didn't change shape once the parallel work started; that discipline is the
reason five concurrently-built pieces came together with one integration pass instead of
five.
