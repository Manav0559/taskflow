import { useState, type FormEvent } from 'react'
import { Workflow, AlertCircle, Loader2 } from 'lucide-react'
import { ApiClient } from '@/lib/api'
import { useConnection } from '@/lib/connection'
import { Button } from '@/components/ui/Button'

export function ConnectScreen() {
  const { apiBaseUrl: savedUrl, setConnection } = useConnection()
  const [apiBaseUrl, setApiBaseUrl] = useState(savedUrl)
  const [token, setToken] = useState('')
  const [status, setStatus] = useState<'idle' | 'checking' | 'error'>('idle')
  const [error, setError] = useState('')

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setStatus('checking')
    setError('')
    try {
      const client = new ApiClient(apiBaseUrl.replace(/\/$/, ''), token)
      await client.healthz()
      setConnection(token, apiBaseUrl.replace(/\/$/, ''))
    } catch {
      setStatus('error')
      setError('Could not reach the API at that URL. Check the address and that the server is running.')
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-surface px-4">
      <div className="w-full max-w-md rounded-2xl border border-border bg-card p-8 shadow-sm">
        <div className="mb-6 flex items-center gap-2.5">
          <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-series-1 text-white">
            <Workflow size={18} />
          </div>
          <div>
            <h1 className="text-base font-semibold text-ink">Connect to TaskFlow</h1>
            <p className="text-xs text-ink-muted">Enter your API address and a bearer token</p>
          </div>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="mb-1 block text-xs font-medium text-ink-secondary">API base URL</label>
            <input
              value={apiBaseUrl}
              onChange={(e) => setApiBaseUrl(e.target.value)}
              placeholder="http://localhost:8080"
              className="w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm text-ink outline-none focus:border-series-1"
              required
            />
          </div>
          <div>
            <label className="mb-1 block text-xs font-medium text-ink-secondary">Bearer token</label>
            <textarea
              value={token}
              onChange={(e) => setToken(e.target.value)}
              placeholder="eyJhbGciOiJIUzI1NiIs..."
              rows={3}
              className="w-full resize-none rounded-lg border border-border bg-surface px-3 py-2 font-mono text-xs text-ink outline-none focus:border-series-1"
              required
            />
            <p className="mt-1.5 text-xs text-ink-muted">
              TaskFlow has no login endpoint — mint one with{' '}
              <code className="rounded bg-surface px-1 py-0.5 text-[11px]">
                api.MintToken(secret, subject, ttl)
              </code>{' '}
              as described in the README, then paste it here.
            </p>
          </div>

          {status === 'error' && (
            <div className="flex items-start gap-2 rounded-lg bg-critical/10 px-3 py-2 text-xs text-critical">
              <AlertCircle size={14} className="mt-0.5 shrink-0" />
              {error}
            </div>
          )}

          <Button type="submit" variant="primary" className="w-full" disabled={status === 'checking'}>
            {status === 'checking' && <Loader2 size={14} className="animate-spin" />}
            Connect
          </Button>
        </form>
      </div>
    </div>
  )
}
