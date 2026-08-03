import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { request } from './client'
import type { Paginated } from './types'

export type ToolKind = 'container' | 'shell' | 'mcp'

export type ToolExample = { title: string; snippet: string }

export type Tool = {
  name: string
  kind: ToolKind
  description?: string
  mcpSource?: string
  enabled?: boolean  // undefined treated as true (backward compat for existing data)
  isNew?: boolean
  examples?: ToolExample[]
}

export type EnvVar = {
  name: string
  value: string
  secret?: boolean
}

export type MCPServer = {
  name: string
  url: string
  token: string
  secret?: boolean
}

export type Sidecar = {
  name: string
  image: string
  port?: number
  mcpPath?: string
  env?: EnvVar[]
}

export type AgentImageSummary = {
  id: string
  displayName?: string
  description?: string
  imageURL: string
  toolCount?: number
  enabledSkills?: string[]
}

export type AgentImageResponse = AgentImageSummary & {
  tools?: Tool[]
  env?: EnvVar[]
  mcpServers?: MCPServer[]
  sidecars?: Sidecar[]
}

export type AgentImageRequest = {
  displayName?: string
  groupId?: string
  description?: string
  imageURL: string
  tools?: Tool[]
  env?: EnvVar[]
  mcpServers?: string[]
  sidecars?: Sidecar[]
  enabledSkills?: string[]
}

export type ListAgentImagesParams = {
  page?: number
  pageSize?: number
}

export function listAgentImages(params: ListAgentImagesParams = {}) {
  return request<Paginated<AgentImageSummary>>('/agent-images', { query: params })
}

export function getAgentImage(id: string) {
  return request<AgentImageResponse>(`/agent-images/${encodeURIComponent(id)}`)
}

export function createAgentImage(body: AgentImageRequest) {
  return request<AgentImageResponse>('/agent-images', { method: 'POST', body })
}

export function updateAgentImage(id: string, body: AgentImageRequest) {
  return request<AgentImageResponse>(`/agent-images/${encodeURIComponent(id)}`, {
    method: 'PUT',
    body,
  })
}

export function deleteAgentImage(id: string) {
  return request<void>(`/agent-images/${encodeURIComponent(id)}`, { method: 'DELETE' })
}

export type RefreshMCPResponse = {
  image: AgentImageResponse
  warnings: string[]
}

export function refreshMCPTools(id: string) {
  return request<RefreshMCPResponse>(`/agent-images/${encodeURIComponent(id)}/refresh-mcp`, {
    method: 'POST',
  })
}

export function useAgentImages(params: ListAgentImagesParams = {}) {
  return useQuery({
    queryKey: ['agent-images', params],
    queryFn: () => listAgentImages(params),
    placeholderData: keepPreviousData,
  })
}

export function useAgentImage(id: string | undefined) {
  return useQuery({
    queryKey: ['agent-images', 'detail', id],
    queryFn: () => getAgentImage(id!),
    enabled: id !== undefined,
  })
}

export function useCreateAgentImage() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: createAgentImage,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['agent-images'] }),
  })
}

export function useUpdateAgentImage() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: AgentImageRequest }) =>
      updateAgentImage(id, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['agent-images'] }),
  })
}

export function useDeleteAgentImage() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: deleteAgentImage,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['agent-images'] }),
  })
}

export function useRefreshMCPTools() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => refreshMCPTools(id),
    onSuccess: (data, id) => {
      qc.setQueryData(['agent-images', 'detail', id], data.image)
      qc.invalidateQueries({ queryKey: ['agent-images'] })
    },
  })
}
