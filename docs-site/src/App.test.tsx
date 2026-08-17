import { describe, expect, it, vi, afterEach } from 'vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { App } from './App'

// DocsPage fetches docs/_sidebar.md and docs/<slug>.md at runtime. Stub the
// network with a minimal two-file docs tree.
const sidebarMd = ['## Getting Started', '', '- [Quick Start](quickstart)', ''].join('\n')
const quickstartMd = ['# Quick Start', '', 'Hello from the published docs.', ''].join('\n')

const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
  const url = String(input)
  if (url.endsWith('_sidebar.md')) return new Response(sidebarMd, { status: 200 })
  if (url.endsWith('quickstart.md')) return new Response(quickstartMd, { status: 200 })
  return new Response('not found', { status: 404 })
})

describe('App', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    cleanup()
  })

  it('renders the sidebar and defaults to the first doc', async () => {
    vi.stubGlobal('fetch', fetchMock)
    render(<App />)

    // Sidebar entry from _sidebar.md.
    expect(await screen.findAllByText('Quick Start')).not.toHaveLength(0)
    // Content of the first doc, rendered from markdown.
    expect(await screen.findByText('Hello from the published docs.')).toBeInTheDocument()
  })
})
