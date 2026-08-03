import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { TriggerForm } from './TriggerForm'
import { renderWithProviders } from '../../test/renderWithProviders'

function defaultFetch(url: string, init?: RequestInit): Response {
  if ((init?.method ?? 'GET') === 'GET' && url.includes('/groups')) {
    return new Response(
      JSON.stringify([
        { id: 'g1', name: 'Team A', description: '', createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z' },
      ]),
      { status: 200 },
    )
  }
  if (init?.method === 'POST' && url.includes('/triggers')) {
    return new Response(JSON.stringify({ id: 'new-id', name: 'x' }), { status: 200 })
  }
  if (init?.method === 'PUT' && url.includes('/triggers')) {
    return new Response(JSON.stringify({ id: 't1', name: 'edited' }), { status: 200 })
  }
  if (url.includes('/connectors')) {
    return new Response(
      JSON.stringify({
        items: [
          {
            id: 'c1',
            name: 'insel-monorepo',
            signatureHeader: 'X-Hub-Signature-256',
            disabled: false,
            status: { ready: true },
          },
        ],
        total: 1,
        page: 1,
        pageSize: 200,
        totalPages: 1,
      }),
      { status: 200 },
    )
  }
  return new Response('{}', { status: 200 })
}

describe('TriggerForm', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => Promise.resolve(defaultFetch(url, init))),
    )
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows validation errors for required fields', async () => {
    renderWithProviders(<TriggerForm agentId="doc-writer" onClose={() => {}} onSaved={() => {}} />)
    await userEvent.click(screen.getByRole('button', { name: /create/i }))
    expect(await screen.findByText(/name is required/i)).toBeInTheDocument()
    expect(screen.getByText(/connector is required/i)).toBeInTheDocument()
  })

  it('pre-fills the disabled Agent field from props', async () => {
    renderWithProviders(<TriggerForm agentId="doc-writer" onClose={() => {}} onSaved={() => {}} />)
    const agentInput = screen.getByLabelText('Agent') as HTMLInputElement
    expect(agentInput.value).toBe('doc-writer')
    expect(agentInput.disabled).toBe(true)
  })

  it('adds and removes filter rows', async () => {
    renderWithProviders(<TriggerForm agentId="doc-writer" onClose={() => {}} onSaved={() => {}} />)
    expect(screen.getByText(/no filters/i)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: /add filter row/i }))
    expect(screen.getByPlaceholderText(/field/)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: /remove filter 1/i }))
    expect(screen.getByText(/no filters/i)).toBeInTheDocument()
  })

  it('pre-fills fields when editing an existing trigger', async () => {
    renderWithProviders(
      <TriggerForm
        agentId="doc-writer"
        trigger={{
          id: 't1',
          name: 'docs-on-issue-open',
          agentRef: 'doc-writer',
          connectorRef: 'c1',
          filters: [{ field: 'action', op: 'eq', value: 'opened' }],
        }}
        onClose={() => {}}
        onSaved={() => {}}
      />,
    )
    // Wait for form to initialize and connectors to load
    await waitFor(() =>
      expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('docs-on-issue-open'),
    )
    // Wait for connector to be selected; Autocomplete shows the connector name
    // as the visible input value, while the hidden input retains the id.
    await waitFor(() =>
      expect((screen.getByLabelText('Connector') as HTMLInputElement).value).toBe('insel-monorepo'),
    )
    expect((screen.getByTestId('hidden-connectorRef') as HTMLInputElement).value).toBe('c1')
  })

  it('calls onSaved after a successful submit', async () => {
    const onSaved = vi.fn()
    renderWithProviders(<TriggerForm agentId="doc-writer" onClose={() => {}} onSaved={onSaved} />)
    await userEvent.type(screen.getByLabelText('Name'), 'on-pr')
    const connectorInput = screen.getByLabelText('Connector')
    await userEvent.click(connectorInput)
    await waitFor(() =>
      expect(screen.getByRole('option', { name: /insel-monorepo/i })).toBeInTheDocument(),
    )
    await userEvent.click(screen.getByRole('option', { name: /insel-monorepo/i }))
    await userEvent.selectOptions(await screen.findByLabelText('Group'), 'g1')
    await userEvent.click(screen.getByRole('button', { name: /create/i }))
    await waitFor(() => expect(onSaved).toHaveBeenCalled())
  })

  it('calls onClose when Cancel is clicked', async () => {
    const onClose = vi.fn()
    renderWithProviders(<TriggerForm agentId="doc-writer" onClose={onClose} onSaved={() => {}} />)
    await userEvent.click(screen.getByRole('button', { name: /cancel/i }))
    expect(onClose).toHaveBeenCalled()
  })

  it('renders not-contains operator correctly when editing', async () => {
    renderWithProviders(
      <TriggerForm
        agentId="doc-writer"
        trigger={{
          id: 't1',
          name: 'no-drafts',
          agentRef: 'doc-writer',
          connectorRef: 'c1',
          filters: [{ field: 'title', op: 'not-contains', value: 'draft' }],
        }}
        onClose={() => {}}
        onSaved={() => {}}
      />,
    )
    await waitFor(() =>
      expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('no-drafts'),
    )
    // The Select should show 'not-contains', not fall back to 'eq'
    const selects = screen.getAllByRole('combobox')
    // The filter-row select is the one inside the filter row (not the connector Autocomplete)
    const filterSelect = selects.find(
      (s) => (s as HTMLSelectElement).value === 'not-contains',
    )
    expect(filterSelect).toBeDefined()
  })

  it('loads values array as comma-separated string for in operator', async () => {
    renderWithProviders(
      <TriggerForm
        agentId="doc-writer"
        trigger={{
          id: 't2',
          name: 'label-filter',
          agentRef: 'doc-writer',
          connectorRef: 'c1',
          filters: [{ field: 'labels', op: 'in', value: '', values: ['bug', 'urgent'] }],
        }}
        onClose={() => {}}
        onSaved={() => {}}
      />,
    )
    await waitFor(() =>
      expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('label-filter'),
    )
    const valueInput = screen.getByPlaceholderText('value1, value2, …') as HTMLInputElement
    expect(valueInput.value).toBe('bug, urgent')
  })

  it('sends values array when submitting with in operator', async () => {
    const fetchMock = vi.fn((url: string, init?: RequestInit) =>
      Promise.resolve(defaultFetch(url, init)),
    )
    vi.stubGlobal('fetch', fetchMock)

    const onSaved = vi.fn()
    renderWithProviders(
      <TriggerForm agentId="doc-writer" onClose={() => {}} onSaved={onSaved} />,
    )
    await userEvent.type(screen.getByLabelText('Name'), 'test-trigger')
    const connectorInput = screen.getByLabelText('Connector')
    await userEvent.click(connectorInput)
    await waitFor(() =>
      expect(screen.getByRole('option', { name: /insel-monorepo/i })).toBeInTheDocument(),
    )
    await userEvent.click(screen.getByRole('option', { name: /insel-monorepo/i }))
    await userEvent.selectOptions(await screen.findByLabelText('Group'), 'g1')

    // Add a filter row with 'in' operator
    await userEvent.click(screen.getByRole('button', { name: /add filter row/i }))
    const fieldInput = screen.getByPlaceholderText(/field/)
    await userEvent.type(fieldInput, 'labels')
    // Select 'in' operator
    const selects = screen.getAllByRole('combobox')
    const filterSelect = selects.find((s) => s.closest('.filter-row'))
    if (filterSelect) {
      await userEvent.selectOptions(filterSelect, 'in')
    }
    const valueInput = screen.getByPlaceholderText('value1, value2, …')
    await userEvent.type(valueInput, 'bug, urgent')

    await userEvent.click(screen.getByRole('button', { name: /create/i }))
    await waitFor(() => expect(onSaved).toHaveBeenCalled())

    // Find the POST call to /triggers
    const postCall = fetchMock.mock.calls.find(
      ([url, init]) => init?.method === 'POST' && (url as string).includes('/triggers'),
    )
    expect(postCall).toBeDefined()
    const body = JSON.parse((postCall![1] as RequestInit).body as string)
    expect(body.filters[0].op).toBe('in')
    expect(body.filters[0].values).toEqual(['bug', 'urgent'])
    expect(body.filters[0].value).toBe('')
  })
})
