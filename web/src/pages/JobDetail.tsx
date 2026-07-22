import { Link, useParams } from 'react-router-dom'
import { Loader2, Pause, Play } from 'lucide-react'
import { Card, CardHeader } from '@/components/ui/Card'
import { Button } from '@/components/ui/Button'
import { JobStatusBadge, RunStatusBadge } from '@/components/StatusBadge'
import { useJob, useJobRuns, usePauseJob, useResumeJob } from '@/lib/queries'
import { formatDateTime, formatDurationMs, relativeTime, runDurationMs } from '@/lib/format'

export function JobDetail() {
  const { id } = useParams<{ id: string }>()
  const { data: job, isLoading } = useJob(id)
  const { data: runs, isLoading: runsLoading } = useJobRuns(id)
  const pauseJob = usePauseJob()
  const resumeJob = useResumeJob()

  if (isLoading) {
    return <Loader2 size={20} className="mx-auto mt-12 animate-spin text-ink-muted" />
  }
  if (!job) {
    return <div className="text-sm text-ink-muted">Job not found.</div>
  }

  return (
    <div className="space-y-4">
      <Card>
        <div className="flex items-start justify-between">
          <div>
            <div className="flex items-center gap-2.5">
              <h2 className="text-lg font-semibold text-ink">{job.name}</h2>
              <JobStatusBadge status={job.status} />
            </div>
            <p className="mt-1 font-mono text-xs text-ink-muted">{job.id}</p>
          </div>
          {job.status === 'active' ? (
            <Button variant="secondary" onClick={() => pauseJob.mutate(job.id)} disabled={pauseJob.isPending}>
              <Pause size={13} /> Pause
            </Button>
          ) : job.status === 'paused' ? (
            <Button variant="primary" onClick={() => resumeJob.mutate(job.id)} disabled={resumeJob.isPending}>
              <Play size={13} /> Resume
            </Button>
          ) : null}
        </div>

        <div className="mt-5 grid grid-cols-2 gap-4 border-t border-border pt-4 text-sm sm:grid-cols-4">
          <Field label="Schedule" value={job.cron_expr ?? 'one-off'} mono />
          <Field label="Priority" value={String(job.priority)} />
          <Field label="Max attempts" value={String(job.max_attempts)} />
          <Field label="Timeout" value={`${job.timeout_seconds}s`} />
          <Field label="Created" value={formatDateTime(job.created_at)} />
          <Field label="Updated" value={formatDateTime(job.updated_at)} />
          {job.idempotency_key && <Field label="Idempotency key" value={job.idempotency_key} mono />}
          {job.depends_on && job.depends_on.length > 0 && (
            <Field label="Depends on" value={`${job.depends_on.length} job(s)`} />
          )}
        </div>

        <div className="mt-4 border-t border-border pt-4">
          <span className="mb-1.5 block text-xs font-medium text-ink-muted">Payload</span>
          <pre className="overflow-x-auto rounded-lg bg-surface p-3 font-mono text-xs text-ink-secondary">
            {JSON.stringify(job.payload, null, 2)}
          </pre>
        </div>
      </Card>

      <Card padded={false}>
        <div className="p-5 pb-0">
          <CardHeader title="Runs" />
        </div>
        <table className="w-full text-sm">
          <thead>
            <tr className="border-y border-border text-left text-xs text-ink-muted">
              <th className="px-5 py-2 font-medium">Status</th>
              <th className="px-5 py-2 font-medium">Attempt</th>
              <th className="px-5 py-2 font-medium">Scheduled</th>
              <th className="px-5 py-2 font-medium">Duration</th>
              <th className="px-5 py-2 font-medium">Leased by</th>
              <th className="px-5 py-2 font-medium">Error</th>
            </tr>
          </thead>
          <tbody>
            {runsLoading && (
              <tr>
                <td colSpan={6} className="px-5 py-6 text-center text-ink-muted">
                  <Loader2 size={16} className="mx-auto animate-spin" />
                </td>
              </tr>
            )}
            {!runsLoading && (runs ?? []).length === 0 && (
              <tr>
                <td colSpan={6} className="px-5 py-6 text-center text-ink-muted">
                  No runs yet
                </td>
              </tr>
            )}
            {(runs ?? []).map((run) => {
              const duration = runDurationMs(run)
              return (
                <tr key={run.id} className="border-b border-border last:border-0 hover:bg-surface">
                  <td className="px-5 py-2.5">
                    <Link to={`/runs/${run.id}`} className="hover:opacity-80">
                      <RunStatusBadge status={run.status} />
                    </Link>
                  </td>
                  <td className="px-5 py-2.5 tabular-nums text-ink-secondary">{run.attempt}</td>
                  <td className="px-5 py-2.5 text-ink-secondary">{relativeTime(run.scheduled_at)}</td>
                  <td className="px-5 py-2.5 tabular-nums text-ink-secondary">
                    {duration !== null ? formatDurationMs(duration) : '—'}
                  </td>
                  <td className="px-5 py-2.5 font-mono text-xs text-ink-secondary">
                    {run.leased_by ?? '—'}
                  </td>
                  <td className="max-w-xs truncate px-5 py-2.5 text-critical">{run.error ?? ''}</td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </Card>
    </div>
  )
}

function Field({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div>
      <span className="block text-xs text-ink-muted">{label}</span>
      <span className={mono ? 'font-mono text-xs text-ink' : 'text-ink'}>{value}</span>
    </div>
  )
}
