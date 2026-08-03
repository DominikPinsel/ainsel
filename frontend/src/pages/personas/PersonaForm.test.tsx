import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes } from 'react-router-dom'
import { PersonaForm } from './PersonaForm'
import { renderWithProviders } from '../../test/renderWithProviders'

function defaultFetch(url: string, init?: RequestInit): Response {
  const method = init?.method ?? 'GET'
  if (url.match(/\/api\/v1\/groups(\?|$)/) && method === 'GET') {
    return new Response(
      JSON.stringify([
        { id: 'g1', name: 'Team A', description: '', createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z' },
      ]),
      { status: 200 },
    )
  }
  if (url.match(/\/api\/v1\/personas(\?|$)/) && method === 'POST') {
    return new Response(
      JSON.stringify({
        id: '01HXNEW',
        name: 'docs-helper',
        description: '',
        currentVersion: 1,
        text: 'Help with docs.',
        createdAt: '2026-05-21T00:00:00Z',
        updatedAt: '2026-05-21T00:00:00Z',
      }),
      { status: 201 },
    )
  }
  if (url.match(/\/api\/v1\/personas\/01HX1$/) && method === 'GET') {
    return new Response(
      JSON.stringify({
        id: '01HX1',
        name: 'code-reviewer',
        description: 'reviews PRs',
        currentVersion: 1,
        text: 'You are a code reviewer.',
        createdAt: '2026-05-01T00:00:00Z',
        updatedAt: '2026-05-01T00:00:00Z',
      }),
      { status: 200 },
    )
  }
  return new Response('{}', { status: 200 })
}

describe('PersonaForm', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) =>
        Promise.resolve(defaultFetch(url, init)),
      ),
    )
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('creates a persona', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/personas/new" element={<PersonaForm />} />
        <Route path="/personas/:id" element={<div>DETAIL:01HXNEW</div>} />
      </Routes>,
      { route: '/personas/new' },
    )
    await userEvent.type(screen.getByLabelText('Name'), 'docs-helper')
    await userEvent.type(screen.getByLabelText(/persona text/i), 'Help with docs.')
    await userEvent.selectOptions(await screen.findByLabelText('Group'), 'g1')
    await userEvent.click(screen.getByRole('button', { name: /create/i }))
    await waitFor(() =>
      expect(screen.getByText('DETAIL:01HXNEW')).toBeInTheDocument(),
    )
  })

  it('loads existing persona in edit mode', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/personas/:id/edit" element={<PersonaForm />} />
      </Routes>,
      { route: '/personas/01HX1/edit' },
    )
    await waitFor(() =>
      expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe(
        'code-reviewer',
      ),
    )
  })

  it('shows validation error when name is empty', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/personas/new" element={<PersonaForm />} />
      </Routes>,
      { route: '/personas/new' },
    )
    await userEvent.click(screen.getByRole('button', { name: /create/i }))
    expect(await screen.findByText(/name is required/i)).toBeInTheDocument()
  })
})
