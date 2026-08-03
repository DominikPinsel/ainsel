import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes } from 'react-router-dom'
import { SkillForm } from './SkillForm'
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
  if (url.match(/\/api\/v1\/skills(\?|$)/) && method === 'POST') {
    return new Response(
      JSON.stringify({
        id: 'docs-helper',
        name: 'Docs Helper',
        description: 'helps with docs',
        body: 'Help with docs.',
        createdAt: '2026-05-21T00:00:00Z',
        updatedAt: '2026-05-21T00:00:00Z',
      }),
      { status: 201 },
    )
  }
  if (url.match(/\/api\/v1\/skills\/code-review$/) && method === 'GET') {
    return new Response(
      JSON.stringify({
        id: 'code-review',
        name: 'Code Review',
        description: 'reviews PRs',
        body: 'You are a code reviewer.',
        createdAt: '2026-05-01T00:00:00Z',
        updatedAt: '2026-05-01T00:00:00Z',
      }),
      { status: 200 },
    )
  }
  return new Response('{}', { status: 200 })
}

describe('SkillForm', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => Promise.resolve(defaultFetch(url, init))),
    )
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('creates a skill', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/skills/new" element={<SkillForm />} />
        <Route path="/skills/:id" element={<div>DETAIL:docs-helper</div>} />
      </Routes>,
      { route: '/skills/new' },
    )
    await userEvent.type(screen.getByLabelText('ID'), 'docs-helper')
    await userEvent.type(screen.getByLabelText('Name'), 'Docs Helper')
    await userEvent.type(screen.getByLabelText(/skill body/i), 'Help with docs.')
    await userEvent.selectOptions(await screen.findByLabelText('Group'), 'g1')
    await userEvent.click(screen.getByRole('button', { name: /create/i }))
    await waitFor(() => expect(screen.getByText('DETAIL:docs-helper')).toBeInTheDocument())
  })

  it('loads existing skill in edit mode', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/skills/:id/edit" element={<SkillForm />} />
      </Routes>,
      { route: '/skills/code-review/edit' },
    )
    await waitFor(() =>
      expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('Code Review'),
    )
    expect((screen.getByLabelText('ID') as HTMLInputElement).value).toBe('code-review')
    expect((screen.getByLabelText('ID') as HTMLInputElement).disabled).toBe(true)
  })

  it('shows validation error when name is empty', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/skills/new" element={<SkillForm />} />
      </Routes>,
      { route: '/skills/new' },
    )
    await userEvent.click(screen.getByRole('button', { name: /create/i }))
    expect(await screen.findByText(/id is required/i)).toBeInTheDocument()
    expect(await screen.findByText(/name is required/i)).toBeInTheDocument()
  })

  it('shows validation error for an invalid slug', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/skills/new" element={<SkillForm />} />
      </Routes>,
      { route: '/skills/new' },
    )
    await userEvent.type(screen.getByLabelText('ID'), 'Bad Slug!')
    await userEvent.type(screen.getByLabelText('Name'), 'Some Skill')
    await userEvent.click(screen.getByRole('button', { name: /create/i }))
    expect(await screen.findByText(/lowercase alphanumeric with hyphens/i)).toBeInTheDocument()
  })
})
