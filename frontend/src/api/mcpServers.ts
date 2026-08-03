import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { request } from './client'

export type MCPServerSummary = {
  name: string
  displayName: string
  description?: string
  url: string
  tokenFromEnv?: string
  createdAt: string
  updatedAt: string
}

export type MCPToolSummary = {
  name: string
  description?: string
}

export type MCPServerRequest = {
  name: string
  groupId?: string
  displayName: string
  description?: string
  url: string
  tokenFromEnv?: string
}

export type MCPServerUpdateRequest = {
  displayName: string
  description?: string
  url: string
  tokenFromEnv?: string
}

export function listMCPServers() {
  return request<MCPServerSummary[]>('/mcp-servers')
}

export function getMCPServer(name: string) {
  return request<MCPServerSummary>(`/mcp-servers/${encodeURIComponent(name)}`)
}

export function createMCPServer(body: MCPServerRequest) {
  return request<MCPServerSummary>('/mcp-servers', { method: 'POST', body })
}

export function updateMCPServer(name: string, body: MCPServerUpdateRequest) {
  return request<MCPServerSummary>(`/mcp-servers/${encodeURIComponent(name)}`, {
    method: 'PUT',
    body,
  })
}

export function deleteMCPServer(name: string) {
  return request<void>(`/mcp-servers/${encodeURIComponent(name)}`, { method: 'DELETE' })
}

export function getMCPServerTools(name: string) {
  return request<MCPToolSummary[]>(`/mcp-servers/${encodeURIComponent(name)}/tools`)
}

export function useMCPServers() {
  return useQuery({
    queryKey: ['mcp-servers'],
    queryFn: listMCPServers,
  })
}

export function useMCPServer(name: string | undefined) {
  return useQuery({
    queryKey: ['mcp-servers', 'detail', name],
    queryFn: () => getMCPServer(name!),
    enabled: name !== undefined,
  })
}

export function useCreateMCPServer() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: createMCPServer,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['mcp-servers'] }),
  })
}

export function useUpdateMCPServer() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ name, body }: { name: string; body: MCPServerUpdateRequest }) =>
      updateMCPServer(name, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['mcp-servers'] }),
  })
}

export function useDeleteMCPServer() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: deleteMCPServer,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['mcp-servers'] }),
  })
}

export function useMCPServerTools(name: string | undefined) {
  return useQuery({
    queryKey: ['mcp-servers', 'tools', name],
    queryFn: () => getMCPServerTools(name!),
    enabled: name !== undefined,
  })
}
