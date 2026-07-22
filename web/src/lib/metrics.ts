export interface MetricSample {
  name: string
  labels: Record<string, string>
  value: number
}

export function gauge(samples: MetricSample[], name: string): number | undefined {
  return samples.find((s) => s.name === name)?.value
}

export function counterTotal(samples: MetricSample[], name: string): number {
  return samples.filter((s) => s.name === name).reduce((sum, s) => sum + s.value, 0)
}

export function counterByLabel(
  samples: MetricSample[],
  name: string,
  labelKey: string,
): Record<string, number> {
  const out: Record<string, number> = {}
  for (const s of samples) {
    if (s.name !== name) continue
    const key = s.labels[labelKey] ?? 'unknown'
    out[key] = (out[key] ?? 0) + s.value
  }
  return out
}

export interface Bucket {
  le: number
  count: number
}

/** Sums cumulative bucket counts across all label combinations for one histogram metric. */
export function histogramBuckets(samples: MetricSample[], name: string): Bucket[] {
  const merged = new Map<number, number>()
  for (const s of samples) {
    if (s.name !== `${name}_bucket`) continue
    const le = s.labels.le === '+Inf' ? Infinity : Number(s.labels.le)
    merged.set(le, (merged.get(le) ?? 0) + s.value)
  }
  return Array.from(merged.entries())
    .map(([le, count]) => ({ le, count }))
    .sort((a, b) => a.le - b.le)
}

/** Converts cumulative (le-based) bucket counts into per-bucket observation counts. */
export function toNonCumulative(buckets: Bucket[]): Bucket[] {
  let prev = 0
  return buckets.map((b) => {
    const count = Math.max(0, b.count - prev)
    prev = b.count
    return { le: b.le, count }
  })
}

export function formatSeconds(le: number): string {
  if (!Number.isFinite(le)) return '∞'
  if (le < 1) return `${Math.round(le * 1000)}ms`
  return `${le}s`
}
