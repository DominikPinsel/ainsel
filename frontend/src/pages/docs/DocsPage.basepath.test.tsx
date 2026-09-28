import { render, screen } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// Regression coverage for image paths under a *deployed* base path.
//
// `DOCS_BASE` is captured at module load from `import.meta.env.BASE_URL`, and
// vitest defaults that to '/'. Under '/' the old buggy output ('/docs/x.png')
// and the fixed output ('/docs/x.png') are byte-identical, so the companion
// suite in DocsPage.test.tsx cannot see the difference — it passes either way.
//
// In production the base is never '/': GitHub Pages serves the docs site from
// '/ainsel/' and the in-app console from '/ainsel-dev/'. A root-absolute
// '/docs/...' image src then resolves against the origin and 404s, because the
// asset really lives at '<base>/docs/...'. Stubbing BASE_URL and re-importing
// is the only way to reproduce that here, so it gets its own file: the
// `vi.resetModules()` must not disturb the statically-imported suite.

const SIDEBAR = ['## Getting Started', '- [Image Paths](image-paths)'].join('\n')

const DOC = [
  '# Image Paths',
  '',
  '![Dashboard](images/dashboard-overview.webp)',
  '',
  '![External](https://example.com/pic.png)',
].join('\n')

async function renderWithBase(base: string, path = '/docs/image-paths') {
  vi.stubEnv('BASE_URL', base)
  vi.resetModules()

  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      const p = String(url)
      if (p.endsWith('_sidebar.md')) {
        return { ok: true, status: 200, text: async () => SIDEBAR } as Response
      }
      const match = p.match(/\/docs\/(.+?)\.md$/)
      const slug = match ? match[1] : ''
      if (slug === 'image-paths') {
        return { ok: true, status: 200, text: async () => DOC } as Response
      }
      return { ok: false, status: 404, text: async () => 'not found' } as Response
    }),
  )

  // Imported *after* the stub so DOCS_BASE picks up the base under test.
  const { DocsPage } = await import('./DocsPage')

  render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/docs" element={<DocsPage />} />
        <Route path="/docs/*" element={<DocsPage />} />
      </Routes>
    </MemoryRouter>,
  )
}

describe('DocsPage image paths under a deployed base', () => {
  beforeEach(() => {
    vi.unstubAllEnvs()
  })

  afterEach(() => {
    vi.unstubAllEnvs()
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('prefixes image srcs with the GitHub Pages base', async () => {
    await renderWithBase('/ainsel/')

    expect(await screen.findByAltText('Dashboard')).toHaveAttribute(
      'src',
      '/ainsel/docs/images/dashboard-overview.webp',
    )
  })

  it('prefixes image srcs with the in-app base', async () => {
    await renderWithBase('/ainsel-dev/')

    expect(await screen.findByAltText('Dashboard')).toHaveAttribute(
      'src',
      '/ainsel-dev/docs/images/dashboard-overview.webp',
    )
  })

  it('leaves absolute image URLs alone regardless of base', async () => {
    await renderWithBase('/ainsel/')

    expect(await screen.findByAltText('External')).toHaveAttribute(
      'src',
      'https://example.com/pic.png',
    )
  })
})
