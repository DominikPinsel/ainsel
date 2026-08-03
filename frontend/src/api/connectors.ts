import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { request } from './client'
import type { Paginated } from './types'

export interface ConnectorCreateRequest {
  name: string
  groupId?: string
  signatureHeader?: string
}

export interface ConnectorUpdateRequest {
  name?: string
  disabled?: boolean
}

export interface ConnectorCondition {
  type: string
  status: string
  reason?: string
}

export interface ConnectorStatus {
  ready: boolean
  conditions?: ConnectorCondition[]
}

export interface ConnectorResponse {
  id: string
  name: string
  signatureHeader: string
  webhookEndpoint?: string
  webhookSecretValue?: string  // Only on create response
  status?: ConnectorStatus
  disabled: boolean
}

export type ListConnectorsParams = {
  page?: number
  pageSize?: number
}

export function listConnectors(params: ListConnectorsParams = {}) {
  return request<Paginated<ConnectorResponse>>('/connectors', { query: params })
}

export function getConnector(id: string) {
  return request<ConnectorResponse>(`/connectors/${encodeURIComponent(id)}`)
}

export function createConnector(body: ConnectorCreateRequest) {
  return request<ConnectorResponse>('/connectors', { method: 'POST', body })
}

export function updateConnector(id: string, body: ConnectorUpdateRequest) {
  return request<ConnectorResponse>(`/connectors/${encodeURIComponent(id)}`, {
    method: 'PUT',
    body,
  })
}

export function deleteConnector(id: string) {
  return request<void>(`/connectors/${encodeURIComponent(id)}`, { method: 'DELETE' })
}

export function rotateConnectorSecret(id: string) {
  return request<ConnectorResponse>(
    `/connectors/${encodeURIComponent(id)}/rotate-secret`,
    { method: 'POST' },
  )
}

export function useConnectors(params: ListConnectorsParams = {}) {
  return useQuery({
    queryKey: ['connectors', params],
    queryFn: () => listConnectors(params),
    placeholderData: keepPreviousData,
  })
}

export function useConnector(id: string | undefined) {
  return useQuery({
    queryKey: ['connectors', 'detail', id],
    queryFn: () => getConnector(id!),
    enabled: id !== undefined,
  })
}

export function useCreateConnector() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: createConnector,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['connectors'] }),
  })
}

export function useUpdateConnector() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: ConnectorUpdateRequest }) =>
      updateConnector(id, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['connectors'] }),
  })
}

export function useDeleteConnector() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: deleteConnector,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['connectors'] }),
  })
}

export function useRotateConnectorSecret() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: rotateConnectorSecret,
    onSuccess: (_, id) => qc.invalidateQueries({ queryKey: ['connectors', 'detail', id] }),
  })
}
