import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { request } from './client'
import type { Paginated } from './types'

export type AgentLLM = { model: string; provider?: string; maxTurns?: number; temperature?: number }
export type AgentPersona = { id: string }
export type AgentImageRef = { name: string; displayName?: string }
/** Agent-scoped skill selection: present = explicit override (empty items
 *  = no skills), absent = inherit the runtime image's enabledSkills. */
export type AgentSkills = { items: string[] }
/** One agent-scoped MCP server definition, resolved from the hub's MCP
 *  registry at write time (a snapshot — registry edits don't rewrite
 *  running agents). */
export type AgentMCPServer = {
  name: string
  url: string
  tokenFromEnv?: string
}
/** Agent-scoped MCP selection: present = explicit override (empty servers
 *  = no connections), absent = inherit the image's mcpServers. */
export type AgentMCP = { servers: AgentMCPServer[] }
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
  /** RFC3339 timestamp of the last hub-mediated write (falls back to
   *  creation time for agents that predate the annotation). */
  updatedAt?: string
}

export type AgentResponse = AgentSummary & {
  llm?: AgentLLM
  persona?: AgentPersona
  enabledTools?: string[]
  skills?: AgentSkills
  mcp?: AgentMCP
  ollamaCloud?: AgentOllamaCloud
  openCode?: AgentOpenCode
  alibabaCloud?: AgentAlibabaCloud
  customProvider?: AgentCustomProvider
}

export type AgentRequest = Omit<AgentResponse, 'id' | 'status' | 'mcp'> & {
  groupId?: string
  /** Write-side MCP selection: registry names, resolved by the hub to
   *  full definitions when writing the agent. */
  mcp?: { servers: string[] }
}

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
