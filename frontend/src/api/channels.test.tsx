import { describe, expect, it, vi } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import {
  buildConnections,
  channelPath,
  isDirectSource,
  useChannels,
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

describe('useChannels', () => {
  it('lists connectors and agents as id-addressed channels without roles', async () => {
    stubLists(agentsPayload, connectorsPayload)
    try {
      const { result } = renderHook(() => useChannels(), { wrapper })
      await waitFor(() => expect(result.current.isLoading).toBe(false))
      const ids = result.current.channels.map((c) => c.id)
      // no built-in cron/chat channels; one channel per connector and agent
      expect(ids).toEqual(['connector:forgejo', 'agent:review-bot'])
      const forgejo = result.current.channels.find((c) => c.id === 'connector:forgejo')
      expect(forgejo?.description).toContain('forgejo')
      expect(forgejo?.entityId).toBe('c1')
      expect(forgejo?.origin).toBe('connector')
      const bot = result.current.channels.find((c) => c.id === 'agent:review-bot')
      expect(bot?.description).toContain('review-bot')
      expect(bot?.entityId).toBe('a1')
      expect(bot?.origin).toBe('agent')
      // the summary type carries no produces/consumes role at all
      expect('roles' in (forgejo ?? {})).toBe(false)
    } finally {
      vi.unstubAllGlobals()
    }
  })

  it('keeps same-named channels distinct (connector and agent are two channels)', async () => {
    stubLists(
      { items: [{ id: 'a9', name: 'forgejo' }], total: 1 },
      { items: [{ id: 'c9', name: 'forgejo' }], total: 1 },
    )
    try {
      const { result } = renderHook(() => useChannels(), { wrapper })
      await waitFor(() => expect(result.current.isLoading).toBe(false))
      const forgejos = result.current.channels.filter((c) => c.name === 'forgejo')
      expect(forgejos.map((c) => c.id).sort()).toEqual(['agent:forgejo', 'connector:forgejo'])
    } finally {
      vi.unstubAllGlobals()
    }
  })
})

describe('buildConnections', () => {
  it('maps subscriptions to channel-to-channel edges', async () => {
    stubLists(agentsPayload, connectorsPayload)
    try {
      const { result } = renderHook(() => useChannels(), { wrapper })
      await waitFor(() => expect(result.current.isLoading).toBe(false))
      const conns = buildConnections(result.current.channels, [
        { id: 't1', name: 'on-issues', agentRef: 'a1', connectorRef: 'c1', filters: [] },
        { id: 't2', name: 'ghost', agentRef: 'gone', connectorRef: 'cX', filters: [] },
      ])
      expect(conns[0].from?.id).toBe('connector:forgejo')
      expect(conns[0].to?.id).toBe('agent:review-bot')
      expect(conns[0].subscription).toBe('on-issues')
      // unresolvable endpoints keep their registry refs
      expect(conns[1].from).toBeUndefined()
      expect(conns[1].fromRef).toBe('cX')
      expect(conns[1].to).toBeUndefined()
      expect(conns[1].toRef).toBe('gone')
    } finally {
      vi.unstubAllGlobals()
    }
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
  it('namespaces routes by origin', () => {
    expect(channelPath('connector:forgejo')).toBe('/channels/connector/forgejo')
    expect(channelPath('agent:my agent')).toBe('/channels/agent/my%20agent')
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
