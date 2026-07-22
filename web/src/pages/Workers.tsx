import { Loader2 } from 'lucide-react'
import { Card } from '@/components/ui/Card'
import { WorkerStatusBadge } from '@/components/StatusBadge'
import { useWorkers } from '@/lib/queries'
import { formatDateTime, relativeTime } from '@/lib/format'

export function Workers() {
  const { data: workers, isLoading } = useWorkers()

  return (
    <Card padded={false}>
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-border text-left text-xs text-ink-muted">
            <th className="px-5 py-3 font-medium">Hostname</th>
            <th className="px-5 py-3 font-medium">Status</th>
            <th className="px-5 py-3 font-medium">Last heartbeat</th>
            <th className="px-5 py-3 font-medium">Started</th>
          </tr>
        </thead>
        <tbody>
          {isLoading && (
            <tr>
              <td colSpan={4} className="px-5 py-8 text-center text-ink-muted">
                <Loader2 size={16} className="mx-auto animate-spin" />
              </td>
            </tr>
          )}
          {!isLoading && (workers ?? []).length === 0 && (
            <tr>
              <td colSpan={4} className="px-5 py-8 text-center text-ink-muted">
                No workers registered
              </td>
            </tr>
          )}
          {(workers ?? []).map((worker) => (
            <tr key={worker.id} className="border-b border-border last:border-0 hover:bg-surface">
              <td className="px-5 py-3 font-mono text-xs text-ink">{worker.hostname}</td>
              <td className="px-5 py-3">
                <WorkerStatusBadge status={worker.status} />
              </td>
              <td className="px-5 py-3 text-ink-secondary">{relativeTime(worker.last_heartbeat)}</td>
              <td className="px-5 py-3 text-ink-secondary">{formatDateTime(worker.started_at)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </Card>
  )
}
