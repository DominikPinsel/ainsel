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
