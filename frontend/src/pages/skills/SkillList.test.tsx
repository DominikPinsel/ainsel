import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import { SkillList } from './SkillList'
import { renderWithProviders } from '../../test/renderWithProviders'

describe('SkillList', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/skills')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                items: [
                  {
                    id: 'code-review',
                    name: 'Code Review',
                    description: 'reviews PRs',
                    tags: [],
                    usedBy: 3,
                    createdAt: '2026-05-01T00:00:00Z',
                    updatedAt: '2026-05-10T00:00:00Z',
                  },
                  {
                    id: 'docs-helper',
                    name: 'Docs Helper',
                    description: 'helps with docs',
                    tags: [],
                    usedBy: 0,
                    createdAt: '2026-05-05T00:00:00Z',
                    updatedAt: '2026-05-05T00:00:00Z',
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

  it('renders skill rows', async () => {
    renderWithProviders(<SkillList />, { route: '/skills' })
    await waitFor(() => expect(screen.getByText('Code Review')).toBeInTheDocument())
    expect(screen.getByText('Docs Helper')).toBeInTheDocument()
    expect(screen.getByText('code-review')).toBeInTheDocument()
  })

  it('shows the Used by column with reference counts', async () => {
    renderWithProviders(<SkillList />, { route: '/skills' })
    await waitFor(() => expect(screen.getByText('Used by')).toBeInTheDocument())
    // code-review is referenced by 3 agent images; docs-helper by none (—).
    expect(screen.getByText('3')).toBeInTheDocument()
    expect(screen.getAllByText('—').length).toBeGreaterThan(0)
  })

  it('shows the New Skill header action', async () => {
    renderWithProviders(<SkillList />, { route: '/skills' })
    expect(await screen.findByRole('button', { name: /new skill/i })).toBeInTheDocument()
  })

  it('shows empty state when no skills', async () => {
    vi.unstubAllGlobals()
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          new Response(
            JSON.stringify({
              items: [],
              total: 0,
              page: 1,
              pageSize: 20,
              totalPages: 0,
            }),
            { status: 200 },
          ),
        ),
      ),
    )
    renderWithProviders(<SkillList />, { route: '/skills' })
    await waitFor(() => expect(screen.getByText(/no skills yet/i)).toBeInTheDocument())
  })
})
