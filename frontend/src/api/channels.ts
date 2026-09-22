import { useQuery } from '@tanstack/react-query'
import { useAgents } from './agents'
import { useConnectors } from './connectors'
import { listEventsPage } from './events'

/**
 * Channels are the named destinations events flow through (see
 * docs/superpowers/specs — Channels design). The backend does not have a
 * channels entity yet, so the channel list is derived from the existing
 * registries: one channel per connector (its producing channel), one per
 * agent (its inbox), plus the two built-in channels `cron` and `chat`.
 */

export type ChannelOrigin = 'connector' | 'agent' | 'builtin'

export type ChannelRole = 'produces' | 'consumes'

export type ChannelSummary = {
  /** Stable, human-readable channel name (the future channels.name). */
  name: string
  displayName: string
  /** Where the channel comes from — presentation hint only. */
  origin: ChannelOrigin
  roles: ChannelRole[]
}

/** The built-in synthetic sources the hub publishes into channels. */
export const BUILTIN_CHANNELS: ChannelSummary[] = [
  { name: 'cron', displayName: 'Cron', origin: 'builtin', roles: ['produces'] },
  { name: 'chat', displayName: 'Chat', origin: 'builtin', roles: ['produces'] },
]

function buildChannelSummaries(
  agents: { name: string }[] | undefined,
  connectors: { name: string }[] | undefined,
): ChannelSummary[] {
  const channels = new Map<string, ChannelSummary>()
  for (const c of connectors ?? []) {
    channels.set(c.name, {
      name: c.name,
      displayName: c.name,
      origin: 'connector',
      roles: ['produces'],
    })
  }
  for (const a of agents ?? []) {
    const existing = channels.get(a.name)
    channels.set(a.name, {
      name: a.name,
      displayName: a.name,
      origin: 'agent',
      roles: existing ? Array.from(new Set([...existing.roles, 'consumes' as const])) : ['consumes'],
    })
  }
  for (const b of BUILTIN_CHANNELS) {
    if (!channels.has(b.name)) channels.set(b.name, b)
  }
  return Array.from(channels.values()).sort((a, b) => a.name.localeCompare(b.name))
}

/** Aggregated channel list: connectors + agents + built-ins, deduped. */
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
  const scope = channel.roles.includes('produces') ? { connector: channel.name } : { agent: channel.name }
  const since = since24h()

  const events = useQuery({
    queryKey: ['channels', 'counts', channel.name, 'events', scope, since],
    queryFn: () => listEventsPage({ ...scope, since, limit: 1 }),
  })
  const unmatched = useQuery({
    queryKey: ['channels', 'counts', channel.name, 'unmatched', scope, since],
    queryFn: () => listEventsPage({ ...scope, since, status: 'unmatched', limit: 1 }),
    enabled: channel.roles.includes('produces'),
  })
  const failed = useQuery({
    queryKey: ['channels', 'counts', channel.name, 'failed', scope, since],
    queryFn: () => listEventsPage({ ...scope, since, status: 'error', limit: 1 }),
  })

  return {
    events24h: events.data?.total ?? 0,
    unmatched24h: unmatched.data?.total ?? 0,
    failed24h: failed.data?.total ?? 0,
    isLoading: events.isLoading || failed.isLoading || (channel.roles.includes('produces') && unmatched.isLoading),
  }
}