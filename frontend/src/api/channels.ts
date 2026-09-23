import { useQuery } from '@tanstack/react-query'
import { useAgents } from './agents'
import { useConnectors } from './connectors'
import { listEventsPage } from './events'
import type { TriggerSummary } from './triggers'

/**
 * Channels are the named streams events live in (see the Channels design
 * spec). The backend does not have a channels entity yet, so channels are
 * derived from the existing registries: one channel per connector, one per
 * agent.
 *
 * A channel has **no role** — it neither "produces" nor "consumes". Events
 * are born in a channel and move between channels when a subscription
 * transfers them; an agent takes everything from its own channel. The
 * channel is just the stream.
 *
 * Identity is the channel id, never the name: a connector `forgejo` and an
 * agent `forgejo` are two distinct channels that merely share a display
 * label. Every FK, route, and UI link uses the id.
 */

export type ChannelOrigin = 'connector' | 'agent'

export type ChannelSummary = {
  /** Stable channel identifier — the future channels.id. Encodes origin so
   *  two channels named `forgejo` never collapse into one. */
  id: string
  /** Rename-able display label (the future channels.name) — NOT unique. */
  name: string
  displayName: string
  /** One line of intent (the future channels.description). */
  description: string
  /** What the channel was provisioned for — provenance metadata, used only
   *  for display defaults and mounting the right config panels. */
  origin: ChannelOrigin
  /** Registry id of the owning entity (agent/connector) when resolvable —
   *  used to mount that entity's configuration panels. */
  entityId?: string
}

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
      description: `Where ${c.name} events arrive`,
      origin: 'connector',
      entityId: c.id,
    })
  }
  for (const a of agents ?? []) {
    channels.push({
      id: idFor('agent', a.name),
      name: a.name,
      displayName: a.name,
      description: `The inbox agent ${a.name} drains`,
      origin: 'agent',
      entityId: a.id,
    })
  }
  return channels.sort(
    (a, b) => a.name.localeCompare(b.name) || a.id.localeCompare(b.id),
  )
}

/** Aggregated channel list: one per connector plus one per agent, keyed by id. */
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

/**
 * A real connection between two channels: a subscription on the destination
 * transfers events from the source. Derived from triggers' registry ids —
 * in the target model these are channel FKs.
 */
export type ChannelConnection = {
  subscription: string
  from?: ChannelSummary
  to?: ChannelSummary
  /** Registry ids even when the channel cannot be resolved (deleted
   *  connector/agent) — consumers filter on these. */
  fromRef?: string
  toRef?: string
}

export function buildConnections(
  channels: ChannelSummary[],
  triggers: TriggerSummary[] | undefined,
): ChannelConnection[] {
  const byEntity = new Map(
    channels
      .filter((c) => c.entityId !== undefined)
      .map((c) => [`${c.origin}#${c.entityId}`, c]),
  )
  return (triggers ?? []).map((t) => ({
    subscription: t.name,
    from: t.connectorRef ? byEntity.get(`connector#${t.connectorRef}`) : undefined,
    to: t.agentRef ? byEntity.get(`agent#${t.agentRef}`) : undefined,
    fromRef: t.connectorRef,
    toRef: t.agentRef,
  }))
}

/** The hub's synthetic connector labels for directly-emitted events. These
 *  are NOT channels (there is no shared cron/chat channel): scheduled ticks
 *  and chat messages are born directly on the target agent's channel. Until
 *  the backend makes that literal, the events still carry these labels, so
 *  the journey renders them as direct births instead of a channel link. */
export const DIRECT_SOURCES = ['cron', 'chat'] as const

export function isDirectSource(producer: string | undefined): boolean {
  return (DIRECT_SOURCES as readonly string[]).includes(producer ?? '')
}

export type ChannelCounts = {
  /** Events that entered this channel in the last 24h. */
  events24h: number
  /** Of those, events no subscription transferred onward (connector channels). */
  unmatched24h: number
  /** Runs that ended in failure or timeout. */
  failed24h: number
}

function since24h(): string {
  return new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString()
}

/**
 * Live counts for one channel via the events API filters (`connector` for
 * where events arrive, `agent` for what reached the inbox).
 */
export function useChannelCounts(channel: ChannelSummary) {
  const isAgentInbox = channel.origin === 'agent'
  const scope = isAgentInbox ? { agent: channel.name } : { connector: channel.name }
  const since = since24h()

  const events = useQuery({
    queryKey: ['channels', 'counts', channel.id, 'events', scope, since],
    queryFn: () => listEventsPage({ ...scope, since, limit: 1 }),
  })
  const unmatched = useQuery({
    queryKey: ['channels', 'counts', channel.id, 'unmatched', scope, since],
    queryFn: () => listEventsPage({ ...scope, since, status: 'unmatched', limit: 1 }),
    enabled: !isAgentInbox,
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
      events.isLoading || failed.isLoading || (!isAgentInbox && unmatched.isLoading),
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

/** Channel id an event was born in, given its connector string. Direct
 *  sources (cron/chat) have no channel — guard with isDirectSource(). */
export function channelIdForProducer(producer: string): string {
  return `connector:${producer}`
}
