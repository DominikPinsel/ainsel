import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { request } from './client'
import type { Paginated } from './types'

export type ChatMessage = {
  id: number
  sessionId: string
  role: 'user' | 'assistant' | 'status'
  content: string
  tokens: number
  createdAt: string
}

export type ChatSession = {
  id: string
  name: string
  agentName: string
  userId: string
  createdAt: string
  updatedAt: string
  messages?: ChatMessage[]
}

export type ChatSessionSummary = Omit<ChatSession, 'messages'>

export type ListChatSessionsParams = {
  agent?: string
  userId?: string
  page?: number
  pageSize?: number
}

export function listChatSessions(params: ListChatSessionsParams = {}) {
  return request<Paginated<ChatSessionSummary>>('/chat/sessions', { query: params })
}

export function getChatSession(id: string) {
  return request<ChatSession>(`/chat/sessions/${encodeURIComponent(id)}`)
}

export function createChatSession(agentName: string) {
  return request<ChatSession>('/chat/sessions', {
    method: 'POST',
    body: { agentName },
  })
}

export function updateChatSession(id: string, name: string) {
  return request<ChatSession>(`/chat/sessions/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    body: { name },
  })
}

export function deleteChatSession(id: string) {
  return request<void>(`/chat/sessions/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

export function sendChatMessage(sessionId: string, content: string) {
  return request<ChatMessage>(`/chat/sessions/${encodeURIComponent(sessionId)}/messages`, {
    method: 'POST',
    body: { role: 'user', content },
  })
}

export function useChatSessions(params: ListChatSessionsParams = {}) {
  return useQuery({
    queryKey: ['chat', 'sessions', params],
    queryFn: () => listChatSessions(params),
    placeholderData: keepPreviousData,
  })
}

export function useChatSession(id: string | undefined) {
  return useQuery({
    queryKey: ['chat', 'session', id],
    queryFn: () => getChatSession(id!),
    enabled: id !== undefined && id !== '',
    // Poll while the session is open — the agent may respond asynchronously
    // via the MCP sidecar. The WebSocket broadcast is a secondary channel;
    // polling ensures we catch updates even if the WS drops.
    refetchInterval: 3000,
  })
}

export function useCreateChatSession() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (agentName: string) => createChatSession(agentName),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['chat', 'sessions'] }),
  })
}

export function useUpdateChatSession() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, name }: { id: string; name: string }) => updateChatSession(id, name),
    onSuccess: (_data, variables) => {
      qc.invalidateQueries({ queryKey: ['chat', 'sessions'] })
      qc.invalidateQueries({ queryKey: ['chat', 'session', variables.id] })
    },
  })
}

export function useDeleteChatSession() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: deleteChatSession,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['chat', 'sessions'] }),
  })
}

export function useSendChatMessage() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ sessionId, content }: { sessionId: string; content: string }) =>
      sendChatMessage(sessionId, content),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['chat'] }),
  })
}