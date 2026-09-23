import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import {
  buildConnections,
  channelPath,
  isChannelDeletable,
  isDirectSource,
  useChannels,
  type ChannelSummary,
} from './channels'
import { useInvocations } from './invocations'

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
}

const agentsPayload = {
  items: [{ id: 'a1', name: 'review-bot' }],
  total: 1,
  page: 1,
  pageSize: 500,
  totalPages: 1,
}

const connectorsPayload = {
  items: [{ id: 'c1', name: 'forgejo' }],
  total: 1,
  page: 1,
  pageSize: 500,
  totalPages: 1,
}

function stubLists(agents: unknown, connectors: unknown) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      if (url.includes('/agents')) {
        return Promise.resolve(new Response(JSON.stringify(agents), { status: 200 }))
      }
      if (url.includes('/connectors')) {
        return Promise.resolve(new Response(JSON.stringify(connectors), { status: 200 }))
      }
      return Promise.resolve(new Response('{}', { status: 200 }))
    }),
  )
}

const customChannel: ChannelSummary = {
  id: 'custom:team-inbox',
  name: 'team-inbox',
  displayName: 'team-inbox',
  description: 'Custom grouping channel for "team-inbox"',
  kind: 'custom',
}

describe('useChannels', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    localStorage.clear()
  })

  it('lists connectors and agents as id-addressed channels of their kind', async () => {
    stubLists(agentsPayload, connectorsPayload)
    const { result } = renderHook(() => useChannels(), { wrapper })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    const ids = result.current.channels.map((c) => c.id)
    // one channel per connector and agent, none deletable-by-kind
    expect(ids).toEqual(['connector:forgejo', 'agent:review-bot'])
    const forgejo = result.current.channels.find((c) => c.id === 'connector:forgejo')
    expect(forgejo?.description).toContain('forgejo')
    expect(forgejo?.entityId).toBe('c1')
    expect(forgejo?.kind).toBe('connector')
    const bot = result.current.channels.find((c) => c.id === 'agent:review-bot')
    expect(bot?.kind).toBe('agent')
    expect(bot?.entityId).toBe('a1')
  })

  it('merges browser-local custom channels into the registry', async () => {
    stubLists(agentsPayload, connectorsPayload)
    localStorage.setItem(
      'ainsel.customChannels.v1',
      JSON.stringify({
        channels: [
          { id: 'custom:team-inbox', name: 'team-inbox', description: 'grouped', createdAt: 'x' },
        ],
        bridges: [],
      }),
    )
    const { result } = renderHook(() => useChannels(), { wrapper })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    const team = result.current.channels.find((c) => c.id === 'custom:team-inbox')
    expect(team?.kind).toBe('custom')
    expect(team?.description).toBe('grouped')
    expect(team?.entityId).toBeUndefined()
  })

  it('keeps same-named channels distinct (connector and agent are two channels)', async () => {
    stubLists(
      { items: [{ id: 'a9', name: 'forgejo' }], total: 1 },
      { items: [{ id: 'c9', name: 'forgejo' }], total: 1 },
    )
    const { result } = renderHook(() => useChannels(), { wrapper })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    const forgejos = result.current.channels.filter((c) => c.name === 'forgejo')
    expect(forgejos.map((c) => c.id).sort()).toEqual(['agent:forgejo', 'connector:forgejo'])
  })
})

describe('buildConnections', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    localStorage.clear()
  })

  it('maps hub subscriptions to channel-to-channel edges', async () => {
    stubLists(agentsPayload, connectorsPayload)
    const { result } = renderHook(() => useChannels(), { wrapper })
    await waitFor(() => expect(result.current.isLoading).toBe(false))
    const conns = buildConnections(
      result.current.channels,
      [
        { id: 't1', name: 'on-issues', agentRef: 'a1', connectorRef: 'c1', filters: [] },
        { id: 't2', name: 'ghost', agentRef: 'gone', connectorRef: 'cX', filters: [] },
      ],
      [],
    )
    expect(conns[0].from?.id).toBe('connector:forgejo')
    expect(conns[0].to?.id).toBe('agent:review-bot')
    expect(conns[0].subscription).toBe('on-issues')
    expect(conns[0].source).toBe('hub')
    // unresolvable endpoints keep their registry refs
    expect(conns[1].from).toBeUndefined()
    expect(conns[1].fromRef).toBe('connector#cX')
    expect(conns[1].to).toBeUndefined()
    expect(conns[1].toRef).toBe('agent#gone')
  })

  it('adds local bridges as preview edges between any channels', () => {
    const channels: ChannelSummary[] = [
      {
        id: 'connector:forgejo',
        name: 'forgejo',
        displayName: 'forgejo',
        description: '',
        kind: 'connector' as const,
        entityId: 'c1',
      },
      customChannel,
    ]
    const conns = buildConnections(
      channels,
      [],
      [{ id: 'b1', from: 'connector:forgejo', to: 'custom:team-inbox', name: 'all-issues' }],
    )
    expect(conns).toHaveLength(1)
    expect(conns[0].source).toBe('local')
    expect(conns[0].edgeId).toBe('b1')
    expect(conns[0].from?.id).toBe('connector:forgejo')
    expect(conns[0].to?.id).toBe('custom:team-inbox')
  })
})

describe('isChannelDeletable', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('only custom channels can be deleted, and only while nothing bridges to them', () => {
    const connector: ChannelSummary = {
      id: 'connector:forgejo',
      name: 'forgejo',
      displayName: 'forgejo',
      description: '',
      kind: 'connector',
    }
    const agent: ChannelSummary = {
      id: 'agent:review-bot',
      name: 'review-bot',
      displayName: 'review-bot',
      description: '',
      kind: 'agent',
    }
    const conns = buildConnections(
      [connector, agent, customChannel],
      [],
      [{ id: 'b1', from: 'connector:forgejo', to: 'custom:team-inbox', name: 'x' }],
    )
    expect(isChannelDeletable(connector, conns)).toBe(false)
    expect(isChannelDeletable(agent, conns)).toBe(false)
    // referenced by a bridge → stays
    expect(isChannelDeletable(customChannel, conns)).toBe(false)
    // bridge removed → deletable again
    expect(isChannelDeletable(customChannel, [])).toBe(true)
  })
})

describe('isDirectSource', () => {
  it('treats the hub synthetic labels as direct births, not channels', () => {
    expect(isDirectSource('cron')).toBe(true)
    expect(isDirectSource('chat')).toBe(true)
    expect(isDirectSource('forgejo')).toBe(false)
    expect(isDirectSource(undefined)).toBe(false)
  })
})

describe('channelPath', () => {
  it('namespaces routes by kind', () => {
    expect(channelPath('connector:forgejo')).toBe('/channels/connector/forgejo')
    expect(channelPath('agent:my agent')).toBe('/channels/agent/my%20agent')
    expect(channelPath('custom:team-inbox')).toBe('/channels/custom/team-inbox')
  })
})

describe('useInvocations enabled option', () => {
  it('does not fetch when enabled is false', async () => {
    const fetchMock = vi.fn(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            invocations: [],
            total: 0,
            capacity: 10,
            page: 1,
            pageSize: 20,
            totalPages: 0,
          }),
          { status: 200 },
        ),
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    try {
      const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
      const { result } = renderHook(() => useInvocations({ agent: 'x' }, { enabled: false }), {
        wrapper: ({ children }: { children: ReactNode }) => (
          <QueryClientProvider client={qc}>{children}</QueryClientProvider>
        ),
      })
      await new Promise((r) => setTimeout(r, 20))
      expect(fetchMock).not.toHaveBeenCalled()
      expect(result.current.isFetched).toBe(false)
    } finally {
      vi.unstubAllGlobals()
    }
  })
})
