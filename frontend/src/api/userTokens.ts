import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { request } from './client'

export type UserToken = {
  id: string
  name: string
  expiresAt: string
  lastUsedAt: string | null
  createdAt: string
  revokedAt: string | null
}

export type CreatedToken = UserToken & { token: string }

export function listUserTokens() {
  return request<UserToken[]>('/user-tokens')
}

export function createUserToken(body: { name: string; expiresInDays: 30 | 60 | 90 }) {
  return request<CreatedToken>('/user-tokens', { method: 'POST', body })
}

export function revokeUserToken(id: string) {
  return request<void>(`/user-tokens/${encodeURIComponent(id)}`, { method: 'DELETE' })
}

export function useUserTokens() {
  return useQuery({ queryKey: ['user-tokens'], queryFn: listUserTokens })
}

export function useCreateUserToken() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: createUserToken,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['user-tokens'] }),
  })
}

export function useRevokeUserToken() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: revokeUserToken,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['user-tokens'] }),
  })
}
