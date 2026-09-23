import { useQuery } from '@tanstack/react-query'
import { useAgents } from './agents'
import { useConnectors } from './connectors'
import { listEventsPage } from './events'
import { useCustomChannels, type ChannelBridge } from './customChannels'
import type { TriggerSummary } from './triggers'

/**
 * Channels are the named streams events live in (see the Channels design
 * spec). Three kinds exist:
 *
 * - **connector** — where a connector's events arrive. One per connector,
 *   auto-provisioned, never deletable: the connector has to put its events
 *   somewhere.
 * - **agent** — the agent's inbox. Everything born in it is prompted to
 *   the agent. One per agent, auto-provisioned, never deletable: it is the
 *   agent.
 * - **custom** — user-created grouping channels (e.g. to collect
 *   subscriptions under one name). Deletable while nothing is attached.
 *
 * Custom channels are currently browser-local (`api/customChannels`): the
 * hub has no channels table yet, so they can only carry local bridges —
 * preview edges that visualise the grouping but move no real events.
 *
 * Identity is the channel id, never the name: a connector `forgejo` and an
 * agent `forgejo` are two distinct channels that merely share a display
 * label. Every FK, route, and UI link uses the id.
 */

export type ChannelKind = 'connector' | 'agent' | 'custom'

export type ChannelSummary = {
  /** Stable channel identifier — the future channels.id. Encodes kind so
   *  two channels named `forgejo` never collapse into one. */
  id: string
  /** Rename-able display label (the future channels.name) — NOT unique. */
  name: string
  displayName: string
  /** One line of intent (the future channels.description). */
  description: string
  /** What the channel is — decides provisioning, deletion, and which
   *  config panels mount. */
  kind: ChannelKind
  /** Registry id of the owning entity (agent/connector) when resolvable —
   *  used to mount that entity's configuration panels. */
  entityId?: string
}

function idFor(kind: ChannelKind, name: string): string {
  return `${kind}:${name}`
}

export function buildChannelSummaries(
  agents: { id: string; name: string }[] | undefined,
  connectors: { id: string; name: string }[] | undefined,
  customs: { id: string; name: string; description: string }[] | undefined,
): ChannelSummary[] {
  const channels: ChannelSummary[] = []
  for (const c of connectors ?? []) {
    channels.push({
      id: idFor('connector', c.name),
      name: c.name,
      displayName: c.name,
      description: `Where ${c.name} events arrive`,
      kind: 'connector',
      entityId: c.id,
    })
  }
  for (const a of agents ?? []) {
    channels.push({
      id: idFor('agent', a.name),
      name: a.name,
      displayName: a.name,
      description: `The inbox agent ${a.name} drains`,
      kind: 'agent',
      entityId: a.id,
    })
  }
  for (const u of customs ?? []) {
    channels.push({
      id: u.id,
      name: u.name,
      displayName: u.name,
      description: u.description,
      kind: 'custom',
    })
  }
  return channels.sort(
    (a, b) => a.name.localeCompare(b.name) || a.id.localeCompare(b.id),
  )
}

/** Aggregated channel list: one per connector, one per agent, plus the
 *  user's custom channels. */
export function useChannels() {
  const agents = useAgents({ pageSize: 500 })
  const connectors = useConnectors({ pageSize: 500 })
  const customs = useCustomChannels()
  const channels = buildChannelSummaries(
    agents.data?.items,
    connectors.data?.items,
    customs.data?.channels,
  )
  return {
    channels,
    isLoading: agents.isLoading || connectors.isLoading,
    error: agents.error ?? connectors.error,
  }
}

/**
 * A real connection between two channels: a subscription on the destination
 * transfers events from the source. Derived from triggers' registry ids —
 * in the target model these are channel FKs — plus the user's local bridges
 * (preview edges on custom channels, `source: 'local'`).
 */
export type ChannelConnection = {
  /** The subscription (or bridge) that performs the transfer. */
  subscription: string
  source: 'hub' | 'local'
  /** Stable id for local edges (removable); hub edges key off the trigger. */
  edgeId?: string
  from?: ChannelSummary
  to?: ChannelSummary
  /** Channel ids even when the channel cannot be resolved (deleted
   *  connector/agent/custom) — consumers filter on these. */
  fromRef?: string
  toRef?: string
}

export function buildConnections(
  channels: ChannelSummary[],
  triggers: TriggerSummary[] | undefined,
  bridges: ChannelBridge[] | undefined,
): ChannelConnection[] {
  const byId = new Map(channels.map((c) => [c.id, c]))
  const byEntity = new Map(
    channels
      .filter((c) => c.entityId !== undefined)
      .map((c) => [`${c.kind}#${c.entityId}`, c]),
  )
  const hub: ChannelConnection[] = (triggers ?? []).map((t) => ({
    subscription: t.name,
    source: 'hub' as const,
    from: t.connectorRef ? byEntity.get(`connector#${t.connectorRef}`) : undefined,
    to: t.agentRef ? byEntity.get(`agent#${t.agentRef}`) : undefined,
    fromRef: t.connectorRef ? `connector#${t.connectorRef}` : undefined,
    toRef: t.agentRef ? `agent#${t.agentRef}` : undefined,
  }))
  const local: ChannelConnection[] = (bridges ?? []).map((b) => ({
    subscription: b.name,
    source: 'local' as const,
    edgeId: b.id,
    from: byId.get(b.from),
    to: byId.get(b.to),
    fromRef: b.from,
    toRef: b.to,
  }))
  return [...hub, ...local]
}

/** A custom channel may be deleted only while no bridge attaches to it;
 *  connector and agent channels are provisioned with their entity and can
 *  never be deleted here. */
export function isChannelDeletable(
  channel: ChannelSummary,
  connections: ChannelConnection[],
): boolean {
  if (channel.kind !== 'custom') return false
  return !connections.some((e) => e.fromRef === channel.id || e.toRef === channel.id)
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
 * where events arrive, `agent` for what reached the inbox). Custom channels
 * hold no events yet — the hub does not know about them.
 */
export function useChannelCounts(channel: ChannelSummary) {
  const isAgentInbox = channel.kind === 'agent'
  const isCustom = channel.kind === 'custom'
  const scope = isAgentInbox ? { agent: channel.name } : { connector: channel.name }
  const since = since24h()

  const events = useQuery({
    queryKey: ['channels', 'counts', channel.id, 'events', scope, since],
    queryFn: () => listEventsPage({ ...scope, since, limit: 1 }),
    enabled: !isCustom,
  })
  const unmatched = useQuery({
    queryKey: ['channels', 'counts', channel.id, 'unmatched', scope, since],
    queryFn: () => listEventsPage({ ...scope, since, status: 'unmatched', limit: 1 }),
    enabled: !isCustom && !isAgentInbox,
  })
  const failed = useQuery({
    queryKey: ['channels', 'counts', channel.id, 'failed', scope, since],
    queryFn: () => listEventsPage({ ...scope, since, status: 'error', limit: 1 }),
    enabled: !isCustom,
  })

  if (isCustom) {
    return {
      events24h: 0,
      unmatched24h: 0,
      failed24h: 0,
      isLoading: false,
      isCustom: true as const,
    }
  }
  return {
    events24h: events.data?.total ?? 0,
    unmatched24h: unmatched.data?.total ?? 0,
    failed24h: failed.data?.total ?? 0,
    isLoading: events.isLoading || failed.isLoading || (!isAgentInbox && unmatched.isLoading),
    isCustom: false as const,
  }
}

/** Route for a channel — kind-namespaced so same-labeled channels never
 *  collapse (the target model addresses channels by id; this is the derived
 *  stand-in). */
export function channelPath(id: string): string {
  const sep = id.indexOf(':')
  const kind = sep >= 0 ? id.slice(0, sep) : 'connector'
  const name = sep >= 0 ? id.slice(sep + 1) : id
  return `/channels/${kind}/${encodeURIComponent(name)}`
}

/** Channel id an event was born in, given its connector string. Direct
 *  sources (cron/chat) have no channel — guard with isDirectSource(). */
export function channelIdForProducer(producer: string): string {
  return `connector:${producer}`
}
