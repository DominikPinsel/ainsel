import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { request } from './client'
import type { Paginated } from './types'

export type CronTriggerStatus = {
  agentValid: boolean
  scheduleValid: boolean
  ready: boolean
  lastRun?: string
  nextRun?: string
}

export type CronTriggerSummary = {
  id: string
  name: string
  agentRef: string
  schedule: string
  prompt: string
  enabled: boolean
  status?: CronTriggerStatus
}

export type CronTriggerResponse = CronTriggerSummary

export type CronTriggerRequest = {
  name: string
  groupId?: string
  agentRef: string
  schedule: string
  prompt: string
  enabled?: boolean
}

export type CronTriggerUpdateRequest = {
  name?: string
  agentRef?: string
  schedule?: string
  prompt?: string
  enabled?: boolean
}

export type ListCronTriggersParams = {
  page?: number
  pageSize?: number
  agent?: string
}

export function listCronTriggers(params: ListCronTriggersParams = {}) {
  return request<Paginated<CronTriggerSummary>>('/cron-triggers', { query: params })
}

export function getCronTrigger(id: string) {
  return request<CronTriggerResponse>(`/cron-triggers/${encodeURIComponent(id)}`)
}

export function createCronTrigger(body: CronTriggerRequest) {
  return request<CronTriggerResponse>('/cron-triggers', { method: 'POST', body })
}

export function updateCronTrigger(id: string, body: CronTriggerUpdateRequest) {
  return request<CronTriggerResponse>(`/cron-triggers/${encodeURIComponent(id)}`, {
    method: 'PUT',
    body,
  })
}

export function deleteCronTrigger(id: string) {
  return request<void>(`/cron-triggers/${encodeURIComponent(id)}`, { method: 'DELETE' })
}

export function useCronTriggers(params: ListCronTriggersParams = {}) {
  return useQuery({
    queryKey: ['cron-triggers', params],
    queryFn: () => listCronTriggers(params),
    placeholderData: keepPreviousData,
  })
}

export function useCronTrigger(id: string | undefined) {
  return useQuery({
    queryKey: ['cron-triggers', 'detail', id],
    queryFn: () => getCronTrigger(id!),
    enabled: id !== undefined,
  })
}

export function useCreateCronTrigger() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: createCronTrigger,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['cron-triggers'] }),
  })
}

export function useUpdateCronTrigger() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: CronTriggerUpdateRequest }) =>
      updateCronTrigger(id, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['cron-triggers'] }),
  })
}

export function useDeleteCronTrigger() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: deleteCronTrigger,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['cron-triggers'] }),
  })
}