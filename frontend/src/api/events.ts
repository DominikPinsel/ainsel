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
  offset?: number
  status?: ActivityStatus
  connector?: string
  agent?: string
  since?: string
}

// Backend wraps the list in `{ events, total }`; total is the number of all
// events matching the filters (not just the returned page), which the
// Activity page uses to drive its pager.
type EventsEnvelope = {
  events: ActivityEntry[]
  total: number
}

export type EventsPage = {
  events: ActivityEntry[]
  total: number
}

export async function listEventsPage(params: ListEventsParams = {}): Promise<EventsPage> {
  const env = await request<EventsEnvelope>('/events', { query: params })
  return { events: env.events ?? [], total: env.total ?? 0 }
}

export async function listEvents(params: ListEventsParams = {}): Promise<ActivityEntry[]> {
  const env = await listEventsPage(params)
  return env.events
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

export function useEventsPage(params: ListEventsParams) {
  return useQuery({
    queryKey: [
      'events',
      'page',
      params.limit ?? null,
      params.offset ?? null,
      params.status ?? null,
      params.connector ?? null,
      params.agent ?? null,
      params.since ?? null,
    ],
    queryFn: () => listEventsPage(params),
  })
}
