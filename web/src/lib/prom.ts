import type { MetricSample } from '@/lib/metrics'

export interface PromInstantSample {
  metric: Record<string, string>
  value: [number, string]
}

export interface PromRangeSample {
  metric: Record<string, string>
  values: [number, string][]
}

interface PromResponse<T> {
  status: 'success' | 'error'
  data?: { resultType: string; result: T[] }
  error?: string
}

export class PromError extends Error {}

export class PromClient {
  private baseUrl: string

  constructor(baseUrl: string) {
    this.baseUrl = baseUrl
  }

  // Deliberately queries /api/v1/query rather than /-/healthy: Prometheus's
  // --web.cors.origin flag only covers the query API, so /-/healthy has no
  // Access-Control-Allow-Origin header and a browser fetch to it is blocked
  // by CORS even when Prometheus is perfectly reachable.
  async healthy(): Promise<boolean> {
    try {
      await this.instantQuery('1')
      return true
    } catch {
      return false
    }
  }

  async instantQuery(query: string): Promise<PromInstantSample[]> {
    const res = await fetch(`${this.baseUrl}/api/v1/query?query=${encodeURIComponent(query)}`)
    if (!res.ok) throw new PromError(`Prometheus query failed (${res.status})`)
    const body = (await res.json()) as PromResponse<PromInstantSample>
    if (body.status !== 'success') throw new PromError(body.error ?? 'Prometheus query error')
    return body.data?.result ?? []
  }

  async rangeQuery(query: string, startSec: number, endSec: number, stepSec: number): Promise<PromRangeSample[]> {
    const params = new URLSearchParams({
      query,
      start: String(startSec),
      end: String(endSec),
      step: String(stepSec),
    })
    const res = await fetch(`${this.baseUrl}/api/v1/query_range?${params}`)
    if (!res.ok) throw new PromError(`Prometheus range query failed (${res.status})`)
    const body = (await res.json()) as PromResponse<PromRangeSample>
    if (body.status !== 'success') throw new PromError(body.error ?? 'Prometheus query error')
    return body.data?.result ?? []
  }
}

/** Adapts an instant-query result vector into the flat MetricSample shape the
 * metrics.ts selectors (gauge/counterByLabel/histogramBuckets) already understand,
 * so panels can consume Prometheus- and scrape-endpoint-sourced data uniformly. */
export function instantToSamples(name: string, result: PromInstantSample[]): MetricSample[] {
  return result.map((r) => ({ name, labels: r.metric, value: Number(r.value[1]) }))
}
