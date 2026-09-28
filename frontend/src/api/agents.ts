import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { request } from './client'
import type { Paginated } from './types'

export type AgentLLM = {
  model: string
  provider?: string
  maxTurns?: number
  temperature?: number
  /** Model accepts image input; advertised in pi's models.json `input` array. */
  vision?: boolean
}
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
/** One of an agent's own environment variables, layered on top of the runtime
 *  image's env. The hub masks secret values, so a secret entry always reads
 *  back with an empty value. */
export type AgentEnvVar = { name: string; value: string; secret?: boolean }
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
  /** Pod floor. Only present on agents that opted into queue scaling; the
   *  agent sleeps down to this count when the queue drains. */
  minReplicas?: number
  status?: {
    ready: boolean
    /** Pods running right now. */
    replicas?: number
    /** Pods the operator wants, from the queue signal. */
    desired?: number
    /** 'queue' when scaling follows the queue, otherwise the count is static. */
    mode?: string
    /** Operator's one-word explanation, e.g. 'ScaledToZero'. */
    reason?: string
    message?: string
  }
  /** RFC3339 timestamp of the last hub-mediated write (falls back to
   *  creation time for agents that predate the annotation). */
  updatedAt?: string
}

/**
 * True when the agent is parked at zero containers by its own scaling floor
 * rather than failing to schedule. Only the operator's mode says which, so the
 * running count alone can never be the test — treating "0" as asleep would hide a
 * genuinely broken agent.
 */
export function isAsleep(status?: AgentSummary['status']): boolean {
  return status?.mode === 'queue' && (status.desired ?? 0) === 0
}

export type AgentResponse = AgentSummary & {
  llm?: AgentLLM
  persona?: AgentPersona
  enabledTools?: string[]
  skills?: AgentSkills
  mcp?: AgentMCP
  /** This agent's env overrides; absent = it runs on the image env alone. */
  env?: AgentEnvVar[]
  ollamaCloud?: AgentOllamaCloud
  openCode?: AgentOpenCode
  alibabaCloud?: AgentAlibabaCloud
  customProvider?: AgentCustomProvider
}

export type AgentRequest = Omit<
  AgentResponse,
  'id' | 'status' | 'mcp' | 'env'
> & {
  groupId?: string
  /** Write-side MCP selection: registry names, resolved by the hub to
   *  full definitions when writing the agent. */
  mcp?: { servers: string[] }
  /** Write-side env overrides: replaces this agent's list. Present-but-empty
   *  clears the overrides so the agent runs on the image env again; a secret
   *  entry submitted with an empty value keeps its stored value. */
  env?: AgentEnvVar[]
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
