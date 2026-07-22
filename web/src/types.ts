export type JobStatus = 'active' | 'paused' | 'archived'

export type RunStatus = 'pending' | 'leased' | 'running' | 'succeeded' | 'failed' | 'dead'

export type WorkerStatus = 'alive' | 'dead'

export interface Job {
  id: string
  name: string
  payload: Record<string, unknown>
  cron_expr?: string | null
  priority: number
  max_attempts: number
  timeout_seconds: number
  status: JobStatus
  idempotency_key?: string | null
  depends_on?: string[] | null
  created_at: string
  updated_at: string
}

export interface JobRun {
  id: string
  job_id: string
  status: RunStatus
  attempt: number
  priority: number
  scheduled_at: string
  leased_by?: string | null
  leased_at?: string | null
  lease_expires_at?: string | null
  started_at?: string | null
  finished_at?: string | null
  result?: Record<string, unknown> | null
  error?: string | null
  created_at: string
}

export interface Worker {
  id: string
  hostname: string
  status: WorkerStatus
  last_heartbeat: string
  started_at: string
}

export interface NewJobInput {
  name: string
  payload: Record<string, unknown>
  cron_expr?: string
  priority?: number
  max_attempts?: number
  timeout_seconds?: number
  idempotency_key?: string
  depends_on?: string[]
}

export interface ApiErrorBody {
  error: string
}
