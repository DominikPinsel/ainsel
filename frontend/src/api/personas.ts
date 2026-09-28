import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { request } from './client'
import type { Paginated } from './types'

export type PersonaSummary = {
  id: string
  name: string
  description: string
  currentVersion: number
  createdAt: string
  updatedAt: string
}

export type PersonaResponse = PersonaSummary & {
  text: string
  /** Agent CR name when this persona is owned by a single agent. */
  ownerAgent?: string
}

export type PersonaRequest = {
  name?: string
  groupId?: string
  description?: string
  text?: string
}

export type ListPersonasParams = {
  page?: number
  pageSize?: number
}

export function listPersonas(params: ListPersonasParams = {}) {
  return request<Paginated<PersonaSummary>>('/personas', { query: params })
}

export function getPersona(id: string) {
  return request<PersonaResponse>(`/personas/${encodeURIComponent(id)}`)
}

export function createPersona(body: PersonaRequest) {
  return request<PersonaResponse>('/personas', { method: 'POST', body })
}

export function updatePersona(id: string, body: PersonaRequest) {
  return request<PersonaResponse>(`/personas/${encodeURIComponent(id)}`, {
    method: 'PUT',
    body,
  })
}

export function deletePersona(id: string) {
  return request<void>(`/personas/${encodeURIComponent(id)}`, { method: 'DELETE' })
}

export function usePersonas(params: ListPersonasParams = {}) {
  return useQuery({
    queryKey: ['personas', params],
    queryFn: () => listPersonas(params),
    placeholderData: keepPreviousData,
  })
}

export function usePersona(id: string | undefined) {
  return useQuery({
    queryKey: ['personas', 'detail', id],
    queryFn: () => getPersona(id!),
    enabled: id !== undefined && id !== '',
  })
}

export function useCreatePersona() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: createPersona,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['personas'] }),
  })
}

export function useUpdatePersona() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: PersonaRequest }) => updatePersona(id, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['personas'] }),
  })
}

export function useDeletePersona() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: deletePersona,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['personas'] }),
  })
}

// ---------------------------------------------------------------------------
// Version history
//
// Every text edit appends a `persona_versions` row, so a persona carries its
// full edit history. The listing is metadata only — fetch a single version for
// its text. Rollback copies an old text into a *new* current version rather
// than rewinding, so history is never rewritten.
//
// These endpoints require read/write access to the persona, so they can 403
// for a caller who can see the persona in a listing but not read it.
// ---------------------------------------------------------------------------

export type PersonaVersionSummary = {
  personaId: string
  versionNumber: number
  createdAt: string
}

export type PersonaVersion = PersonaVersionSummary & {
  text: string
}

export type ListPersonaVersionsParams = {
  page?: number
  pageSize?: number
}

export function listPersonaVersions(
  id: string,
  params: ListPersonaVersionsParams = {},
) {
  return request<Paginated<PersonaVersionSummary>>(
    `/personas/${encodeURIComponent(id)}/versions`,
    { query: params },
  )
}

export function getPersonaVersion(id: string, versionNumber: number) {
  return request<PersonaVersion>(
    `/personas/${encodeURIComponent(id)}/versions/${versionNumber}`,
  )
}

export function rollbackPersona(id: string, toVersion: number) {
  return request<PersonaResponse>(`/personas/${encodeURIComponent(id)}/rollback`, {
    method: 'POST',
    body: { toVersion },
  })
}

export function usePersonaVersions(
  id: string | undefined,
  params: ListPersonaVersionsParams = {},
) {
  return useQuery({
    queryKey: ['personas', 'versions', id, params],
    queryFn: () => listPersonaVersions(id!, params),
    enabled: id !== undefined && id !== '',
    placeholderData: keepPreviousData,
  })
}

/** Fetches one version's text. Pass `null` to keep the query disabled. */
export function usePersonaVersion(id: string | undefined, versionNumber: number | null) {
  return useQuery({
    queryKey: ['personas', 'version', id, versionNumber],
    queryFn: () => getPersonaVersion(id!, versionNumber!),
    enabled: id !== undefined && id !== '' && versionNumber !== null,
  })
}

export function useRollbackPersona() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, toVersion }: { id: string; toVersion: number }) =>
      rollbackPersona(id, toVersion),
    // A rollback bumps currentVersion and appends a history row, so the
    // listing, the detail view and the version list all go stale at once.
    onSuccess: () => qc.invalidateQueries({ queryKey: ['personas'] }),
  })
}

// ---------------------------------------------------------------------------
// Agent-scoped personas
//
// Editing an agent's persona inline must not rewrite a shared template that
// other agents use, so the hub forks a private, agent-owned persona on the
// first save (copy-on-write). `owned` tells the UI which case it is in.
// ---------------------------------------------------------------------------

export type AgentPersonaView = {
  /** True when the referenced persona belongs to this agent. */
  owned: boolean
  /** Persona id the Agent CR references, even if it no longer exists. */
  ref?: string
  persona?: PersonaResponse
}

export type AgentPersonaBody = {
  name?: string
  description?: string
  text: string
}

export function getAgentPersona(agentId: string) {
  return request<AgentPersonaView>(`/agents/${encodeURIComponent(agentId)}/persona`)
}

export function saveAgentPersona(agentId: string, body: AgentPersonaBody) {
  return request<AgentPersonaView>(`/agents/${encodeURIComponent(agentId)}/persona`, {
    method: 'PUT',
    body,
  })
}

export function useAgentPersona(agentId: string) {
  return useQuery({
    queryKey: ['agent-persona', agentId],
    queryFn: () => getAgentPersona(agentId),
  })
}

export function useSaveAgentPersona() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ agentId, body }: { agentId: string; body: AgentPersonaBody }) =>
      saveAgentPersona(agentId, body),
    onSuccess: (view, { agentId }) => {
      qc.setQueryData(['agent-persona', agentId], view)
      // A copy-on-write save re-points the agent's persona reference.
      qc.invalidateQueries({ queryKey: ['agents'] })
      qc.invalidateQueries({ queryKey: ['personas'] })
    },
  })
}
