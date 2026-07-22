import { useState, type FormEvent } from 'react'
import { CheckCircle2, XCircle } from 'lucide-react'
import { Card, CardHeader } from '@/components/ui/Card'
import { Button } from '@/components/ui/Button'
import { useConnection } from '@/lib/connection'
import { usePromHealthy } from '@/lib/observability'

const FIELD_CLASS =
  'w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm text-ink outline-none focus:border-series-1'
const LABEL_CLASS = 'mb-1 block text-xs font-medium text-ink-secondary'

export function Settings() {
  const { apiBaseUrl, token, prometheusUrl, setConnection, setPrometheusUrl, disconnect } = useConnection()
  const [editedBaseUrl, setEditedBaseUrl] = useState(apiBaseUrl)
  const [editedToken, setEditedToken] = useState(token)
  const [editedPromUrl, setEditedPromUrl] = useState(prometheusUrl)
  const { data: promHealthy, isFetching: promChecking } = usePromHealthy()

  function handleApiSubmit(e: FormEvent) {
    e.preventDefault()
    setConnection(editedToken, editedBaseUrl.replace(/\/$/, ''))
  }

  function handlePromSubmit(e: FormEvent) {
    e.preventDefault()
    setPrometheusUrl(editedPromUrl.replace(/\/$/, ''))
  }

  return (
    <div className="max-w-xl space-y-4">
      <Card>
        <CardHeader title="API connection" subtitle="Bearer token used for /v1/* requests" />
        <form onSubmit={handleApiSubmit} className="space-y-4">
          <div>
            <label className={LABEL_CLASS}>API base URL</label>
            <input
              value={editedBaseUrl}
              onChange={(e) => setEditedBaseUrl(e.target.value)}
              className={FIELD_CLASS}
            />
          </div>
          <div>
            <label className={LABEL_CLASS}>Bearer token</label>
            <textarea
              value={editedToken}
              onChange={(e) => setEditedToken(e.target.value)}
              rows={3}
              className={`${FIELD_CLASS} resize-none font-mono text-xs`}
            />
          </div>
          <div className="flex justify-between">
            <Button type="button" variant="danger" onClick={disconnect}>
              Disconnect
            </Button>
            <Button type="submit" variant="primary">
              Save
            </Button>
          </div>
        </form>
      </Card>

      <Card>
        <CardHeader
          title="Prometheus"
          subtitle="Optional — powers queue depth, leader status, latency & throughput panels on Overview"
        />
        <div className="mb-4 flex items-center gap-2 text-sm">
          {promChecking ? (
            <span className="text-ink-muted">Checking…</span>
          ) : promHealthy ? (
            <span className="flex items-center gap-1.5 text-good">
              <CheckCircle2 size={15} /> Connected
            </span>
          ) : (
            <span className="flex items-center gap-1.5 text-critical">
              <XCircle size={15} /> Unreachable
            </span>
          )}
        </div>
        <form onSubmit={handlePromSubmit} className="space-y-4">
          <div>
            <label className={LABEL_CLASS}>Prometheus URL</label>
            <input
              value={editedPromUrl}
              onChange={(e) => setEditedPromUrl(e.target.value)}
              className={FIELD_CLASS}
              placeholder="http://localhost:9093"
            />
            <p className="mt-1.5 text-xs text-ink-muted">
              Maps to the <code className="rounded bg-surface px-1 py-0.5">prometheus</code>{' '}
              service in docker-compose.yml (port 9093). Requires{' '}
              <code className="rounded bg-surface px-1 py-0.5">--web.cors.origin</code> to be set,
              which is already configured for local dev.
            </p>
          </div>
          <div className="flex justify-end">
            <Button type="submit" variant="primary">
              Save
            </Button>
          </div>
        </form>
      </Card>
    </div>
  )
}
