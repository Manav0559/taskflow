import { useTheme } from '@/lib/theme'
import type { RunStatus } from '@/types'

const ORDER: RunStatus[] = ['pending', 'leased', 'running', 'succeeded', 'failed', 'dead']
const LABELS: Record<RunStatus, string> = {
  pending: 'Pending',
  leased: 'Leased',
  running: 'Running',
  succeeded: 'Succeeded',
  failed: 'Failed',
  dead: 'Dead',
}

export function StatusDistributionBar({ counts }: { counts: Partial<Record<RunStatus, number>> }) {
  const { colors } = useTheme()
  const colorFor: Record<RunStatus, string> = {
    pending: colors.inkMuted,
    leased: colors.series[0],
    running: colors.warning,
    succeeded: colors.good,
    failed: colors.serious,
    dead: colors.critical,
  }

  const total = ORDER.reduce((sum, s) => sum + (counts[s] ?? 0), 0)

  if (total === 0) {
    return <div className="text-sm text-ink-muted">No runs yet</div>
  }

  const segments = ORDER.filter((s) => (counts[s] ?? 0) > 0)

  return (
    <div>
      <div className="flex h-3 w-full gap-0.5 overflow-hidden rounded-full bg-border">
        {segments.map((status) => {
          const count = counts[status] ?? 0
          const pct = (count / total) * 100
          return (
            <div
              key={status}
              className="h-full first:rounded-l-full last:rounded-r-full"
              style={{ width: `${pct}%`, backgroundColor: colorFor[status] }}
              title={`${LABELS[status]}: ${count}`}
            />
          )
        })}
      </div>
      <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1.5">
        {segments.map((status) => (
          <div key={status} className="flex items-center gap-1.5 text-xs text-ink-secondary">
            <span
              className="h-2 w-2 rounded-full"
              style={{ backgroundColor: colorFor[status] }}
            />
            {LABELS[status]}
            <span className="tabular-nums text-ink-muted">{counts[status]}</span>
          </div>
        ))}
      </div>
    </div>
  )
}
