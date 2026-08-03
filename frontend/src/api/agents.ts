import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { request } from './client'
import type { Paginated } from './types'

export type AgentLLM = { model: string; provider?: string; maxTurns?: number; temperature?: number }
export type AgentPersona = { id: string }
export type AgentImageRef = { name: string; displayName?: string }
export type AgentOllamaCloud = { apiKey?: string }
export type AgentOpenCode = { apiKey?: string }
export type AgentAlibabaCloud = { apiKey?: string }
export type AgentCustomProvider = { url?: string; apiKey?: string }

export type AgentSummary = {
  id: string
  name: string
  description?: string
  llm?: { model: string }
  imageRef?: AgentImageRef
  persona?: AgentPersona
  replicas?: number
  status?: { ready: boolean; replicas?: number }
}

export type AgentResponse = AgentSummary & {
  llm?: AgentLLM
  persona?: AgentPersona
  enabledTools?: string[]
  ollamaCloud?: AgentOllamaCloud
  openCode?: AgentOpenCode
  alibabaCloud?: AgentAlibabaCloud
  customProvider?: AgentCustomProvider
}

export type AgentRequest = Omit<AgentResponse, 'id' | 'status'> & { groupId?: string }

export type ListAgentsParams = {
  page?: number
  pageSize?: number
}

export function listAgents(params: ListAgentsParams = {}) {
  return request<Paginated<AgentSummary>>('/agents', { query: params })
}

export function getAgent(id: string) {
  return request<AgentResponse>(`/agents/${encodeURIComponent(id)}`)
}

export function createAgent(body: AgentRequest) {
  return request<AgentResponse>('/agents', { method: 'POST', body })
}

export function updateAgent(id: string, body: AgentRequest) {
  return request<AgentResponse>(`/agents/${encodeURIComponent(id)}`, {
    method: 'PUT',
    body,
  })
}

export function deleteAgent(id: string) {
  return request<void>(`/agents/${encodeURIComponent(id)}`, { method: 'DELETE' })
}

export function useAgents(params: ListAgentsParams = {}) {
  return useQuery({
    queryKey: ['agents', params],
    queryFn: () => listAgents(params),
    placeholderData: keepPreviousData,
  })
}

export function useAgent(id: string | undefined) {
  return useQuery({
    queryKey: ['agents', 'detail', id],
    queryFn: () => getAgent(id!),
    enabled: id !== undefined,
  })
}

export function useCreateAgent() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: createAgent,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['agents'] }),
  })
}

export function useUpdateAgent() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: AgentRequest }) => updateAgent(id, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['agents'] }),
  })
}

export function useDeleteAgent() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: deleteAgent,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['agents'] }),
  })
}
