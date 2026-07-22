import { useMemo } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiClient } from '@/lib/api'
import { useConnection } from '@/lib/connection'
import type { NewJobInput } from '@/types'

export function useApiClient(): ApiClient {
  const { apiBaseUrl, token } = useConnection()
  return useMemo(() => new ApiClient(apiBaseUrl, token), [apiBaseUrl, token])
}

export function useHealthz() {
  const api = useApiClient()
  const { isConfigured } = useConnection()
  return useQuery({
    queryKey: ['healthz'],
    queryFn: () => api.healthz(),
    enabled: isConfigured,
    retry: false,
    refetchInterval: 15_000,
  })
}

export function useJobs(params: { status?: string; limit?: number; offset?: number } = {}) {
  const api = useApiClient()
  const { isConfigured } = useConnection()
  return useQuery({
    queryKey: ['jobs', params],
    queryFn: () => api.listJobs(params),
    enabled: isConfigured,
    refetchInterval: 5_000,
  })
}

export function useJob(id: string | undefined) {
  const api = useApiClient()
  const { isConfigured } = useConnection()
  return useQuery({
    queryKey: ['job', id],
    queryFn: () => api.getJob(id!),
    enabled: isConfigured && !!id,
    refetchInterval: 5_000,
  })
}

export function useJobRuns(jobId: string | undefined) {
  const api = useApiClient()
  const { isConfigured } = useConnection()
  return useQuery({
    queryKey: ['job-runs', jobId],
    queryFn: () => api.listJobRuns(jobId!),
    enabled: isConfigured && !!jobId,
    refetchInterval: 5_000,
  })
}

export function useRun(id: string | undefined) {
  const api = useApiClient()
  const { isConfigured } = useConnection()
  return useQuery({
    queryKey: ['run', id],
    queryFn: () => api.getRun(id!),
    enabled: isConfigured && !!id,
    refetchInterval: 5_000,
  })
}

export function useWorkers() {
  const api = useApiClient()
  const { isConfigured } = useConnection()
  return useQuery({
    queryKey: ['workers'],
    queryFn: () => api.listWorkers(),
    enabled: isConfigured,
    refetchInterval: 5_000,
  })
}

export function useCreateJob() {
  const api = useApiClient()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: NewJobInput) => api.createJob(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['jobs'] }),
  })
}

export function usePauseJob() {
  const api = useApiClient()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => api.pauseJob(id),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: ['jobs'] })
      qc.invalidateQueries({ queryKey: ['job', id] })
    },
  })
}

export function useResumeJob() {
  const api = useApiClient()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => api.resumeJob(id),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: ['jobs'] })
      qc.invalidateQueries({ queryKey: ['job', id] })
    },
  })
}
