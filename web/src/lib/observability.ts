import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { PromClient, instantToSamples } from '@/lib/prom'
import type { MetricSample } from '@/lib/metrics'
import { useConnection } from '@/lib/connection'

const QUERIES: { name: string; expr: string }[] = [
  { name: 'taskflow_queue_depth', expr: 'max(taskflow_queue_depth)' },
  { name: 'taskflow_scheduler_is_leader', expr: 'taskflow_scheduler_is_leader' },
  { name: 'taskflow_jobs_submitted_total', expr: 'sum(taskflow_jobs_submitted_total)' },
  { name: 'taskflow_cache_hits_total', expr: 'sum(taskflow_cache_hits_total) by (entity)' },
  { name: 'taskflow_cache_misses_total', expr: 'sum(taskflow_cache_misses_total) by (entity)' },
  { name: 'taskflow_runs_completed_total', expr: 'sum(taskflow_runs_completed_total) by (outcome)' },
  { name: 'taskflow_leases_reclaimed_total', expr: 'sum(taskflow_leases_reclaimed_total)' },
  { name: 'taskflow_runs_leased_total', expr: 'sum(taskflow_runs_leased_total) by (worker_id)' },
  {
    name: 'taskflow_http_request_duration_seconds_bucket',
    expr: 'sum(taskflow_http_request_duration_seconds_bucket) by (le)',
  },
  { name: 'taskflow_run_duration_seconds_bucket', expr: 'sum(taskflow_run_duration_seconds_bucket) by (le)' },
]

export function usePromClient(): PromClient {
  const { prometheusUrl } = useConnection()
  return useMemo(() => new PromClient(prometheusUrl), [prometheusUrl])
}

export function usePromHealthy() {
  const client = usePromClient()
  return useQuery({
    queryKey: ['prom-healthy', client],
    queryFn: () => client.healthy(),
    refetchInterval: 15_000,
    retry: false,
  })
}

/** Cluster-wide metrics aggregated by Prometheus across the api/worker/scheduler
 * scrape targets — data that no single service's own /metrics endpoint carries. */
export function useObservability() {
  const client = usePromClient()
  return useQuery({
    queryKey: ['observability', client],
    queryFn: async (): Promise<MetricSample[]> => {
      const results = await Promise.all(QUERIES.map((q) => client.instantQuery(q.expr)))
      return results.flatMap((result, i) => instantToSamples(QUERIES[i].name, result))
    },
    refetchInterval: 15_000,
    retry: false,
  })
}

export function useQueueDepthHistory(windowSeconds = 900, stepSeconds = 15) {
  const client = usePromClient()
  return useQuery({
    queryKey: ['queue-depth-history', client, windowSeconds, stepSeconds],
    queryFn: async () => {
      const end = Math.floor(Date.now() / 1000)
      const start = end - windowSeconds
      const series = await client.rangeQuery('max(taskflow_queue_depth)', start, end, stepSeconds)
      const points = series[0]?.values ?? []
      return points.map(([t, v]) => ({ t: t * 1000, value: Number(v) }))
    },
    refetchInterval: 15_000,
    retry: false,
  })
}
