import { useQuery } from '@tanstack/react-query'
import { request } from './client'

// Shape mirrors what GET /api/v1/observability/metrics/summary returns:
// scalar floats per metric. Backend names: snake_case for the metric registry,
// but JSON tags use camelCase — and the errors counter is `routingErrors`.
export type ObservabilitySummary = {
  eventsConsumed: number
  triggersMatched: number
  eventsRouted: number
  routingErrors: number
  updatedAt: string
}

export type TimeseriesPoint = {
  timestamp: string
  value: number
}

export type TimeseriesSeries = {
  name: string
  points: TimeseriesPoint[]
}

// Backend returns a single metric's points flat (no series wrapper); the
// metric registry uses snake_case names.
export type ObservabilityTimeseries = {
  metric: string
  range: string
  step?: string
  points: TimeseriesPoint[]
}

export type Range = '1h' | '6h' | '24h' | '7d'

export type MetricName =
  | 'events_consumed'
  | 'triggers_matched'
  | 'events_routed'
  | 'routing_errors'

export type TimeseriesParams = {
  range: Range
  metric: MetricName
}

export type TokensSummary = {
  totalTokens: number
  inputTokens: number
  outputTokens: number
}

export type TokensSubjectRow = {
  agent: string
  agentName?: string
  repo?: string
  eventType?: string
  model?: string
  inputTokens: number
  outputTokens: number
  totalTokens: number
}

export type TokensBySubject = {
  range: string
  rows: TokensSubjectRow[]
}

export type TokenEventRow = {
  event: string
  inputTokens: number
  outputTokens: number
  totalTokens: number
}

export type TokensByEvent = {
  range: string
  rows: TokenEventRow[]
}

export type LogLevel = 'debug' | 'info' | 'warn' | 'error'

export type LogEntry = {
  timestamp: string
  level?: LogLevel
  message: string
  app?: string
  agent?: string
  agentName?: string
  labels?: Record<string, string>
}

export type LogsParams = {
  app?: string
  agent?: string
  range?: Range
  limit?: number
}

export function getObservabilitySummary(range?: Range) {
  const query = range ? { range } : undefined
  return request<ObservabilitySummary>('/observability/metrics/summary', { query })
}

export function useObservabilitySummary(range?: Range) {
  return useQuery({
    queryKey: ['observability', 'summary', range ?? 'default'],
    queryFn: () => getObservabilitySummary(range),
  })
}

export function getObservabilityTimeseries(params: TimeseriesParams) {
  return request<ObservabilityTimeseries>('/observability/metrics/timeseries', {
    query: params,
  })
}

export function useObservabilityTimeseries(params: TimeseriesParams) {
  return useQuery({
    queryKey: ['observability', 'timeseries', params],
    queryFn: () => getObservabilityTimeseries(params),
  })
}

export function getTokensSummary(range?: Range) {
  const query = range ? { range } : undefined
  return request<TokensSummary>('/observability/metrics/tokens/summary', { query })
}

export function useTokensSummary(range?: Range) {
  return useQuery({
    queryKey: ['observability', 'tokens', 'summary', range ?? 'default'],
    queryFn: () => getTokensSummary(range),
  })
}

export function getTokensBySubject(range: Range) {
  return request<TokensBySubject>('/observability/metrics/tokens/by-subject', {
    query: { range },
  })
}

export function useTokensBySubject(range: Range) {
  return useQuery({
    queryKey: ['observability', 'tokens', 'by-subject', range],
    queryFn: () => getTokensBySubject(range),
  })
}

export function getTokensByEvent(range: Range) {
  return request<TokensByEvent>('/observability/metrics/tokens/by-event', {
    query: { range },
  })
}

export function useTokensByEvent(range: Range) {
  return useQuery({
    queryKey: ['observability', 'tokens', 'by-event', range],
    queryFn: () => getTokensByEvent(range),
  })
}

// Backend wraps the list in `{ logs, total, query }`; unwrap so consumers can
// keep treating the result as an array. The raw element shape from the backend
// is `{ timestamp, message, agentName?, labels? }` — `level`, `app`, and
// `agent` live inside the Loki stream `labels` map, not as top-level fields.
// We normalize here so every consumer sees consistent top-level fields.
type LogsEnvelope = {
  logs: LogEntry[]
  total: number
  query: string
}

// normalizeLogEntry copies `labels.agent`, `labels.app`, and `labels.level`
// into top-level fields when the direct fields are absent. This lets UI
// components read `row.agent`, `row.app`, `row.level` without worrying about
// the labels fallback.
function normalizeLogEntry(entry: LogEntry): LogEntry {
  const labels = entry.labels
  if (!labels) return entry
  return {
    ...entry,
    agent: entry.agent ?? labels.agent,
    app: entry.app ?? labels.app,
    level: entry.level ?? (labels.level as LogLevel | undefined),
  }
}

export async function getObservabilityLogs(params: LogsParams = {}): Promise<LogEntry[]> {
  const env = await request<LogsEnvelope>('/observability/logs', { query: params })
  return (env.logs ?? []).map(normalizeLogEntry)
}

export function useObservabilityLogs(
  params: LogsParams,
  opts: { enabled?: boolean; refetchInterval?: number | false } = {},
) {
  return useQuery({
    queryKey: ['observability', 'logs', params],
    queryFn: () => getObservabilityLogs(params),
    refetchInterval: opts.refetchInterval ?? false,
    enabled: opts.enabled ?? true,
  })
}