import { useQuery } from '@tanstack/react-query'
import { useAgents } from './agents'
import { useConnectors } from './connectors'
import { listEventsPage } from './events'

/**
 * Channels are the event streams agents and connectors read from / write to
 * (see docs/superpowers/specs — Channels design). The backend does not have a
 * channels entity yet, so channels are derived from the existing registries.
 *
 * Crucially, a channel is identified by its **UUID-like key**, not its name:
 * a connector `forgejo` and an agent `forgejo` are two distinct channels that
 * merely share a display label. Name collisions are expected; every FK,
 * route, and UI link uses the channel key.
 */

export type ChannelOrigin = 'connector' | 'agent' | 'builtin'

export type ChannelRole = 'produces' | 'consumes'

export type ChannelSummary = {
  /** Stable channel identifier — the future channels.id. Encodes origin so
   *  two channels named `forgejo` never collapse into one. */
  id: string
  /** Rename-able display label (the future channels.name) — NOT unique. */
  name: string
  displayName: string
  /** One line of intent (the future channels.description). */
  description: string
  /** Where the channel comes from — presentation hint only. */
  origin: ChannelOrigin
  roles: ChannelRole[]
  /** Registry id of the owning entity (agent/connector) when resolvable —
   *  used to mount that entity's configuration panels. */
  entityId?: string
}

/** The built-in synthetic sources the hub publishes into channels. In the
 *  target design schedules and chat live on the consuming agent's own
 *  channel; these built-ins surface the events produced today. */
export const BUILTIN_CHANNELS: ChannelSummary[] = [
  {
    id: 'builtin:cron',
    name: 'cron',
    displayName: 'cron',
    description: 'Scheduled ticks (cron)',
    origin: 'builtin',
    roles: ['produces'],
  },
  {
    id: 'builtin:chat',
    name: 'chat',
    displayName: 'chat',
    description: 'Messages sent to agents',
    origin: 'builtin',
    roles: ['produces'],
  },
]

function idFor(origin: ChannelOrigin, name: string): string {
  return `${origin}:${name}`
}

function buildChannelSummaries(
  agents: { id: string; name: string }[] | undefined,
  connectors: { id: string; name: string }[] | undefined,
): ChannelSummary[] {
  const channels: ChannelSummary[] = []
  for (const c of connectors ?? []) {
    channels.push({
      id: idFor('connector', c.name),
      name: c.name,
      displayName: c.name,
      description: `Ingested from the ${c.name} connector`,
      origin: 'connector',
      roles: ['produces'],
      entityId: c.id,
    })
  }
  for (const a of agents ?? []) {
    channels.push({
      id: idFor('agent', a.name),
      name: a.name,
      displayName: a.name,
      description: `Inbox of agent ${a.name}`,
      origin: 'agent',
      roles: ['consumes'],
      entityId: a.id,
    })
  }
  channels.push(...BUILTIN_CHANNELS)
  return channels.sort(
    (a, b) => a.name.localeCompare(b.name) || a.id.localeCompare(b.id),
  )
}

/** Aggregated channel list: connectors + agents + built-ins, keyed by id. */
export function useChannels() {
  const agents = useAgents({ pageSize: 500 })
  const connectors = useConnectors({ pageSize: 500 })
  const channels = buildChannelSummaries(agents.data?.items, connectors.data?.items)
  return {
    channels,
    isLoading: agents.isLoading || connectors.isLoading,
    error: agents.error ?? connectors.error,
  }
}

export type ChannelCounts = {
  /** Events entered this channel in the last 24h. */
  events24h: number
  /** Of those, events no rule fanned out (unmatched). */
  unmatched24h: number
  /** Runs that ended in failure or timeout. */
  failed24h: number
}

function since24h(): string {
  return new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString()
}

/**
 * Live counts for one channel via the events API filters
 * (`connector` for producing channels, `agent` for consumed channels).
 */
export function useChannelCounts(channel: ChannelSummary) {
  const isProducer = channel.roles.includes('produces')
  const scope = isProducer ? { connector: channel.name } : { agent: channel.name }
  const since = since24h()

  const events = useQuery({
    queryKey: ['channels', 'counts', channel.id, 'events', scope, since],
    queryFn: () => listEventsPage({ ...scope, since, limit: 1 }),
  })
  const unmatched = useQuery({
    queryKey: ['channels', 'counts', channel.id, 'unmatched', scope, since],
    queryFn: () => listEventsPage({ ...scope, since, status: 'unmatched', limit: 1 }),
    enabled: isProducer,
  })
  const failed = useQuery({
    queryKey: ['channels', 'counts', channel.id, 'failed', scope, since],
    queryFn: () => listEventsPage({ ...scope, since, status: 'error', limit: 1 }),
  })

  return {
    events24h: events.data?.total ?? 0,
    unmatched24h: unmatched.data?.total ?? 0,
    failed24h: failed.data?.total ?? 0,
    isLoading:
      events.isLoading || failed.isLoading || (isProducer && unmatched.isLoading),
  }
}

/** Route for a channel — origin-namespaced so same-labeled channels never
 *  collapse (the target model addresses channels by id; this is the derived
 *  stand-in). */
export function channelPath(id: string): string {
  const sep = id.indexOf(':')
  const origin = sep >= 0 ? id.slice(0, sep) : 'connector'
  const name = sep >= 0 ? id.slice(sep + 1) : id
  return `/channels/${origin}/${encodeURIComponent(name)}`
}

/** Channel id an event was born in, given its producing connector string. */
export function channelIdForProducer(producer: string): string {
  if (producer === 'cron' || producer === 'chat') return `builtin:${producer}`
  return `connector:${producer}`
}
