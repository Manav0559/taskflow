import { useState } from 'react'
import { Link } from 'react-router-dom'
import { ChevronLeft, ChevronRight, Loader2, Pause, Play, Plus } from 'lucide-react'
import { Card } from '@/components/ui/Card'
import { Button } from '@/components/ui/Button'
import { JobStatusBadge } from '@/components/StatusBadge'
import { CreateJobModal } from '@/components/CreateJobModal'
import { useJobs, usePauseJob, useResumeJob } from '@/lib/queries'
import { relativeTime } from '@/lib/format'
import type { JobStatus } from '@/types'

const PAGE_SIZE = 20
const STATUS_FILTERS: { label: string; value: JobStatus | undefined }[] = [
  { label: 'All', value: undefined },
  { label: 'Active', value: 'active' },
  { label: 'Paused', value: 'paused' },
  { label: 'Archived', value: 'archived' },
]

export function Jobs() {
  const [status, setStatus] = useState<JobStatus | undefined>(undefined)
  const [offset, setOffset] = useState(0)
  const [showCreate, setShowCreate] = useState(false)
  const { data: jobs, isLoading } = useJobs({ status, limit: PAGE_SIZE, offset })
  const pauseJob = usePauseJob()
  const resumeJob = useResumeJob()

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div className="flex gap-1 rounded-lg border border-border bg-card p-1">
          {STATUS_FILTERS.map((f) => (
            <button
              key={f.label}
              onClick={() => {
                setStatus(f.value)
                setOffset(0)
              }}
              className={`rounded-md px-3 py-1.5 text-xs font-medium transition-colors ${
                status === f.value ? 'bg-series-1 text-white' : 'text-ink-secondary hover:bg-surface'
              }`}
            >
              {f.label}
            </button>
          ))}
        </div>
        <Button variant="primary" onClick={() => setShowCreate(true)}>
          <Plus size={14} /> New job
        </Button>
      </div>

      <Card padded={false}>
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border text-left text-xs text-ink-muted">
              <th className="px-5 py-3 font-medium">Name</th>
              <th className="px-5 py-3 font-medium">Status</th>
              <th className="px-5 py-3 font-medium">Schedule</th>
              <th className="px-5 py-3 font-medium">Priority</th>
              <th className="px-5 py-3 font-medium">Updated</th>
              <th className="px-5 py-3 font-medium" />
            </tr>
          </thead>
          <tbody>
            {isLoading && (
              <tr>
                <td colSpan={6} className="px-5 py-8 text-center text-ink-muted">
                  <Loader2 size={16} className="mx-auto animate-spin" />
                </td>
              </tr>
            )}
            {!isLoading && (jobs ?? []).length === 0 && (
              <tr>
                <td colSpan={6} className="px-5 py-8 text-center text-ink-muted">
                  No jobs found
                </td>
              </tr>
            )}
            {(jobs ?? []).map((job) => (
              <tr key={job.id} className="border-b border-border last:border-0 hover:bg-surface">
                <td className="px-5 py-3">
                  <Link to={`/jobs/${job.id}`} className="font-medium text-ink hover:text-series-1">
                    {job.name}
                  </Link>
                </td>
                <td className="px-5 py-3">
                  <JobStatusBadge status={job.status} />
                </td>
                <td className="px-5 py-3 font-mono text-xs text-ink-secondary">
                  {job.cron_expr ?? 'one-off'}
                </td>
                <td className="px-5 py-3 tabular-nums text-ink-secondary">{job.priority}</td>
                <td className="px-5 py-3 text-ink-secondary">{relativeTime(job.updated_at)}</td>
                <td className="px-5 py-3 text-right">
                  {job.status === 'active' ? (
                    <Button
                      variant="ghost"
                      onClick={() => pauseJob.mutate(job.id)}
                      disabled={pauseJob.isPending}
                    >
                      <Pause size={13} /> Pause
                    </Button>
                  ) : job.status === 'paused' ? (
                    <Button
                      variant="ghost"
                      onClick={() => resumeJob.mutate(job.id)}
                      disabled={resumeJob.isPending}
                    >
                      <Play size={13} /> Resume
                    </Button>
                  ) : null}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        <div className="flex items-center justify-between border-t border-border px-5 py-3">
          <span className="text-xs text-ink-muted">
            Showing {offset + 1}–{offset + (jobs?.length ?? 0)}
          </span>
          <div className="flex gap-2">
            <Button variant="secondary" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}>
              <ChevronLeft size={14} /> Prev
            </Button>
            <Button
              variant="secondary"
              disabled={(jobs?.length ?? 0) < PAGE_SIZE}
              onClick={() => setOffset(offset + PAGE_SIZE)}
            >
              Next <ChevronRight size={14} />
            </Button>
          </div>
        </div>
      </Card>

      {showCreate && <CreateJobModal onClose={() => setShowCreate(false)} />}
    </div>
  )
}
