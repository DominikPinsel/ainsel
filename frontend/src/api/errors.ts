import { useQuery } from '@tanstack/react-query'
import { request } from './client'

export type ErrorSeverity = 'error' | 'warning'
export type ErrorSource = 'router' | 'connector' | 'api' | 'agent' | 'hub' | 'gateway'

export type PlatformError = {
  id: string
  timestamp: string
  severity: ErrorSeverity
  source: ErrorSource
  message: string
  details?: Record<string, unknown>
}

export type ListErrorsParams = {
  limit?: number
  severity?: ErrorSeverity
  source?: ErrorSource
  since?: string
}

// Backend wraps the list in `{ errors, total }`; unwrap so consumers can
// keep treating the result as an array.
type ErrorsEnvelope = {
  errors: PlatformError[]
  total: number
}

export async function listErrors(params: ListErrorsParams = {}): Promise<PlatformError[]> {
  const env = await request<ErrorsEnvelope>('/errors', { query: params })
  return env.errors ?? []
}

export function useErrors(params: ListErrorsParams = {}) {
  return useQuery({
    queryKey: ['errors', params],
    queryFn: () => listErrors(params),
  })
}
