import { encodeEventFilters, type EventFilter } from './eventFilters'

function buildSearch(params: Record<string, string | undefined>): string {
  const q = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v) q.set(k, v)
  }
  const s = q.toString()
  return s ? `?${s}` : ''
}

export function eventsLink(opts: {
  filters?: EventFilter[]
  status?: string
  connector?: string
  agent?: string
  range?: string
} = {}): string {
  return `/observability/events${buildSearch({
    q: opts.filters?.length ? encodeEventFilters(opts.filters) : undefined,
    status: opts.status,
    connector: opts.connector,
    agent: opts.agent,
    range: opts.range,
  })}`
}

export function routingLink(opts: { agent?: string; trigger?: string; range?: string } = {}): string {
  return `/observability/routing${buildSearch({
    agent: opts.agent,
    trigger: opts.trigger,
    range: opts.range,
  })}`
}

export function errorsLink(opts: { severity?: string; source?: string } = {}): string {
  return `/observability/errors${buildSearch({ severity: opts.severity, source: opts.source })}`
}

export function tokensLink(opts: { agent?: string; range?: string } = {}): string {
  return `/observability/tokens${buildSearch({ agent: opts.agent, range: opts.range })}`
}
