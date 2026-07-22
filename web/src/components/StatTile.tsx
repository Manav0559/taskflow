import type { ReactNode } from 'react'
import clsx from 'clsx'
import { Card } from '@/components/ui/Card'

export function StatTile({
  label,
  value,
  hint,
  icon,
  tone = 'neutral',
}: {
  label: string
  value: ReactNode
  hint?: string
  icon?: ReactNode
  tone?: 'neutral' | 'good' | 'warning' | 'critical'
}) {
  const toneClass = {
    neutral: 'text-ink',
    good: 'text-good',
    warning: 'text-warning',
    critical: 'text-critical',
  }[tone]

  return (
    <Card>
      <div className="flex items-center justify-between">
        <span className="text-xs font-medium uppercase tracking-wide text-ink-muted">
          {label}
        </span>
        {icon && <span className="text-ink-muted">{icon}</span>}
      </div>
      <div className={clsx('mt-2 text-2xl font-semibold tabular-nums', toneClass)}>{value}</div>
      {hint && <div className="mt-1 text-xs text-ink-muted">{hint}</div>}
    </Card>
  )
}
