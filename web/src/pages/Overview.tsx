import { AlertTriangle, Crown, Database, ListChecks, RotateCcw, Server } from 'lucide-react'
import { Card, CardHeader } from '@/components/ui/Card'
import { StatTile } from '@/components/StatTile'
import { StatusDistributionBar } from '@/components/charts/StatusDistributionBar'
import { HistogramChart } from '@/components/charts/HistogramChart'
import { Sparkline } from '@/components/charts/Sparkline'
import { JobStatusBadge } from '@/components/StatusBadge'
import { useJobs, useWorkers } from '@/lib/queries'
import { useObservability, usePromHealthy, useQueueDepthHistory } from '@/lib/observability'
import { gauge, counterTotal, counterByLabel, histogramBuckets } from '@/lib/metrics'
import { useTheme } from '@/lib/theme'
import { relativeTime } from '@/lib/format'
import { Link } from 'react-router-dom'

export function Overview() {
  const { colors } = useTheme()
  const { data: promHealthy } = usePromHealthy()
  const { data: samples, isLoading: obsLoading } = useObservability()
  const { data: queueHistory } = useQueueDepthHistory()
  const { data: jobs } = useJobs({ limit: 5 })
  const { data: workers } = useWorkers()

  const clusterAvailable = promHealthy && samples && !obsLoading

  const queueDepth = samples ? gauge(samples, 'taskflow_queue_depth') : undefined
  const jobsSubmitted = samples ? counterTotal(samples, 'taskflow_jobs_submitted_total') : undefined
  const leasesReclaimed = samples ? counterTotal(samples, 'taskflow_leases_reclaimed_total') : undefined

  const cacheHits = samples ? counterByLabel(samples, 'taskflow_cache_hits_total', 'entity') : {}
  const cacheMisses = samples ? counterByLabel(samples, 'taskflow_cache_misses_total', 'entity') : {}
  const totalHits = Object.values(cacheHits).reduce((a, b) => a + b, 0)
  const totalMisses = Object.values(cacheMisses).reduce((a, b) => a + b, 0)
  const cacheHitRate = totalHits + totalMisses > 0 ? (totalHits / (totalHits + totalMisses)) * 100 : undefined

  const leaders = (samples ?? []).filter(
    (s) => s.name === 'taskflow_scheduler_is_leader' && s.value === 1,
  )

  const runOutcomes = samples ? counterByLabel(samples, 'taskflow_runs_completed_total', 'outcome') : {}
  const httpBuckets = samples ? histogramBuckets(samples, 'taskflow_http_request_duration_seconds') : []
  const runDurationBuckets = samples ? histogramBuckets(samples, 'taskflow_run_duration_seconds') : []
  const leasedByWorker = samples ? counterByLabel(samples, 'taskflow_runs_leased_total', 'worker_id') : {}

  const aliveWorkers = (workers ?? []).filter((w) => w.status === 'alive').length
  const recentJobs = [...(jobs ?? [])].sort(
    (a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime(),
  )

  return (
    <div className="space-y-6">
      {!promHealthy && (
        <div className="flex items-center gap-2 rounded-lg border border-warning/30 bg-warning/10 px-4 py-3 text-sm text-ink">
          <AlertTriangle size={16} className="shrink-0 text-warning" />
          Prometheus isn't reachable at the configured URL, so cluster-wide metrics (queue depth,
          leader status, latency, worker throughput) are unavailable. The API connection itself is
          unaffected.{' '}
          <Link to="/settings" className="font-medium text-series-1 underline underline-offset-2">
            Check settings
          </Link>
        </div>
      )}

      <div className="grid grid-cols-2 gap-4 lg:grid-cols-5">
        <StatTile
          label="Queue depth"
          value={queueDepth !== undefined ? queueDepth : '—'}
          icon={<ListChecks size={16} />}
        />
        <StatTile
          label="Cache hit rate"
          value={cacheHitRate !== undefined ? `${cacheHitRate.toFixed(1)}%` : '—'}
          icon={<Database size={16} />}
        />
        <StatTile
          label="Jobs submitted"
          value={jobsSubmitted !== undefined ? jobsSubmitted : '—'}
          icon={<ListChecks size={16} />}
        />
        <StatTile
          label="Leases reclaimed"
          value={leasesReclaimed !== undefined ? leasesReclaimed : '—'}
          hint="self-healing after worker crashes"
          icon={<RotateCcw size={16} />}
        />
        <StatTile
          label="Workers alive"
          value={workers ? `${aliveWorkers}/${workers.length}` : '—'}
          icon={<Server size={16} />}
        />
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <CardHeader title="Queue depth" subtitle="max across scheduler replicas, last 15 min" />
          {clusterAvailable ? (
            <Sparkline
              data={queueHistory ?? []}
              color={colors.series[0]}
              gradientId="queue-depth-spark"
            />
          ) : (
            <div className="flex h-16 items-center text-sm text-ink-muted">Unavailable</div>
          )}
        </Card>
        <Card>
          <CardHeader
            title="Scheduler leader"
            subtitle={leaders.length === 1 ? 'healthy' : 'unexpected'}
          />
          {leaders.length === 0 ? (
            <div className="flex items-center gap-2 text-sm text-critical">
              <AlertTriangle size={15} /> No leader elected
            </div>
          ) : (
            leaders.map((l, i) => (
              <div key={i} className="flex items-center gap-2 text-sm text-ink">
                <Crown size={15} className="text-warning" />
                {l.labels.instance ?? 'scheduler'}
              </div>
            ))
          )}
        </Card>
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader title="Run outcomes" subtitle="cumulative, all time" />
          <StatusDistributionBar
            counts={{
              succeeded: runOutcomes.succeeded ?? 0,
              failed: runOutcomes.failed ?? 0,
              dead: runOutcomes.dead ?? 0,
            }}
          />
        </Card>
        <Card>
          <CardHeader title="Runs leased per worker" subtitle="cumulative" />
          {Object.keys(leasedByWorker).length === 0 ? (
            <div className="text-sm text-ink-muted">No data yet</div>
          ) : (
            <div className="space-y-2">
              {Object.entries(leasedByWorker)
                .sort((a, b) => b[1] - a[1])
                .map(([workerId, count]) => {
                  const max = Math.max(...Object.values(leasedByWorker))
                  return (
                    <div key={workerId} className="flex items-center gap-3 text-sm">
                      <span className="w-32 truncate text-ink-secondary">{workerId}</span>
                      <div className="h-2 flex-1 overflow-hidden rounded-full bg-border">
                        <div
                          className="h-full rounded-full"
                          style={{
                            width: `${(count / max) * 100}%`,
                            backgroundColor: colors.series[0],
                          }}
                        />
                      </div>
                      <span className="w-10 text-right tabular-nums text-ink-muted">{count}</span>
                    </div>
                  )
                })}
            </div>
          )}
        </Card>
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader title="HTTP request latency" subtitle="all routes, distribution" />
          <HistogramChart buckets={httpBuckets} color={colors.series[0]} />
        </Card>
        <Card>
          <CardHeader title="Run duration" subtitle="worker execution time, distribution" />
          <HistogramChart buckets={runDurationBuckets} color={colors.series[1]} />
        </Card>
      </div>

      <Card padded={false}>
        <div className="p-5 pb-0">
          <CardHeader title="Recent jobs" action={<Link to="/jobs" className="text-xs font-medium text-series-1">View all</Link>} />
        </div>
        <table className="w-full text-sm">
          <thead>
            <tr className="border-y border-border text-left text-xs text-ink-muted">
              <th className="px-5 py-2 font-medium">Name</th>
              <th className="px-5 py-2 font-medium">Status</th>
              <th className="px-5 py-2 font-medium">Priority</th>
              <th className="px-5 py-2 font-medium">Updated</th>
            </tr>
          </thead>
          <tbody>
            {recentJobs.length === 0 && (
              <tr>
                <td colSpan={4} className="px-5 py-6 text-center text-ink-muted">
                  No jobs yet
                </td>
              </tr>
            )}
            {recentJobs.map((job) => (
              <tr key={job.id} className="border-b border-border last:border-0 hover:bg-surface">
                <td className="px-5 py-2.5">
                  <Link to={`/jobs/${job.id}`} className="font-medium text-ink hover:text-series-1">
                    {job.name}
                  </Link>
                </td>
                <td className="px-5 py-2.5">
                  <JobStatusBadge status={job.status} />
                </td>
                <td className="px-5 py-2.5 tabular-nums text-ink-secondary">{job.priority}</td>
                <td className="px-5 py-2.5 text-ink-secondary">{relativeTime(job.updated_at)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </Card>
    </div>
  )
}
