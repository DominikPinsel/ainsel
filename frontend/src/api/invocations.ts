import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { request } from './client'
import type { Paginated } from './types'

export type InvocationStatus = 'running' | 'success' | 'failure' | 'timeout'

// Presentation variant for an invocation status, keyed off the backend contract
// (running / success / failure / timeout). Single source of truth shared by the
// Routing and Event Detail pages so their status styling cannot drift apart.
export type InvocationStatusVariant = 'ok' | 'err' | 'warn' | 'stale' | 'default'

export function invocationStatusVariant(s: string): InvocationStatusVariant {
  if (s === 'success') return 'ok'
  if (s === 'failure') return 'err'
  if (s === 'timeout') return 'stale'
  if (s === 'running') return 'warn'
  return 'default'
}

export type InvocationEntry = {
  id: string
  agent: string
  agentName?: string
  trigger?: string
  triggerName?: string
  timestamp: string
  durationMs?: number
  status: InvocationStatus
  error?: string
}

export type ListInvocationsParams = {
  agent?: string
  status?: InvocationStatus
  event?: string
  since?: string
  pageSize?: number
}

// Backend uses `invocations` as the list key (not `items`) and adds `capacity`.
// Map to the shared Paginated<T> shape consumers expect.
type InvocationsEnvelope = {
  invocations: InvocationEntry[]
  total: number
  capacity: number
  page: number
  pageSize: number
  totalPages: number
}

export async function listInvocations(
  params: ListInvocationsParams = {},
): Promise<Paginated<InvocationEntry>> {
  const env = await request<InvocationsEnvelope>('/invocations', { query: params })
  return {
    items: env.invocations ?? [],
    total: env.total ?? 0,
    page: env.page ?? 1,
    pageSize: env.pageSize ?? 0,
    totalPages: env.totalPages ?? 0,
  }
}

export function useInvocations(
  params: ListInvocationsParams,
  opts: { refetchInterval?: number | false } = {},
) {
  return useQuery({
    queryKey: ['invocations', params],
    queryFn: () => listInvocations(params),
    placeholderData: keepPreviousData,
    refetchInterval: opts.refetchInterval ?? false,
  })
}
