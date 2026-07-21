# Verification: real runs, real numbers

Every claim in this document was produced by actually running the system, not by
inspecting the code and asserting behavior. Commands are included so any of this can
be reproduced locally (`docker compose up -d postgres`, build the three binaries with
`go build ./cmd/...`, run them with the env vars shown).

## Automated tests

- `go test ./... -race` — unit tests for `internal/scheduler` and `internal/worker`
  (in-memory fake store), all passing.
- `go test ./integration/... -race` (requires `DATABASE_URL` pointed at a real
  Postgres) — two end-to-end tests against the real store, HTTP API, promoter, and
  worker pool together: `TestJobLifecycleEndToEnd` (create → promote → lease →
  execute → succeeded) and `TestDeadLetterOnUnregisteredHandler` (create → promote →
  lease → no handler → dead-lettered). Both pass.

## Load test: `POST /v1/jobs` throughput

Run with `RATE_LIMIT_RPS=10000 RATE_LIMIT_BURST=10000` (rate limiting deliberately
raised out of the way here — see the separate rate-limit enforcement run below for
that behavior under its real default) via `k6 run loadtest/api_smoke.js`, ramping to
20 concurrent VUs over 75s total:

| Metric | Result |
|---|---|
| Total requests | 130,462 |
| Sustained throughput | 1,739 req/s |
| p50 latency | 6.75ms |
| p90 latency | 13.34ms |
| p95 latency | 16.92ms |
| Max latency | 144.1ms |
| Error rate | 0.00% |

This is a single-instance, single-Postgres-connection-pool number on a laptop, not a
production capacity claim — but it's a real measured ceiling for this code path, not
an assumption.

## Rate limiter: enforcement under real concurrent load

The per-client-IP token bucket (`internal/api/ratelimit.go`) defaults to
`RATE_LIMIT_RPS=20`, `RATE_LIMIT_BURST=40`. A 30-VU concurrent burst for 5 seconds
against that default (all traffic from one IP, as it would be from a single
misbehaving client) produced:

| Status | Count |
|---|---|
| 201 Created | 139 |
| 429 Too Many Requests | 165,502 |

139 ≈ `burst(40) + rps(20) × 5s(≈100)`, which is exactly what the token-bucket math
predicts — the limiter is not just present in code, it measurably caps throughput at
the configured rate under real concurrent pressure.

## Chaos test: worker crash mid-execution

Goal: prove a job survives its worker dying mid-run, without operator intervention.

1. Submitted a `sleep` job (`payload.seconds=10`, `max_attempts=3`,
   `timeout_seconds=30`) with `chaos-worker-1` running `LEASE_DURATION=5s`.
2. Confirmed via `GET /v1/jobs/{id}/runs` that `chaos-worker-1` leased and started
   executing it (`status=running`, `lease_expires_at` ~5s out).
3. `kill -9`'d `chaos-worker-1` mid-sleep — no graceful shutdown, no chance to report
   failure.
4. Started `chaos-worker-2` (`LEASE_DURATION=15s`) against the same database.

Actual log output from `chaos-worker-2`:

```
{"level":"INFO","msg":"worker starting","worker_id":"chaos-worker-2"}
{"level":"WARN","msg":"reclaimed expired leases","count":1}
{"level":"INFO","msg":"job run succeeded","job_name":"sleep"}
```

Final run state: `status=succeeded`, `leased_by=chaos-worker-2`, `attempt=3`,
`result={"slept_seconds":10}`. The `taskflow_leases_reclaimed_total` counter read `1`
on worker2's `/metrics` — the reclaim janitor (`internal/worker/janitor.go`) actually
detected and repaired the abandoned lease, and the job completed successfully on a
different worker with no manual intervention. This is the concrete mechanism behind
the "crash recovery" and "at-least-once execution" claims in
[ARCHITECTURE.md](ARCHITECTURE.md) — not a theoretical property, an observed one.

Note the attempt count reached 3 (not 2): `chaos-worker-1`'s own janitor reclaimed its
first (already-expired) lease and re-leased it to itself once before the process was
killed, incrementing the attempt counter a second time — visible evidence that lease
expiry is checked independent of which worker owns it, including by the original
worker itself.

## Distributed tracing: a real trace pulled from Jaeger

Started Postgres + a bundled Jaeger (`jaegertracing/all-in-one`, OTLP/HTTP on 4318)
via `docker compose up -d postgres jaeger`, ran the `api` binary with
`OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4318`, minted a token, and did:

```
curl -X POST http://localhost:8080/v1/jobs -H "Authorization: Bearer $TOKEN" \
  -d '{"name":"echo","payload":{"trace":"check"},"max_attempts":2,"timeout_seconds":10}'
```

Then queried Jaeger's own API (`GET /api/traces?service=api`) rather than eyeballing
the UI, to get the raw span data:

```
$ curl -s "http://localhost:16686/api/services"
{"data":["jaeger-all-in-one","api"],"total":2,...}

$ curl -s "http://localhost:16686/api/traces/<trace-id>" | ...
7ffdf80e... | pg.query | parent: [b2e983ee...] | db.statement: begin
f17eb48e... | pg.query | parent: [b2e983ee...] | db.statement: INSERT INTO jobs (...)
d9289ea5... | pg.query | parent: [b2e983ee...] | db.statement: commit
b2e983ee... | POST     | parent: []           |
```

The `POST` span (from `otelhttp.NewHandler` wrapping the router in `cmd/api/main.go`)
is the parent of all three query spans (from the `pgx.QueryTracer` in
`internal/store/tracing.go`) inside the transaction `CreateJob` runs. This confirms
the instrumentation produces real linked parent/child traces for a request, not just
disconnected spans that happen to share a service name — pulled from a running
collector, not inferred from source. See
[ARCHITECTURE.md](ARCHITECTURE.md#distributed-tracing) for what's *not* yet linked
(the async hop from promoter to worker).

## Caching

Started Postgres + Redis (`docker compose up -d postgres redis`), ran `api` with
`REDIS_ADDR=localhost:6379`, created a job, then:

```
$ curl -w "time: %{time_total}s\n" .../v1/jobs/<id>   # 1st call, cold
time: 0.059545s
$ redis-cli GET taskflow:job:<id>
{"id":"...","name":"echo",...,"status":"active",...}   # confirmed actually cached
$ curl -w "time: %{time_total}s\n" .../v1/jobs/<id>   # 2nd call, cached
time: 0.000968s
$ curl .../metrics | grep taskflow_cache
taskflow_cache_hits_total{entity="job"} 1
taskflow_cache_misses_total{entity="job"} 1
```

~60x faster on the cached read (59.5ms → 0.97ms), and the hit/miss counters matched
exactly what happened (one miss, then one hit) — not just present in code, observed.

Then, to confirm invalidation actually prevents staleness (not just "the code calls
Del()"): paused the same job via `POST /v1/jobs/<id>/pause`, then immediately
`GET /v1/jobs/<id>` again. Response showed `"status":"paused"` on the very next read —
no stale `"active"` was served during the 30s TTL window that would otherwise still
have been valid. The pause handler's own `GetJob` call (to return the updated job in
its response) repopulates the cache with the fresh value, so the entry that exists in
Redis afterward is correct, not just absent.
