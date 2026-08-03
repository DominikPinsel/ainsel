import { useQuery } from '@tanstack/react-query'
import { request } from './client'

export type ConversationRole = 'user' | 'assistant' | 'toolResult'

export type ConversationMessage = {
  id: number
  invocationId?: string
  correlationId?: string
  agentName: string
  role: ConversationRole
  content: string // JSON-serialized blocks
  model?: string
  inputTokens?: number
  outputTokens?: number
  stopReason?: string
  createdAt: string
}

type ConversationsEnvelope = { messages: ConversationMessage[]; total: number }

export type ListConversationsParams = {
  invocation?: string
  agent?: string
  correlation?: string
  limit?: number
}

export type ConversationsResult = { messages: ConversationMessage[]; total: number }

export async function listConversations(
  params: ListConversationsParams = {},
): Promise<ConversationsResult> {
  const env = await request<ConversationsEnvelope>('/observability/conversations', {
    query: params,
  })
  return { messages: env.messages ?? [], total: env.total ?? 0 }
}

export function useConversations(params: ListConversationsParams) {
  return useQuery({
    queryKey: ['conversations', params],
    queryFn: () => listConversations(params),
    enabled: !!params.invocation,
  })
}
