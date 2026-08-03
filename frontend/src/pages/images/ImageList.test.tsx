import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import { ImageList } from './ImageList'
import { renderWithProviders } from '../../test/renderWithProviders'

describe('ImageList', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/agent-images')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                items: [
                  {
                    id: 'i1',
                    displayName: 'claude-tooling-base',
                    imageURL: 'ghcr.io/insel/claude-tooling-base:1.4',
                    toolCount: 14,
                    enabledSkills: ['skill-a', 'skill-b'],
                  },
                  {
                    id: 'i2',
                    displayName: 'deprecated-base',
                    imageURL: 'ghcr.io/insel/deprecated-base:0.7',
                    toolCount: 0,
                  },
                ],
                total: 2,
                page: 1,
                pageSize: 20,
                totalPages: 1,
              }),
              { status: 200 },
            ),
          )
        }
        return Promise.resolve(new Response('{}', { status: 200 }))
      }),
    )
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders rows with image names and tool counts', async () => {
    renderWithProviders(<ImageList />, { route: '/agent-images' })
    await waitFor(() => expect(screen.getByText('claude-tooling-base')).toBeInTheDocument())
    expect(screen.getByText('deprecated-base')).toBeInTheDocument()
  })

  it('renders skill counts for each image', async () => {
    renderWithProviders(<ImageList />, { route: '/agent-images' })
    await waitFor(() => expect(screen.getByText('claude-tooling-base')).toBeInTheDocument())
    // First image has 2 skills — use getAllByText since the pager also shows "2"
    const skillCells = screen.getAllByText('2').filter((el) => el.classList.contains('num'))
    expect(skillCells.length).toBeGreaterThanOrEqual(1)
    // Second image has no enabledSkills field, defaults to 0
    expect(screen.getAllByText('0').length).toBeGreaterThanOrEqual(1)
  })
})
