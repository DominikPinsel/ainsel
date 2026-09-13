import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AgentPersonaSection } from './AgentPersonaSection'
import type { AgentPersonaView } from '../../api/personas'
import type { AgentResponse } from '../../api/agents'
import { renderWithProviders } from '../../test/renderWithProviders'

const agent = { id: 'a1', name: 'doc-writer' } as AgentResponse

const template = {
  id: 'p-tmpl',
  name: 'docs-writer',
  description: 'docs persona',
  currentVersion: 3,
  text: '# Persona\n\nYou are a docs writer.',
  createdAt: '2026-05-01T00:00:00Z',
  updatedAt: '2026-05-01T00:00:00Z',
}

type Calls = { method: string; url: string; body?: string }[]

/**
 * Stubs fetch for the persona tab: the agent-scoped persona view, the template
 * list, and the agent PATCH used when switching the linked persona.
 */
function stubFetch(view: AgentPersonaView, opts: { saveStatus?: number } = {}) {
  const calls: Calls = []
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      const method = init?.method ?? 'GET'
      calls.push({ method, url, body: init?.body ? String(init.body) : undefined })

      if (/\/api\/v1\/agents\/a1\/persona$/.test(url)) {
        if (method === 'PUT') {
          const status = opts.saveStatus ?? 200
          if (status !== 200) {
            return Promise.resolve(
              new Response(JSON.stringify({ error: 'text is required' }), { status }),
            )
          }
          const body = JSON.parse(String(init?.body ?? '{}')) as Record<string, string>
          return Promise.resolve(
            new Response(
              JSON.stringify({
                owned: true,
                ref: 'p-own',
                persona: { ...template, id: 'p-own', ownerAgent: 'a1', ...body },
              }),
              { status: 200 },
            ),
          )
        }
        return Promise.resolve(new Response(JSON.stringify(view), { status: 200 }))
      }
      if (url.includes('/personas')) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              items: [template],
              total: 1,
              page: 1,
              pageSize: 200,
              totalPages: 1,
            }),
            { status: 200 },
          ),
        )
      }
      if (url.includes('/agents/a1')) {
        return Promise.resolve(
          new Response(JSON.stringify({ id: 'a1', name: 'doc-writer' }), { status: 200 }),
        )
      }
      return Promise.resolve(new Response('{}', { status: 200 }))
    }),
  )
  return calls
}

function renderSection() {
  return renderWithProviders(<AgentPersonaSection agent={agent} />)
}

describe('AgentPersonaSection', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('warns that a linked template is shared before the first save', async () => {
    stubFetch({ owned: false, ref: template.id, persona: template })
    renderSection()

    expect(
      await screen.findByText(/shared template.*private copy/i),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /save as own persona/i }),
    ).toBeInTheDocument()
    expect(screen.getByText(/first save forks a private copy/i)).toBeInTheDocument()
  })

  it('prefills the editor from the linked persona', async () => {
    stubFetch({ owned: false, ref: template.id, persona: template })
    renderSection()

    const text = await screen.findByLabelText('Persona Text')
    expect((text as HTMLTextAreaElement).value).toContain('You are a docs writer.')
    expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('docs-writer')
    expect((screen.getByLabelText('Description') as HTMLInputElement).value).toBe(
      'docs persona',
    )
  })

  it('saves inline edits to the agent-scoped endpoint, not the template', async () => {
    const calls = stubFetch({ owned: false, ref: template.id, persona: template })
    renderSection()

    const text = await screen.findByLabelText('Persona Text')
    await userEvent.clear(text)
    await userEvent.type(text, 'You write release notes.')
    await userEvent.click(screen.getByRole('button', { name: /save as own persona/i }))

    await waitFor(() => {
      const save = calls.find((c) => c.method === 'PUT' && /\/agents\/a1\/persona$/.test(c.url))
      expect(save).toBeDefined()
      expect(JSON.parse(save!.body!)).toEqual({
        name: 'docs-writer',
        description: 'docs persona',
        text: 'You write release notes.',
      })
    })
    // The shared template endpoint must never be written from this page.
    expect(calls.some((c) => c.method === 'PUT' && /\/personas\//.test(c.url))).toBe(false)
  })

  it('reports an owned persona as private', async () => {
    stubFetch({
      owned: true,
      ref: 'p-own',
      persona: { ...template, id: 'p-own', name: 'docs-writer', ownerAgent: 'a1' },
    })
    renderSection()

    expect(
      await screen.findByText(/belongs to this agent.*private to it/i),
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^save persona$/i })).toBeInTheDocument()
    expect(screen.queryByText(/first save forks a private copy/i)).not.toBeInTheDocument()
  })

  it('keeps the owned persona selectable alongside templates', async () => {
    stubFetch({
      owned: true,
      ref: 'p-own',
      persona: { ...template, id: 'p-own', ownerAgent: 'a1' },
    })
    renderSection()

    const select = (await screen.findByLabelText('Linked persona')) as HTMLSelectElement
    await waitFor(() => expect(select.options.length).toBeGreaterThan(1))
    const labels = within(select).getAllByRole('option').map((o) => o.textContent)
    expect(labels).toContain("docs-writer — this agent's own")
    expect(labels).toContain('docs-writer')
    expect(select.value).toBe('p-own')
  })

  it('offers to create a persona when the agent has none', async () => {
    const calls = stubFetch({ owned: false })
    renderSection()

    expect(
      await screen.findByText(/has no persona yet.*belongs to this agent/i),
    ).toBeInTheDocument()
    const select = screen.getByLabelText('Linked persona') as HTMLSelectElement
    expect(select.value).toBe('')

    await userEvent.type(screen.getByLabelText('Persona Text'), 'You are concise.')
    await userEvent.click(screen.getByRole('button', { name: /save as own persona/i }))

    await waitFor(() => {
      const save = calls.find((c) => c.method === 'PUT' && /\/agents\/a1\/persona$/.test(c.url))
      expect(save).toBeDefined()
      const body = JSON.parse(save!.body!) as Record<string, unknown>
      expect(body.text).toBe('You are concise.')
      // No name typed: the hub derives one from the agent.
      expect(body.name).toBeUndefined()
    })
  })

  it('flags a dangling persona reference', async () => {
    stubFetch({ owned: false, ref: 'p-gone' })
    renderSection()

    expect(
      await screen.findByText(/no longer exists.*creates a new one/i),
    ).toBeInTheDocument()
  })

  it('surfaces a save failure', async () => {
    stubFetch({ owned: false, ref: template.id, persona: template }, { saveStatus: 400 })
    renderSection()

    await screen.findByLabelText('Persona Text')
    await userEvent.click(screen.getByRole('button', { name: /save as own persona/i }))

    expect(await screen.findByText(/text is required/i)).toBeInTheDocument()
  })

  it('rejects an empty persona text before submitting', async () => {
    const calls = stubFetch({ owned: true, ref: 'p-own', persona: { ...template, id: 'p-own' } })
    renderSection()

    const text = await screen.findByLabelText('Persona Text')
    await userEvent.clear(text)
    await userEvent.click(screen.getByRole('button', { name: /^save persona$/i }))

    expect(await screen.findByText(/persona text is required/i)).toBeInTheDocument()
    expect(calls.some((c) => c.method === 'PUT')).toBe(false)
  })
})
