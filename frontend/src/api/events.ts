import { useQuery } from '@tanstack/react-query'
import { request } from './client'

export type ActivityStatus = 'matched' | 'unmatched' | 'error'

export type RunStatus = 'running' | 'success' | 'failure' | 'timeout'

export type ActivityMatch = {
  trigger: string
  agent: string
  runStatus?: RunStatus
  durationMs?: number
  error?: string
}

export type ActivityEntry = {
  id: string
  timestamp: string
  connector?: string
  status: ActivityStatus
  matches?: ActivityMatch[]
  payload?: unknown
}

export type ListEventsParams = {
  limit?: number
  status?: ActivityStatus
  connector?: string
  since?: string
}

// Backend wraps the list in `{ events, total }`; unwrap so consumers can
// keep treating the result as an array.
type EventsEnvelope = {
  events: ActivityEntry[]
  total: number
}

export async function listEvents(params: ListEventsParams = {}): Promise<ActivityEntry[]> {
  const env = await request<EventsEnvelope>('/events', { query: params })
  return env.events ?? []
}

export async function getEvent(id: string): Promise<ActivityEntry> {
  return request<ActivityEntry>(`/events/${encodeURIComponent(id)}`)
}

export function useEvent(id: string) {
  return useQuery({
    queryKey: ['events', 'detail', id],
    queryFn: () => getEvent(id),
    enabled: id !== '',
  })
}

export function useRecentEvents(limit: number) {
  return useQuery({
    queryKey: ['events', 'recent', limit],
    queryFn: () => listEvents({ limit }),
  })
}
