import { useState, type FormEvent } from 'react'
import { AlertCircle, Loader2 } from 'lucide-react'
import { Modal } from '@/components/ui/Modal'
import { Button } from '@/components/ui/Button'
import { useCreateJob } from '@/lib/queries'
import { ApiError } from '@/lib/api'

const FIELD_CLASS =
  'w-full rounded-lg border border-border bg-surface px-3 py-2 text-sm text-ink outline-none focus:border-series-1'
const LABEL_CLASS = 'mb-1 block text-xs font-medium text-ink-secondary'

export function CreateJobModal({ onClose }: { onClose: () => void }) {
  const createJob = useCreateJob()
  const [name, setName] = useState('')
  const [payload, setPayload] = useState('{\n  "hello": "world"\n}')
  const [cronExpr, setCronExpr] = useState('')
  const [priority, setPriority] = useState('0')
  const [maxAttempts, setMaxAttempts] = useState('5')
  const [timeoutSeconds, setTimeoutSeconds] = useState('300')
  const [error, setError] = useState('')

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError('')

    let parsedPayload: Record<string, unknown>
    try {
      parsedPayload = JSON.parse(payload)
    } catch {
      setError('Payload must be valid JSON.')
      return
    }

    try {
      await createJob.mutateAsync({
        name,
        payload: parsedPayload,
        cron_expr: cronExpr || undefined,
        priority: Number(priority),
        max_attempts: Number(maxAttempts),
        timeout_seconds: Number(timeoutSeconds),
      })
      onClose()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to create job.')
    }
  }

  return (
    <Modal title="New job" onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-4">
        <div>
          <label className={LABEL_CLASS}>Name</label>
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            className={FIELD_CLASS}
            placeholder="echo"
            required
          />
        </div>

        <div>
          <label className={LABEL_CLASS}>Payload (JSON)</label>
          <textarea
            value={payload}
            onChange={(e) => setPayload(e.target.value)}
            rows={4}
            className={`${FIELD_CLASS} resize-none font-mono text-xs`}
          />
        </div>

        <div>
          <label className={LABEL_CLASS}>Cron expression (optional — omit for a one-off run)</label>
          <input
            value={cronExpr}
            onChange={(e) => setCronExpr(e.target.value)}
            className={FIELD_CLASS}
            placeholder="*/5 * * * *"
          />
        </div>

        <div className="grid grid-cols-3 gap-3">
          <div>
            <label className={LABEL_CLASS}>Priority</label>
            <input
              type="number"
              value={priority}
              onChange={(e) => setPriority(e.target.value)}
              className={FIELD_CLASS}
            />
          </div>
          <div>
            <label className={LABEL_CLASS}>Max attempts</label>
            <input
              type="number"
              min={1}
              value={maxAttempts}
              onChange={(e) => setMaxAttempts(e.target.value)}
              className={FIELD_CLASS}
            />
          </div>
          <div>
            <label className={LABEL_CLASS}>Timeout (s)</label>
            <input
              type="number"
              min={1}
              value={timeoutSeconds}
              onChange={(e) => setTimeoutSeconds(e.target.value)}
              className={FIELD_CLASS}
            />
          </div>
        </div>

        {error && (
          <div className="flex items-start gap-2 rounded-lg bg-critical/10 px-3 py-2 text-xs text-critical">
            <AlertCircle size={14} className="mt-0.5 shrink-0" />
            {error}
          </div>
        )}

        <div className="flex justify-end gap-2 pt-2">
          <Button type="button" variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" variant="primary" disabled={createJob.isPending}>
            {createJob.isPending && <Loader2 size={14} className="animate-spin" />}
            Create job
          </Button>
        </div>
      </form>
    </Modal>
  )
}
