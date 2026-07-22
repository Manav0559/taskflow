export function relativeTime(iso: string | null | undefined): string {
  if (!iso) return '—'
  const date = new Date(iso)
  const diffMs = date.getTime() - Date.now()
  const diffSec = Math.round(diffMs / 1000)
  const abs = Math.abs(diffSec)

  const units: [Intl.RelativeTimeFormatUnit, number][] = [
    ['year', 31536000],
    ['month', 2592000],
    ['week', 604800],
    ['day', 86400],
    ['hour', 3600],
    ['minute', 60],
    ['second', 1],
  ]
  const rtf = new Intl.RelativeTimeFormat('en', { numeric: 'auto' })
  for (const [unit, secs] of units) {
    if (abs >= secs || unit === 'second') {
      return rtf.format(Math.round(diffSec / secs), unit)
    }
  }
  return rtf.format(diffSec, 'second')
}

export function formatDateTime(iso: string | null | undefined): string {
  if (!iso) return '—'
  return new Date(iso).toLocaleString(undefined, {
    dateStyle: 'medium',
    timeStyle: 'medium',
  })
}

export function formatDurationMs(ms: number): string {
  if (ms < 1000) return `${Math.round(ms)}ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(2)}s`
  const min = Math.floor(ms / 60_000)
  const sec = Math.round((ms % 60_000) / 1000)
  return `${min}m ${sec}s`
}

export function runDurationMs(run: { started_at?: string | null; finished_at?: string | null }): number | null {
  if (!run.started_at || !run.finished_at) return null
  return new Date(run.finished_at).getTime() - new Date(run.started_at).getTime()
}
