import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { request } from './client'

export type HubUser = {
  id: string
  email: string
  username: string
  isAdmin: boolean
  createdAt: string
  updatedAt: string
}

export function listUsers() {
  return request<HubUser[]>('/users')
}

export function getUser(id: string) {
  return request<HubUser>(`/users/${encodeURIComponent(id)}`)
}

export function createUser(body: {
  username: string
  password: string
  email?: string
  isAdmin?: boolean
}) {
  return request<HubUser>('/users', { method: 'POST', body })
}

export function deleteUser(id: string) {
  return request<void>(`/users/${encodeURIComponent(id)}`, { method: 'DELETE' })
}

export function resetUserPassword(id: string, password: string) {
  return request<HubUser>(`/users/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    body: { password },
  })
}

export function updateUser(id: string, body: { isAdmin?: boolean }) {
  return request<HubUser>(`/users/${encodeURIComponent(id)}`, { method: 'PATCH', body })
}

export function useUsers() {
  return useQuery({ queryKey: ['users'], queryFn: listUsers })
}

export function useUser(id: string | undefined) {
  return useQuery({
    queryKey: ['users', id],
    queryFn: () => getUser(id!),
    enabled: id !== undefined,
  })
}

export function userDisplayName(user: { id: string; username: string; email: string }): string {
  if (user.username && user.username !== user.id) return user.username
  return user.email || user.id
}

/** True when neither a human-readable username nor an email is available. */
export function isUnsyncedUser(user: { id: string; username: string; email: string }): boolean {
  return (!user.username || user.username === user.id) && !user.email
}

export function useUpdateUser() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: { isAdmin?: boolean } }) => updateUser(id, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['users'] }),
  })
}

export function useCreateUser() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: createUser,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['users'] }),
  })
}

export function useDeleteUser() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: deleteUser,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['users'] }),
  })
}

export function useResetUserPassword() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, password }: { id: string; password: string }) =>
      resetUserPassword(id, password),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['users'] }),
  })
}

export function syncMe() {
  return request<HubUser>('/users/me/sync', { method: 'POST' })
}

export function syncUser(id: string) {
  return request<HubUser>(`/users/${encodeURIComponent(id)}/sync`, { method: 'POST' })
}

export function useSyncMe() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: syncMe,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['currentUser'] })
      qc.invalidateQueries({ queryKey: ['users'] })
    },
  })
}

export function useSyncUser() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => syncUser(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['users'] }),
  })
}
