# taskflow dashboard

A React + TypeScript + Tailwind dashboard for the [taskflow](../README.md) job
scheduler: browse and manage jobs, inspect runs, watch worker health, and see
cluster-wide metrics — queue depth, cache hit rate, scheduler leader status,
request/run latency, and per-worker throughput.

## Stack

- React 19 + Vite + TypeScript
- Tailwind CSS v4 (CSS-first config, `src/index.css`)
- TanStack Query for data fetching/polling/mutations
- React Router for navigation
- Recharts for time-series and distribution charts
- A validated, colorblind-safe color system (see `src/lib/palette.ts`) — categorical,
  sequential, and status hues each pass the six-check gate (CVD ΔE, contrast, dark
  mode) rather than being picked by eye

## Running it

```bash
npm install
npm run dev
```

Open `http://localhost:5173`. On first load you'll be asked for:

- **API base URL** — the taskflow API, `http://localhost:8080` by default
- **Bearer token** — taskflow has no login endpoint; mint one as described in the
  [main README](../README.md#getting-a-token) and paste it in

Both are stored in `localStorage`, not committed anywhere. Override the default API
URL at build time with `VITE_API_BASE_URL`.

### Prometheus (optional, for cluster metrics)

Queue depth, scheduler leader status, run-outcome counts, request/run latency
distributions, and per-worker lease counts are aggregated across the `api`,
`worker`, and `scheduler` services — no single service's own `/metrics` endpoint
carries all of them. The dashboard gets these directly from Prometheus's HTTP
query API (`/api/v1/query`, `/api/v1/query_range`) rather than duplicating that
aggregation logic client-side.

Configured separately in Settings (default `http://localhost:9093`, matching the
`prometheus` service in `docker-compose.yml`). If it's unreachable, the rest of the
dashboard (jobs, runs, workers) still works — Overview just shows those panels as
unavailable. Override the default at build time with `VITE_PROMETHEUS_URL`.

The docker-compose `prometheus` service is started with `--web.cors.origin=.*` so
the browser can call it directly; see the comment there for why.

## Why the API needed a change

The Go API had no CORS support (browser dashboards need it; server-to-server
Prometheus scraping doesn't). `internal/api/router.go` now takes a `corsOrigins
[]string` and applies `github.com/go-chi/cors`, configured via the new
`CORS_ALLOWED_ORIGINS` env var (comma-separated, defaults to `*`). This doesn't
affect the auth model — bearer-token auth, unlike cookies, is CORS-safe with a
wildcard origin since there's no ambient credential a third-party page could ride
along with.

## Structure

```
src/
  lib/
    api.ts              REST client for the taskflow API
    prom.ts              Prometheus HTTP API client (instant + range queries)
    observability.ts     react-query hooks combining the two into dashboard-ready data
    metrics.ts            Prometheus sample selectors (gauge/counter/histogram helpers)
    connection.tsx        API/token/Prometheus URL state (localStorage-backed)
    theme.tsx              light/dark/system theme state
    palette.ts             the validated color tokens, resolved per theme
  components/            Sidebar, Topbar, status badges, stat tiles, charts, modals
  pages/                 Overview, Jobs, JobDetail, RunDetail, Workers, Settings
```
