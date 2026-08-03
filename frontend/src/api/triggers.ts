import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { request } from './client'
import type { Paginated } from './types'

export type TriggerFilter = {
  field: string
  op: string
  value: string
  values?: string[]
}

export type TriggerSummary = {
  id: string
  name: string
  agentRef?: string
  connectorRef?: string
  filters?: TriggerFilter[]
  status?: { agentValid: boolean; connectorValid: boolean }
}

export type TriggerResponse = TriggerSummary

export type TriggerRequest = Omit<TriggerResponse, 'id' | 'status'> & { groupId?: string }

export type ListTriggersParams = {
  page?: number
  pageSize?: number
  agent?: string
  connector?: string
}

export function listTriggers(params: ListTriggersParams = {}) {
  return request<Paginated<TriggerSummary>>('/triggers', { query: params })
}

export function getTrigger(id: string) {
  return request<TriggerResponse>(`/triggers/${encodeURIComponent(id)}`)
}

export function createTrigger(body: TriggerRequest) {
  return request<TriggerResponse>('/triggers', { method: 'POST', body })
}

export function updateTrigger(id: string, body: TriggerRequest) {
  return request<TriggerResponse>(`/triggers/${encodeURIComponent(id)}`, {
    method: 'PUT',
    body,
  })
}

export function deleteTrigger(id: string) {
  return request<void>(`/triggers/${encodeURIComponent(id)}`, { method: 'DELETE' })
}

export function useTriggers(params: ListTriggersParams = {}) {
  return useQuery({
    queryKey: ['triggers', params],
    queryFn: () => listTriggers(params),
    placeholderData: keepPreviousData,
  })
}

export function useTrigger(id: string | undefined) {
  return useQuery({
    queryKey: ['triggers', 'detail', id],
    queryFn: () => getTrigger(id!),
    enabled: id !== undefined,
  })
}

export function useCreateTrigger() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: createTrigger,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['triggers'] }),
  })
}

export function useUpdateTrigger() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: TriggerRequest }) =>
      updateTrigger(id, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['triggers'] }),
  })
}

export function useDeleteTrigger() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: deleteTrigger,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['triggers'] }),
  })
}
