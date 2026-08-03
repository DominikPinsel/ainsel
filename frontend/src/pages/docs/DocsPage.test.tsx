import { render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DocsPage } from './DocsPage'

// --- Mocks ------------------------------------------------------------------

const SIDEBAR = [
  '## Getting Started',
  '- [Architecture](architecture)',
  '- [Administrator Guide](administrator-guide)',
  '',
  '## Guides',
  '- [Writing a Connector](guides/writing-a-connector)',
  '- [Adding Memory](adding-memory)',
  '- [Dedup Test](dedup-test)',
  '- [Tilde Fence](tilde-fence)',
].join('\n')

const DOCS: Record<string, string> = {
  architecture:
    '# Ainsel Platform Architecture\n\nTechnical architecture reference. See the [administrator guide](administrator-guide.md).',
  'administrator-guide': '# Administrator Guide\n\nEnd-to-end admin journey.',
  'guides/writing-a-connector': '# Writing a Connector\n\nConnector guide.',

  // Mirrors docs/adding-memory.md structure: real headings + a 4-backtick
  // fenced block containing 3-backtick fences with headings inside.
  'adding-memory': [
    '# Adding Memory',
    '',
    '## Prerequisites',
    '',
    'Some prereq text.',
    '',
    '## Step 1 — Set up mem0',
    '',
    'Setup instructions.',
    '',
    '## Step 2 — Configure the skill',
    '',
    'Config instructions.',
    '',
    '````markdown',
    '# Memory Management',
    '',
    '## Environment',
    '',
    'Some env info.',
    '',
    '## Authentication',
    '',
    'Auth details.',
    '',
    '```bash',
    'echo "hello"',
    '```',
    '',
    '## API Reference',
    '',
    '### Store a memory',
    '',
    'Store details.',
    '',
    '## Rules',
    '',
    'Rule details.',
    '````',
    '',
    '## Step 3 — Write the handler',
    '',
    'Handler instructions.',
    '',
    '## Step 4 — Register the skill',
    '',
    'Register instructions.',
    '',
    '## Step 5 — Verify',
    '',
    'Verify instructions.',
    '',
    '## How it fits together',
    '',
    'Overview text.',
    '',
    '## Troubleshooting',
    '',
    'Troubleshooting text.',
    '',
    '## See also',
    '',
    'See also text.',
  ].join('\n'),

  // Tests that dedup is not polluted by fenced headings.
  'dedup-test': [
    '# Dedup Test',
    '',
    '## Setup',
    '',
    'First setup section.',
    '',
    '````',
    '## Setup',
    '## Details',
    '````',
    '',
    '## Details',
    '',
    'Second real heading with same text.',
    '',
    '## Cleanup',
    '',
    'Cleanup text.',
    '',
    '## Summary',
    '',
    'Summary text.',
  ].join('\n'),

  // Tilde fences should also be handled correctly.
  'tilde-fence': [
    '# Tilde Fence',
    '',
    '## Real Heading One',
    '',
    'Text.',
    '',
    '~~~',
    '## Phantom Inside Tilde',
    '~~~',
    '',
    '## Real Heading Two',
    '',
    'Text.',
    '',
    '## Real Heading Three',
    '',
    'Text.',
    '',
    '## Real Heading Four',
    '',
    'Text.',
  ].join('\n'),
}

let fetchMock: ReturnType<typeof vi.fn>

beforeEach(() => {
  fetchMock = vi.fn(async (url: string) => {
    const p = String(url)
    if (p.endsWith('_sidebar.md')) {
      return { ok: true, status: 200, text: async () => SIDEBAR } as Response
    }
    const match = p.match(/\/docs\/(.+?)\.md$/)
    const slug = match ? match[1] : ''
    if (slug in DOCS) {
      return { ok: true, status: 200, text: async () => DOCS[slug] } as Response
    }
    return { ok: false, status: 404, text: async () => 'not found' } as Response
  })
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

// --- Helpers ----------------------------------------------------------------

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/docs" element={<DocsPage />} />
        <Route path="/docs/*" element={<DocsPage />} />
      </Routes>
    </MemoryRouter>,
  )
}

// --- Tests ------------------------------------------------------------------

describe('DocsPage', () => {
  it('defaults to the first sidebar entry when on /docs', async () => {
    renderAt('/docs')
    await waitFor(() => {
      expect(screen.getByText(/Technical architecture reference/i)).toBeInTheDocument()
    })
  })

  it('renders the requested topic content', async () => {
    renderAt('/docs/administrator-guide')
    await waitFor(() => {
      expect(screen.getByText(/End-to-end admin journey/i)).toBeInTheDocument()
    })
  })

  it('renders section headers from _sidebar.md', async () => {
    renderAt('/docs/architecture')
    await waitFor(() => {
      expect(screen.getByText('Getting Started')).toBeInTheDocument()
      expect(screen.getByText('Guides')).toBeInTheDocument()
    })
  })

  it('fetches the sidebar and renders the sub-nav links grouped by section', async () => {
    renderAt('/docs/architecture')
    const nav = await screen.findByRole('navigation')
    await waitFor(() => {
      expect(within(nav).getByRole('link', { name: 'Architecture' })).toBeInTheDocument()
      expect(within(nav).getByRole('link', { name: 'Administrator Guide' })).toBeInTheDocument()
      expect(within(nav).getByRole('link', { name: /Writing a Connector/i })).toBeInTheDocument()
    })
  })

  it('marks the current topic as active in the sub-nav', async () => {
    renderAt('/docs/administrator-guide')
    const nav = await screen.findByRole('navigation')
    await waitFor(() => {
      const link = within(nav).getByRole('link', { name: 'Administrator Guide' })
      expect(link).toHaveClass('active')
    })
  })

  it('does not mark non-current topics as active', async () => {
    renderAt('/docs/architecture')
    const nav = await screen.findByRole('navigation')
    await waitFor(() => {
      expect(within(nav).getByRole('link', { name: 'Architecture' })).toHaveClass('active')
      expect(within(nav).getByRole('link', { name: 'Administrator Guide' })).not.toHaveClass('active')
    })
  })

  it('supports multi-segment slugs (subdirectory)', async () => {
    renderAt('/docs/guides/writing-a-connector')
    await waitFor(() => {
      expect(screen.getAllByText(/Connector guide/i).length).toBeGreaterThanOrEqual(1)
    })
  })

  it('rewrites relative .md links in content to in-app /docs/ routes', async () => {
    renderAt('/docs/architecture')
    // The content body contains a rewritten link to /docs/administrator-guide.
    // Use an exact (case-sensitive) match: the nav link is capitalised
    // "Administrator Guide" while the in-content link is lowercase.
    const contentLink = await screen.findByRole('link', { name: 'administrator guide' })
    expect(contentLink).toHaveAttribute('href', '/docs/administrator-guide')
  })

  it('redirects to /docs when the topic slug is invalid', async () => {
    renderAt('/docs/nonexistent')
    // After redirect to /docs, the first sidebar entry (architecture) renders.
    await waitFor(() => {
      expect(screen.getByText(/Technical architecture reference/i)).toBeInTheDocument()
    })
  })

  // --- TOC regression tests (#673) ------------------------------------------

  it('excludes headings inside nested fenced code blocks from the TOC', async () => {
    renderAt('/docs/adding-memory')

    // Wait for the content to render (text appears in both h2 and TOC link).
    await screen.findAllByText(/Prerequisites/i)

    // The TOC aside should contain real headings.
    const tocAside = await screen.findByText('Contents')
    const tocContainer = tocAside.closest('aside')!

    // Real headings must be present in the TOC.
    expect(within(tocContainer).getByText('Prerequisites')).toBeInTheDocument()
    expect(within(tocContainer).getByText(/Step 1/)).toBeInTheDocument()
    expect(within(tocContainer).getByText(/Step 2/)).toBeInTheDocument()
    expect(within(tocContainer).getByText(/Step 3/)).toBeInTheDocument()
    expect(within(tocContainer).getByText(/Step 4/)).toBeInTheDocument()
    expect(within(tocContainer).getByText(/Step 5/)).toBeInTheDocument()
    expect(within(tocContainer).getByText(/How it fits together/)).toBeInTheDocument()
    expect(within(tocContainer).getByText('Troubleshooting')).toBeInTheDocument()
    expect(within(tocContainer).getByText('See also')).toBeInTheDocument()

    // Phantom headings from inside the fenced code block must NOT appear.
    expect(within(tocContainer).queryByText('Environment')).not.toBeInTheDocument()
    expect(within(tocContainer).queryByText('Authentication')).not.toBeInTheDocument()
    expect(within(tocContainer).queryByText('API Reference')).not.toBeInTheDocument()
    expect(within(tocContainer).queryByText('Store a memory')).not.toBeInTheDocument()
    expect(within(tocContainer).queryByText('Rules')).not.toBeInTheDocument()
  })

  it('aligns TOC entry ids positionally with rendered h2/h3 elements', async () => {
    renderAt('/docs/adding-memory')

    // Wait for the content to render (text appears in both h2 and TOC link).
    await screen.findAllByText(/Prerequisites/i)

    // Collect all h2/h3 elements in the rendered content (scope to <main>
    // to exclude the titleblock header <h2>).
    const main = document.querySelector('main.docs-main')!
    const headings = main.querySelectorAll('h2, h3')
    // Collect all TOC links.
    const tocAside = await screen.findByText('Contents')
    const tocContainer = tocAside.closest('aside')!
    const tocLinks = tocContainer.querySelectorAll('a.docs-toc-link')

    // Each TOC link href should match the corresponding heading's id.
    expect(tocLinks.length).toBe(headings.length)
    tocLinks.forEach((link, i) => {
      const href = link.getAttribute('href')?.slice(1) // strip leading #
      expect(headings[i].id).toBe(href)
    })
  })

  it('does not let fenced headings pollute the dedup set', async () => {
    renderAt('/docs/dedup-test')

    await waitFor(() => {
      expect(screen.getByText(/First setup section/i)).toBeInTheDocument()
    })

    // The TOC should be visible (4 real headings: Setup, Details, Cleanup, Summary).
    const tocAside = await screen.findByText('Contents')
    const tocContainer = tocAside.closest('aside')!

    // "Setup" should appear once with the plain slug.
    const setupLinks = tocContainer.querySelectorAll('a.docs-toc-link[href="#setup"]')
    expect(setupLinks.length).toBe(1)

    // "Details" should appear once with the plain slug (not -2),
    // because the fenced "Details" must not have consumed the slug.
    const detailsLinks = tocContainer.querySelectorAll('a.docs-toc-link[href="#details"]')
    expect(detailsLinks.length).toBe(1)

    // Phantom headings must not appear.
    expect(within(tocContainer).queryByText('Phantom Inside Tilde')).not.toBeInTheDocument()
  })

  it('excludes headings inside tilde fences from the TOC', async () => {
    renderAt('/docs/tilde-fence')

    // Wait for the content to render (text appears in both h2 and TOC link).
    await screen.findAllByText(/Real Heading One/i)

    const tocAside = await screen.findByText('Contents')
    const tocContainer = tocAside.closest('aside')!

    // Real headings must be present.
    expect(within(tocContainer).getByText('Real Heading One')).toBeInTheDocument()
    expect(within(tocContainer).getByText('Real Heading Two')).toBeInTheDocument()
    expect(within(tocContainer).getByText('Real Heading Three')).toBeInTheDocument()
    expect(within(tocContainer).getByText('Real Heading Four')).toBeInTheDocument()

    // Phantom heading from inside the tilde fence must NOT appear.
    expect(within(tocContainer).queryByText('Phantom Inside Tilde')).not.toBeInTheDocument()
  })
})
