import type { Job, JobRun, NewJobInput, Worker } from '@/types'
import { notifyUnauthorized } from '@/lib/authEvents'

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

interface RequestOptions {
  method?: string
  body?: unknown
}

export class ApiClient {
  private baseUrl: string
  private token: string

  constructor(baseUrl: string, token: string) {
    this.baseUrl = baseUrl
    this.token = token
  }

  private async request<T>(path: string, opts: RequestOptions = {}): Promise<T> {
    const res = await fetch(`${this.baseUrl}${path}`, {
      method: opts.method ?? 'GET',
      headers: {
        ...(this.token ? { Authorization: `Bearer ${this.token}` } : {}),
        ...(opts.body ? { 'Content-Type': 'application/json' } : {}),
      },
      body: opts.body ? JSON.stringify(opts.body) : undefined,
    })

    if (!res.ok) {
      let message = res.statusText
      try {
        const body = (await res.json()) as { error?: string }
        if (body.error) message = body.error
      } catch {
        // response wasn't JSON — fall back to statusText
      }
      if (res.status === 401) notifyUnauthorized()
      throw new ApiError(res.status, message)
    }

    if (res.status === 204) return undefined as T
    return (await res.json()) as T
  }

  healthz() {
    return this.request<{ status: string }>('/healthz')
  }

  listJobs(params: { status?: string; limit?: number; offset?: number } = {}) {
    const qs = new URLSearchParams()
    if (params.status) qs.set('status', params.status)
    if (params.limit) qs.set('limit', String(params.limit))
    if (params.offset) qs.set('offset', String(params.offset))
    const suffix = qs.toString() ? `?${qs.toString()}` : ''
    return this.request<Job[]>(`/v1/jobs${suffix}`)
  }

  getJob(id: string) {
    return this.request<Job>(`/v1/jobs/${id}`)
  }

  createJob(input: NewJobInput) {
    return this.request<Job>('/v1/jobs', { method: 'POST', body: input })
  }

  pauseJob(id: string) {
    return this.request<Job>(`/v1/jobs/${id}/pause`, { method: 'POST' })
  }

  resumeJob(id: string) {
    return this.request<Job>(`/v1/jobs/${id}/resume`, { method: 'POST' })
  }

  listJobRuns(jobId: string, limit = 50) {
    return this.request<JobRun[]>(`/v1/jobs/${jobId}/runs?limit=${limit}`)
  }

  getRun(id: string) {
    return this.request<JobRun>(`/v1/runs/${id}`)
  }

  listWorkers() {
    return this.request<Worker[]>('/v1/workers')
  }
}
