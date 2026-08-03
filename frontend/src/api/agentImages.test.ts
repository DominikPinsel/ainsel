import React from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { listAgentImages, useRefreshMCPTools } from './agentImages'

const mockFetch = () => globalThis.fetch as ReturnType<typeof vi.fn>

describe('api/agentImages', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn())
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('listAgentImages GETs /agent-images with pagination', async () => {
    mockFetch().mockResolvedValue(
      new Response(
        JSON.stringify({ items: [], total: 5, page: 1, pageSize: 20, totalPages: 1 }),
        { status: 200 },
      ),
    )
    await listAgentImages({ page: 1, pageSize: 20 })
    const [url] = mockFetch().mock.calls[0]
    expect(url).toContain('/agent-images')
    expect(url).toContain('pageSize=20')
  })
})

describe('useRefreshMCPTools', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn())
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('stores the response directly in the detail query cache so tools appear without a second fetch', async () => {
    const returnedImage = {
      id: 'img-1',
      imageURL: 'ghcr.io/org/image:latest',
      tools: [{ name: 'read_file', kind: 'mcp' as const, description: 'Read a file from MCP' }],
    }
    ;(globalThis.fetch as ReturnType<typeof vi.fn>).mockResolvedValue(
      new Response(JSON.stringify({ image: returnedImage, warnings: [] }), { status: 200 }),
    )

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: Infinity } } })
    const wrapper = ({ children }: { children: React.ReactNode }) =>
      React.createElement(QueryClientProvider, { client: qc }, children)

    const { result } = renderHook(() => useRefreshMCPTools(), { wrapper })

    result.current.mutate('img-1')

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(qc.getQueryData(['agent-images', 'detail', 'img-1'])).toEqual(returnedImage)
  })
})
