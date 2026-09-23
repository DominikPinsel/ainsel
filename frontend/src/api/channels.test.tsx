import { describe, expect, it, vi } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { BUILTIN_CHANNELS, useChannels } from './channels'
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

describe('useChannels', () => {
  it('merges agents, connectors and built-ins into one deduped list', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/agents')) {
          return Promise.resolve(new Response(JSON.stringify(agentsPayload), { status: 200 }))
        }
        if (url.includes('/connectors')) {
          return Promise.resolve(new Response(JSON.stringify(connectorsPayload), { status: 200 }))
        }
        return Promise.resolve(new Response('{}', { status: 200 }))
      }),
    )
    try {
      const { result } = renderHook(() => useChannels(), { wrapper })
      await waitFor(() => expect(result.current.isLoading).toBe(false))
      const names = result.current.channels.map((c) => c.name)
      expect(names).toEqual(['chat', 'cron', 'forgejo', 'review-bot'])
      const forgejo = result.current.channels.find((c) => c.name === 'forgejo')
      expect(forgejo?.roles).toEqual(['produces'])
      const bot = result.current.channels.find((c) => c.name === 'review-bot')
      expect(bot?.roles).toEqual(['consumes'])
    } finally {
      vi.unstubAllGlobals()
    }
  })

  it('exposes the built-in channels', () => {
    expect(BUILTIN_CHANNELS.map((c) => c.name)).toEqual(['cron', 'chat'])
    for (const b of BUILTIN_CHANNELS) expect(b.roles).toContain('produces')
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