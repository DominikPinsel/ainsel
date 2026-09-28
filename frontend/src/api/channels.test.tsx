import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import {
  birthChannelPath,
  buildConnections,
  buildRelationGraph,
  channelPath,
  isChannelDeletable,
  isDirectSource,
  useChannel,
  useChannelIds,
  useChannels,
  type Channel,
  type ChannelSubscription,
  type ChannelView,
} from './channels'
import { useInvocations } from './invocations'

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
}

const channelPage = {
  items: [
    {
      id: 'ch-forgejo',
      kind: 'connector',
      name: 'forgejo',
      description: 'Events published by connector "forgejo"',
      entityRef: 'c1',
      counts: { events: 12, unmatched: 3, failed: 1 },
      bridges: 0,
      subscriptions: 2,
    },
    {
      id: 'ch-inbox',
      kind: 'agent',
      name: 'forgejo',
      description: 'Inbox channel for agent "forgejo"',
      entityRef: 'a9',
      counts: { events: 9, unmatched: 0, failed: 0 },
      bridges: 1,
      subscriptions: 3,
    },
    {
      id: 'ch-group',
      kind: 'custom',
      name: 'team-inbox',
      description: 'grouped',
      counts: { events: 0, unmatched: 0, failed: 0 },
      bridges: 1,
      subscriptions: 1,
    },
  ],
  total: 3,
  page: 1,
  pageSize: 500,
  totalPages: 1,
  window: '24h0m0s',
}

function stubChannels(payload: unknown = channelPage) {
  const fetchMock = vi.fn((url: string) => {
    if (String(url).includes('/channels/ch-group')) {
      return Promise.resolve(
        new Response(
          JSON.stringify({
            ...channelPage.items[2],
            counts: channelPage.items[2].counts,
            incoming: [],
            outgoing: [],
          }),
          { status: 200 },
        ),
      )
    }
    return Promise.resolve(new Response(JSON.stringify(payload), { status: 200 }))
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('useChannels', () => {
  it('reads the hub registry rather than synthesising from other lists', async () => {
    const fetchMock = stubChannels()
    const { result } = renderHook(() => useChannels({ pageSize: 500 }), { wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(String(fetchMock.mock.calls[0][0])).toContain('/api/v1/channels?')
    expect(String(fetchMock.mock.calls[0][0])).toContain('pageSize=500')
    // One request: the hub already returns kinds, counts and subscription totals.
    expect(fetchMock).toHaveBeenCalledTimes(1)

    const ids = result.current.data!.items.map((c) => c.id)
    expect(ids).toEqual(['ch-forgejo', 'ch-inbox', 'ch-group'])
    const home = result.current.data!.items[0]
    expect(home.kind).toBe('connector')
    expect(home.entityRef).toBe('c1')
    expect(home.counts.unmatched).toBe(3)
  })

  it('keeps same-labeled channels distinct: a connector and an agent both named forgejo', async () => {
    stubChannels()
    const { result } = renderHook(() => useChannels(), { wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    const sameName = result.current.data!.items.filter((c) => c.name === 'forgejo')
    expect(sameName.map((c) => c.id).sort()).toEqual(['ch-forgejo', 'ch-inbox'])
    expect(new Set(sameName.map((c) => c.kind)).size).toBe(2)
  })

  it('forwards the kind filter to the hub', async () => {
    const fetchMock = stubChannels()
    renderHook(() => useChannels({ kind: 'agent' }), { wrapper })
    await waitFor(() => expect(fetchMock).toHaveBeenCalled())
    expect(String(fetchMock.mock.calls[0][0])).toContain('kind=agent')
  })
})

describe('useChannel', () => {
  it('fetches one channel by id with its subscriptions', async () => {
    const fetchMock = stubChannels()
    const { result } = renderHook(() => useChannel('ch-group'), { wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(String(fetchMock.mock.calls[0][0])).toContain('/api/v1/channels/ch-group')
    expect(result.current.data!.kind).toBe('custom')
    expect(result.current.data!.incoming).toEqual([])
  })
})

describe('buildConnections', () => {
  const view = (
    over: Partial<ChannelView> & Pick<Channel, 'id' | 'kind' | 'name'>,
  ): ChannelView => ({
    description: '',
    counts: { events: 0, unmatched: 0, failed: 0 },
    bridges: 0,
    subscriptions: 0,
    ...over,
  })

  const channels = [
    view({ id: 'ch-forgejo', kind: 'connector', name: 'forgejo', entityRef: 'c1' }),
    view({ id: 'ch-inbox', kind: 'agent', name: 'forgejo', entityRef: 'a9' }),
    view({ id: 'ch-group', kind: 'custom', name: 'team-inbox' }),
  ]

  it('resolves trigger and bridge edges onto their channels', () => {
    const subs: ChannelSubscription[] = [
      {
        source: 'trigger',
        refId: 'on-issues',
        name: 'on-issues',
        fromChannel: 'ch-forgejo',
        toChannel: 'ch-inbox',
      },
      {
        source: 'bridge',
        refId: 'br-1',
        name: 'all-issues',
        fromChannel: 'ch-forgejo',
        toChannel: 'ch-group',
      },
    ]
    const conns = buildConnections(subs, channels)
    expect(conns[0].from?.id).toBe('ch-forgejo')
    expect(conns[0].to?.id).toBe('ch-inbox')
    expect(conns[0].source).toBe('trigger')
    // Only bridges are removable here; a trigger edge has no bridge id.
    expect(conns[0].edgeId).toBeUndefined()
    expect(conns[1].edgeId).toBe('br-1')
    expect(conns[1].from?.name).toBe('forgejo')
  })

  it('keeps dangling endpoints as refs so the row still renders', () => {
    const subs: ChannelSubscription[] = [
      {
        source: 'trigger',
        refId: 'ghost',
        name: 'ghost',
        fromChannel: '',
        toChannel: '',
        fromRef: 'cX',
        toRef: 'gone',
      },
    ]
    const conns = buildConnections(subs, channels)
    expect(conns[0].from).toBeUndefined()
    expect(conns[0].fromRef).toBe('trigger:cX')
    expect(conns[0].toRef).toBe('agent:gone')
  })

  it('drops nothing when the channel list is missing entries', () => {
    const subs: ChannelSubscription[] = [
      { source: 'bridge', refId: 'br-9', name: 'x', fromChannel: 'ch-gone', toChannel: 'ch-group' },
    ]
    const conns = buildConnections(subs, channels)
    expect(conns).toHaveLength(1)
    expect(conns[0].from).toBeUndefined()
    expect(conns[0].fromRef).toBe('ch-gone')
    expect(conns[0].to?.name).toBe('team-inbox')
  })
})

describe('isChannelDeletable', () => {
  const mk = (over: Partial<Channel> & Pick<Channel, 'id' | 'kind' | 'name'>): Channel => ({
    description: '',
    ...over,
  })

  it('only custom channels, and only while nothing subscribes through them', () => {
    const connector = mk({ id: 'ch-forgejo', kind: 'connector', name: 'forgejo' })
    const agent = mk({ id: 'ch-inbox', kind: 'agent', name: 'forgejo' })
    const custom = mk({ id: 'ch-group', kind: 'custom', name: 'team-inbox' })
    const conns = buildConnections(
      [
        {
          source: 'bridge',
          refId: 'br-1',
          name: 'x',
          fromChannel: 'ch-forgejo',
          toChannel: 'ch-group',
        },
      ],
      [connector, agent, custom],
    )
    expect(isChannelDeletable(connector, conns)).toBe(false)
    expect(isChannelDeletable(agent, conns)).toBe(false)
    // referenced by a bridge → stays
    expect(isChannelDeletable(custom, conns)).toBe(false)
    // bridge removed → deletable again
    expect(isChannelDeletable(custom, [])).toBe(true)
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
  it('addresses channels by id', () => {
    expect(channelPath('ch-forgejo')).toBe('/channels/ch-forgejo')
    expect(channelPath('ch my agent')).toBe('/channels/ch%20my%20agent')
  })
})

describe('birthChannelPath', () => {
  const channels = [
    {
      id: 'ch-forgejo',
      kind: 'connector' as const,
      // The display name is rename-able; entityRef is the label events carry.
      name: 'Forgejo',
      description: '',
      entityRef: 'forgejo',
    },
  ]

  it('links the id the hub stamped on the event', () => {
    expect(birthChannelPath({ channelId: 'ch-group', connector: 'forgejo' })).toBe(
      '/channels/ch-group',
    )
  })

  it('falls back to the connector channel that owns the label when the record has no id', () => {
    expect(birthChannelPath({ connector: 'forgejo' }, channels)).toBe('/channels/ch-forgejo')
  })

  it('never guesses a channel for a direct source or an unknown label', () => {
    expect(birthChannelPath({ connector: 'cron' }, channels)).toBeUndefined()
    expect(birthChannelPath({ connector: 'chat' }, channels)).toBeUndefined()
    expect(birthChannelPath({ connector: 'slack' }, channels)).toBeUndefined()
    expect(birthChannelPath({ connector: undefined }, channels)).toBeUndefined()
    // Without a channel list to resolve against, a label alone is not a link.
    expect(birthChannelPath({ connector: 'forgejo' })).toBeUndefined()
  })
})

describe('useChannelIds', () => {
  it('resolves labels per kind so a shared name cannot collapse', async () => {
    stubChannels()
    const { result } = renderHook(() => useChannelIds(), { wrapper })
    await waitFor(() => expect(result.current.inbox('forgejo')).toBe('ch-inbox'))
    expect(result.current.home('forgejo')).toBe('ch-forgejo')
    expect(result.current.inbox('nobody')).toBeUndefined()
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

describe('buildRelationGraph', () => {
  const ch = (id: string, name: string, kind: 'connector' | 'agent' | 'custom' = 'agent') =>
    ({ id, name, kind }) as Channel
  const conn = (
    from: Channel | undefined,
    to: Channel | undefined,
    subscription: string,
    source: 'trigger' | 'bridge' = 'trigger',
  ) => ({
    subscription,
    source,
    from,
    to,
    fromRef: from?.id,
    toRef: to?.id,
  })

  it('shows parents one level up and never explores their context', () => {
    const forgejo = ch('ch-1', 'connector-forgejo', 'connector')
    const inbox = ch('ch-2', 'agent-reviewer')
    const group = ch('ch-3', 'team-inbox', 'custom')
    const model = buildRelationGraph({ id: 'ch-3', name: 'team-inbox' }, [
      conn(forgejo, inbox, 'trigger-dev-mention'),
      conn(inbox, group, 'all-issues', 'bridge'),
    ])
    expect(model.definition).toContain('agent-reviewer')
    // the forgejo channel feeds the parent but is NOT part of this view
    expect(model.definition).not.toContain('connector-forgejo')
    expect(model.fedBy).toEqual([
      { id: 'ch-2', name: 'agent-reviewer', edge: 'bridged · all-issues' },
    ])
    expect(model.feeds).toHaveLength(0)
  })

  it('walks downstream only, within the depth budget', () => {
    const a = ch('ch-a', 'A', 'connector')
    const b = ch('ch-b', 'B')
    const c = ch('ch-c', 'C', 'custom')
    const d = ch('ch-d', 'D', 'custom')
    const model = buildRelationGraph({ id: 'ch-a', name: 'A' }, [
      conn(a, b, 't1'),
      conn(b, c, 'br1', 'bridge'),
      conn(c, d, 'br2', 'bridge'),
    ])
    // depth 0 children and depth 1 grandchildren render; the great-grandchild
    // behind the budget does not.
    expect(model.definition).toContain('"B"')
    expect(model.definition).toContain('"C"')
    expect(model.definition).not.toContain('"D"')
    expect(model.feeds).toEqual([{ id: 'ch-b', name: 'B', edge: 'via t1' }])
  })

  it('deduplicates cycles instead of re-expanding them', () => {
    const a = ch('ch-a', 'A', 'connector')
    const b = ch('ch-b', 'B')
    const model = buildRelationGraph({ id: 'ch-a', name: 'A' }, [
      conn(a, b, 't1'),
      conn(b, a, 't2'),
    ])
    // one node per channel, and each edge exactly once
    expect(Object.keys(model.nodeChannels)).toHaveLength(2)
    expect(model.definition.match(/-->/g)).toHaveLength(2)
    expect(model.fedBy.map((f) => f.id)).toEqual(['ch-b'])
    expect(model.feeds.map((f) => f.id)).toEqual(['ch-b'])
  })

  it('renders unresolvable endpoints as ghost nodes without ids', () => {
    const inbox = ch('ch-2', 'agent-reviewer')
    const dangling = {
      subscription: 't-dangling',
      source: 'trigger' as const,
      from: undefined,
      to: inbox,
      fromRef: 'trigger:old-connector',
    }
    const model = buildRelationGraph({ id: 'ch-2', name: 'agent-reviewer' }, [dangling])
    expect(model.definition).toContain(':::relGhost')
    expect(model.definition).toContain('"trigger:old-connector"')
    expect(model.fedBy).toEqual([
      { id: undefined, name: 'trigger:old-connector', edge: 'via t-dangling' },
    ])
  })

  it('is empty for an unconnected channel', () => {
    const model = buildRelationGraph({ id: 'ch-x', name: 'lonely' }, [])
    expect(model.empty).toBe(true)
    expect(model.definition).not.toContain('-->')
    expect(model.definition).toContain('"lonely"')
  })
})
