import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo } from 'react'
import { request } from './client'
import type { ActivityEntry, EventsPage } from './events'

/**
 * Channels are the named streams events live in. The hub owns them, so
 * everything here reads one API instead of reconstructing the picture from
 * the agent, connector and trigger registries.
 *
 * Three kinds exist:
 *
 * - **connector** — where a connector's events arrive. One per connector,
 *   provisioned from the connector registry, never deletable: the connector
 *   has to put its events somewhere.
 * - **agent** — the agent's inbox. Everything born in it is prompted to the
 *   agent. One per agent, provisioned from the agent registry, never
 *   deletable: it is the agent.
 * - **custom** — user-created grouping channels. Deletable while nothing is
 *   attached.
 *
 * Identity is the channel id, never the name: a connector `forgejo` and an
 * agent `forgejo` are two distinct channels that merely share a display
 * label. Every link, subscription and route uses the id.
 *
 * Subscriptions come in two flavours and are read together but stored
 * separately: `trigger` edges are the hub's existing connector → agent
 * routing rules (still the source of truth for that routing, never copied
 * into a bridge), while `bridge` edges are the transfers a user attached
 * between channels.
 */

export type ChannelKind = 'connector' | 'agent' | 'custom'

export type ChannelStatus = 'matched' | 'unmatched' | 'error'

export type ChannelCounts = {
  events: number
  unmatched: number
  failed: number
}

export type Channel = {
  id: string
  kind: ChannelKind
  name: string
  description: string
  /** Registry id of the owning entity (connector/agent CR) for provisioned
   *  channels; empty on custom channels. */
  entityRef?: string
  /** True when the owning entity is gone but the channel is kept so its
   *  event history stays reachable. */
  orphaned?: boolean
  createdAt?: string
  updatedAt?: string
}

/** A channel plus the numbers the list table renders. */
export type ChannelView = Channel & {
  counts: ChannelCounts
  bridges: number
  subscriptions: number
}

export type ChannelSubscriptionSource = 'trigger' | 'bridge'

export type ChannelSubscription = {
  source: ChannelSubscriptionSource
  /** Trigger name or bridge id — what identifies the edge itself. */
  refId: string
  name: string
  fromChannel: string
  toChannel: string
  /** Registry ref when the endpoint is an entity rather than a channel. */
  fromRef?: string
  toRef?: string
  /** Endpoint labels resolved at read time, so a dangling edge still renders. */
  fromName?: string
  toName?: string
  fromKind?: ChannelKind
  toKind?: ChannelKind
}

export type ChannelDetail = ChannelView & {
  incoming: ChannelSubscription[]
  outgoing: ChannelSubscription[]
}

export type ChannelPage = {
  items: ChannelView[]
  total: number
  page: number
  pageSize: number
  totalPages: number
  /** Duration string the rate counts cover, e.g. "24h0m0s". */
  window: string
}

export type SubscriptionPage = {
  items: ChannelSubscription[]
  total: number
}

export type NewChannelInput = {
  name: string
  description?: string
  groupId?: string
}

export type ChannelListParams = {
  kind?: ChannelKind
  page?: number
  pageSize?: number
  /** RFC 3339 lower bound for the rate counts. Defaults to 24 hours ago. */
  since?: string
}

export type ChannelEventParams = {
  limit?: number
  offset?: number
  status?: ChannelStatus
  agent?: string
  subject?: string
  since?: string
}

const KEY = ['channels'] as const

// ---------------------------------------------------------------------------
// Queries
// ---------------------------------------------------------------------------

export function useChannels(params: ChannelListParams = {}) {
  return useQuery({
    queryKey: [...KEY, 'list', params],
    queryFn: () =>
      request<ChannelPage>('/channels', {
        query: {
          kind: params.kind,
          page: params.page,
          pageSize: params.pageSize ?? 500,
          since: params.since,
        },
      }),
  })
}

export function useChannel(id: string | undefined) {
  return useQuery({
    queryKey: [...KEY, 'get', id],
    queryFn: () => request<ChannelDetail>(`/channels/${encodeURIComponent(id ?? '')}`),
    enabled: !!id,
  })
}

/** Every subscription in the system, both registries merged. */
export function useChannelSubscriptions() {
  return useQuery({
    queryKey: [...KEY, 'subscriptions'],
    queryFn: () => request<SubscriptionPage>('/channel-subscriptions'),
  })
}

/** One channel's timeline: events born in it plus events transferred into it. */
export function useChannelEvents(
  id: string | undefined,
  params: ChannelEventParams = {},
  opts: { enabled?: boolean } = {},
) {
  const enabled = (opts.enabled ?? true) && !!id
  return useQuery({
    queryKey: [...KEY, 'events', id, params],
    queryFn: () =>
      request<EventsPage>(`/channels/${encodeURIComponent(id ?? '')}/events`, {
        query: { ...params },
      }),
    enabled,
  })
}

// ---------------------------------------------------------------------------
// Mutations
// ---------------------------------------------------------------------------

export function useCreateChannel() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: NewChannelInput) =>
      request<Channel>('/channels', { method: 'POST', body: input }),
    onSuccess: () => qc.invalidateQueries({ queryKey: KEY }),
  })
}

export function useDeleteChannel() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      request<void>(`/channels/${encodeURIComponent(id)}`, { method: 'DELETE' }),
    onSuccess: () => qc.invalidateQueries({ queryKey: KEY }),
  })
}

/** Attach a transfer: events arriving in `fromChannel` are also delivered to
 *  `toChannel`. At least one end must be a custom channel — a plain
 *  connector → agent pairing is a trigger, not a bridge. */
export function useAttachBridge() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ from, to, name }: { from: string; to: string; name?: string }) =>
      request<{ id: string }>(`/channels/${encodeURIComponent(from)}/bridges`, {
        method: 'POST',
        body: { to, name },
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: KEY }),
  })
}

export function useDetachBridge() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ from, bridgeId }: { from: string; bridgeId: string }) =>
      request<void>(
        `/channels/${encodeURIComponent(from)}/bridges/${encodeURIComponent(bridgeId)}`,
        { method: 'DELETE' },
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: KEY }),
  })
}

// ---------------------------------------------------------------------------
// Derived view models
// ---------------------------------------------------------------------------

/**
 * A rendered connection between two channels. `bridge` edges are removable
 * here (`edgeId` is the bridge id); `trigger` edges are managed on the
 * trigger screens, so the row links there instead.
 */
export type ChannelConnection = {
  subscription: string
  source: ChannelSubscriptionSource
  /** Bridge id — only present on bridge edges, which are the removable ones. */
  edgeId?: string
  from?: Channel
  to?: Channel
  /** Channel ids even when the channel is not in the current page. */
  fromRef?: string
  toRef?: string
}

export function buildConnections(
  subscriptions: ChannelSubscription[] | undefined,
  channels: Channel[] | undefined,
): ChannelConnection[] {
  const byId = new Map<string, Channel>((channels ?? []).map((c) => [c.id, c]))
  return (subscriptions ?? []).map((s) => {
    const from = byId.get(s.fromChannel)
    const to = byId.get(s.toChannel)
    return {
      subscription: s.name,
      source: s.source,
      edgeId: s.source === 'bridge' ? s.refId : undefined,
      from,
      to,
      // A trigger edge whose connector publishes to no channel at all has no
      // fromChannel: the label it matches on is still worth showing.
      fromRef: s.fromChannel || (s.fromRef ? `${s.source}:${s.fromRef}` : undefined),
      toRef: s.toChannel || (s.toRef ? `agent:${s.toRef}` : undefined),
    }
  })
}

/** Only custom channels are deletable, and only while nothing is attached:
 *  a stream cannot disappear underneath the subscriptions that use it. */
export function isChannelDeletable(channel: Channel, connections: ChannelConnection[]): boolean {
  if (channel.kind !== 'custom') return false
  return !connections.some((e) => e.fromRef === channel.id || e.toRef === channel.id)
}

/**
 * Producer labels for events that skip a channel: a scheduled tick and a chat
 * message are born directly in the target agent's inbox. The hub stamps the
 * real channel id on new events, so this only covers history and the label a
 * journey row falls back to when nothing matched.
 */
export const DIRECT_SOURCES = ['cron', 'chat'] as const

export function isDirectSource(producer: string | undefined): boolean {
  return (DIRECT_SOURCES as readonly string[]).includes(producer ?? '')
}

/** Route for a channel — addressed by id, since labels collide. */
export function channelPath(id: string): string {
  return `/channels/${encodeURIComponent(id)}`
}

/** Best available link for an event's birth channel: the stamped id when the
 *  hub recorded one, otherwise — only when the caller passes the channel list
 *  it already has — the connector channel that owns the producer label.
 *  History predating the registry has no id on the event. Direct sources
 *  (cron/chat) are born in an inbox, not a connector channel, so they never
 *  link to one. */
export function birthChannelPath(
  entry: Pick<ActivityEntry, 'channelId' | 'connector'>,
  channels?: Channel[],
): string | undefined {
  if (entry.channelId) return channelPath(entry.channelId)
  if (!entry.connector || isDirectSource(entry.connector) || !channels) return undefined
  const owned = channels.find((c) => c.kind === 'connector' && c.entityRef === entry.connector)
  return owned ? channelPath(owned.id) : undefined
}

/** Sort helper for the channel table: provisioned streams first. */
export function sortChannels(channels: Channel[]): Channel[] {
  return [...channels].sort((a, b) => a.name.localeCompare(b.name) || a.id.localeCompare(b.id))
}

/**
 * Label → channel id lookup for views that only carry a name. The activity
 * feed reports which *agent* received an event, not which inbox channel the
 * task landed in, and history predating the registry has no id on the event
 * at all. Resolution is per kind, because a connector and an agent may share
 * a label — that is the whole reason identity is the id.
 */
export function useChannelIds() {
  const { data, isLoading } = useChannels({ pageSize: 500 })
  const ids = useMemo(() => {
    const byLabel = new Map<string, string>()
    for (const c of data?.items ?? []) {
      // The feed names an agent by its registry ref, the UI by its display
      // name; a provisioned channel answers to both.
      byLabel.set(`${c.kind}\u0000${c.name}`, c.id)
      if (c.entityRef) byLabel.set(`${c.kind}\u0000${c.entityRef}`, c.id)
    }
    return {
      inbox: (agentName?: string) =>
        agentName ? byLabel.get(`agent\u0000${agentName}`) : undefined,
      home: (connectorLabel?: string) =>
        connectorLabel ? byLabel.get(`connector\u0000${connectorLabel}`) : undefined,
    }
  }, [data])
  return { ...ids, isLoading }
}

/** Look up a channel by id within an already-fetched list. */
export function findChannel(
  channels: Channel[] | undefined,
  id: string | undefined,
): Channel | undefined {
  if (!id) return undefined
  return channels?.find((c) => c.id === id)
}

/**
 * The relation graph of one channel, as a Mermaid flowchart definition.
 *
 * The shape is deliberate: at most one level UP (whatever feeds this channel),
 * and from the channel itself only DOWNSTREAM edges, explored breadth-first to
 * a bounded depth. The old view walked every edge in both directions at every
 * level, which read as noise — "who feeds me, and where does my traffic go"
 * was the same flat list of arrows.
 *
 * Node keys (A0, A1, …) map back to channel ids in `nodeChannels`; endpoints
 * the channel list couldn't resolve render as dashed ghost nodes labelled with
 * their raw ref, so a dangling edge is visible but unclickable.
 */
export type RelationEndpoint = { id?: string; name: string; edge: string }

export type RelationGraphModel = {
  definition: string
  /** Mermaid node key → channel id, for click-through navigation. */
  nodeChannels: Record<string, string>
  /** First tier, for the text summary (and the diagram's absence). */
  fedBy: RelationEndpoint[]
  feeds: RelationEndpoint[]
  empty: boolean
}

/** How many downstream levels the graph expands below the root channel. */
export const RELATION_DOWN_DEPTH = 2

export function buildRelationGraph(
  root: { id: string; name: string },
  edges: ChannelConnection[],
  downDepth = RELATION_DOWN_DEPTH,
): RelationGraphModel {
  const clean = (s: string) => s.replace(/["\n\r]/g, ' ').trim() || '—'
  const fromIdent = (e: ChannelConnection) => e.from?.id ?? e.fromRef
  const toIdent = (e: ChannelConnection) => e.to?.id ?? e.toRef

  const keys = new Map<string, string>()
  const nodeDefs = new Map<string, { text: string; ghost: boolean }>()
  const nodeChannels: Record<string, string> = {}
  const ghostKeys: string[] = []
  let seq = 0
  const addNode = (
    ident: string | undefined,
    text: string,
    ghost: boolean,
    chanId?: string,
  ): string => {
    const slot = ident ?? '~unknown'
    const known = keys.get(slot)
    if (known) return known
    const key = `A${seq++}`
    keys.set(slot, key)
    nodeDefs.set(key, { text: clean(text), ghost })
    if (!ghost && chanId) nodeChannels[key] = chanId
    if (ghost) ghostKeys.push(key)
    return key
  }

  const edgeLines: string[] = []
  const seenEdges = new Set<string>()
  const addEdge = (fromKey: string, toKey: string, e: ChannelConnection) => {
    const text = e.source === 'bridge' ? `bridged · ${e.subscription}` : `via ${e.subscription}`
    const slot = `${fromKey}>${toKey}>${text}`
    if (seenEdges.has(slot)) return
    seenEdges.add(slot)
    edgeLines.push(`  ${fromKey} -->|"${clean(text)}"| ${toKey}`)
    return text
  }

  const rootKey = addNode(root.id, root.name, false, root.id)
  const fedBy: RelationEndpoint[] = []
  const feeds: RelationEndpoint[] = []

  // One level up: whoever feeds this channel, without exploring their context.
  for (const e of edges) {
    if (toIdent(e) !== root.id) continue
    const key = addNode(fromIdent(e), e.from?.name ?? e.fromRef ?? 'unknown', !e.from, e.from?.id)
    const text = addEdge(key, rootKey, e)
    fedBy.push({
      id: e.from?.id,
      name: e.from?.name ?? e.fromRef ?? 'unknown',
      edge: text ?? '',
    })
  }

  // Downstream only: children, grandchildren, … to the depth budget. A child
  // already on the graph (a cycle, or shared parent) gets its edge but is not
  // expanded again — the visited set is what keeps a cyclic graph finite.
  const visited = new Set<string>([root.id])
  let frontier = [{ id: root.id, key: rootKey }]
  for (let depth = 0; depth < downDepth && frontier.length > 0; depth++) {
    const next: { id: string; key: string }[] = []
    for (const node of frontier) {
      for (const e of edges) {
        if (fromIdent(e) !== node.id) continue
        const ghost = !e.to
        const key = addNode(toIdent(e), e.to?.name ?? e.toRef ?? 'unknown', ghost, e.to?.id)
        const text = addEdge(node.key, key, e)
        if (depth === 0) {
          feeds.push({
            id: e.to?.id,
            name: e.to?.name ?? e.toRef ?? 'unknown',
            edge: text ?? '',
          })
        }
        if (e.to && !visited.has(e.to.id)) {
          visited.add(e.to.id)
          next.push({ id: e.to.id, key })
        }
      }
    }
    frontier = next
  }

  const lines = [
    'graph TD',
    ...[...nodeDefs.entries()].map(
      ([key, def]) => `  ${key}["${def.text}"]${def.ghost ? ':::relGhost' : ''}`,
    ),
    ...edgeLines,
    '  classDef relRoot stroke:#e0af68,stroke-width:2.5px',
    '  classDef relGhost stroke-dasharray:4 4,opacity:0.55',
    `  class ${rootKey} relRoot`,
  ]
  if (ghostKeys.length > 0) lines.push(`  class ${ghostKeys.join(',')} relGhost`)

  return {
    definition: lines.join('\n'),
    nodeChannels,
    fedBy,
    feeds,
    empty: nodeDefs.size === 1 && edgeLines.length === 0,
  }
}
