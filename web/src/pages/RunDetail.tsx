import type { ReactNode } from 'react'
import { Link, useParams } from 'react-router-dom'
import { Loader2 } from 'lucide-react'
import { Card } from '@/components/ui/Card'
import { RunStatusBadge } from '@/components/StatusBadge'
import { useRun } from '@/lib/queries'
import { formatDateTime, formatDurationMs, runDurationMs } from '@/lib/format'

export function RunDetail() {
  const { id } = useParams<{ id: string }>()
  const { data: run, isLoading } = useRun(id)

  if (isLoading) {
    return <Loader2 size={20} className="mx-auto mt-12 animate-spin text-ink-muted" />
  }
  if (!run) {
    return <div className="text-sm text-ink-muted">Run not found.</div>
  }

  const duration = runDurationMs(run)

  return (
    <div className="space-y-4">
      <Card>
        <div className="flex items-center gap-2.5">
          <RunStatusBadge status={run.status} />
          <span className="font-mono text-xs text-ink-muted">{run.id}</span>
        </div>

        <div className="mt-5 grid grid-cols-2 gap-4 border-t border-border pt-4 text-sm sm:grid-cols-3">
          <Field label="Job" value={<Link to={`/jobs/${run.job_id}`} className="font-mono text-xs text-series-1">{run.job_id}</Link>} />
          <Field label="Attempt" value={String(run.attempt)} />
          <Field label="Priority" value={String(run.priority)} />
          <Field label="Scheduled at" value={formatDateTime(run.scheduled_at)} />
          <Field label="Started at" value={formatDateTime(run.started_at)} />
          <Field label="Finished at" value={formatDateTime(run.finished_at)} />
          <Field label="Duration" value={duration !== null ? formatDurationMs(duration) : '—'} />
          <Field label="Leased by" value={run.leased_by ?? '—'} mono />
          <Field label="Lease expires" value={formatDateTime(run.lease_expires_at)} />
        </div>

        {run.error && (
          <div className="mt-4 border-t border-border pt-4">
            <span className="mb-1.5 block text-xs font-medium text-ink-muted">Error</span>
            <pre className="overflow-x-auto rounded-lg bg-critical/10 p-3 font-mono text-xs text-critical">
              {run.error}
            </pre>
          </div>
        )}

        {run.result && (
          <div className="mt-4 border-t border-border pt-4">
            <span className="mb-1.5 block text-xs font-medium text-ink-muted">Result</span>
            <pre className="overflow-x-auto rounded-lg bg-surface p-3 font-mono text-xs text-ink-secondary">
              {JSON.stringify(run.result, null, 2)}
            </pre>
          </div>
        )}
      </Card>
    </div>
  )
}

function Field({ label, value, mono }: { label: string; value: ReactNode; mono?: boolean }) {
  return (
    <div>
      <span className="block text-xs text-ink-muted">{label}</span>
      <span className={mono ? 'font-mono text-xs text-ink' : 'text-ink'}>{value}</span>
    </div>
  )
}
