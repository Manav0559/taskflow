import type { JobStatus, RunStatus, WorkerStatus } from '@/types'

interface StatusStyle {
  label: string
  color: string
}

const RUN_STYLES: Record<RunStatus, StatusStyle> = {
  pending: { label: 'Pending', color: 'var(--color-ink-muted)' },
  leased: { label: 'Leased', color: 'var(--color-series-1)' },
  running: { label: 'Running', color: 'var(--color-warning)' },
  succeeded: { label: 'Succeeded', color: 'var(--color-good)' },
  failed: { label: 'Failed', color: 'var(--color-serious)' },
  dead: { label: 'Dead', color: 'var(--color-critical)' },
}

const JOB_STYLES: Record<JobStatus, StatusStyle> = {
  active: { label: 'Active', color: 'var(--color-good)' },
  paused: { label: 'Paused', color: 'var(--color-warning)' },
  archived: { label: 'Archived', color: 'var(--color-ink-muted)' },
}

const WORKER_STYLES: Record<WorkerStatus, StatusStyle> = {
  alive: { label: 'Alive', color: 'var(--color-good)' },
  dead: { label: 'Dead', color: 'var(--color-critical)' },
}

function Badge({ style }: { style: StatusStyle }) {
  return (
    <span className="inline-flex items-center gap-1.5 rounded-full border border-border px-2 py-0.5 text-xs font-medium text-ink-secondary">
      <span className="h-1.5 w-1.5 rounded-full" style={{ backgroundColor: style.color }} />
      {style.label}
    </span>
  )
}

export function RunStatusBadge({ status }: { status: RunStatus }) {
  return <Badge style={RUN_STYLES[status]} />
}

export function JobStatusBadge({ status }: { status: JobStatus }) {
  return <Badge style={JOB_STYLES[status]} />
}

export function WorkerStatusBadge({ status }: { status: WorkerStatus }) {
  return <Badge style={WORKER_STYLES[status]} />
}

export function runStatusColor(status: RunStatus): string {
  return RUN_STYLES[status].color
}
