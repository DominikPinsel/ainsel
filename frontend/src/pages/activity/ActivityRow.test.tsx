import { describe, expect, it, vi } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes } from 'react-router-dom'
import { ActivityRow } from './ActivityRow'
import { renderWithProviders } from '../../test/renderWithProviders'
import type { ActivityEntry } from '../../api/events'

const entry: ActivityEntry = {
  id: 'evt-123',
  timestamp: new Date(Date.now() - 30_000).toISOString(),
  connector: 'c-111',
  status: 'matched',
  matches: [{ trigger: 't1', agent: 'doc-writer' }],
  payload: { action: 'opened', repository: 'ainsel' },
}

function renderRow({
  expanded = false,
  onToggle = vi.fn(),
  triggerNameById,
  agentNameById,
}: {
  expanded?: boolean
  onToggle?: () => void
  triggerNameById?: Map<string, string>
  agentNameById?: Map<string, string>
} = {}) {
  const utils = renderWithProviders(
    <table>
      <tbody>
        <ActivityRow
          entry={entry}
          expanded={expanded}
          onToggle={onToggle}
          triggerNameById={triggerNameById}
          agentNameById={agentNameById}
        />
      </tbody>
    </table>,
    { route: '/activity' },
  )
  return { onToggle, ...utils }
}

describe('ActivityRow', () => {
  it('renders an always-visible link to the event detail', () => {
    renderRow()
    const link = screen.getByRole('link', { name: 'open →' })
    expect(link).toHaveAttribute('href', '/observability/events/evt-123')
  })

  it('does not toggle expansion when the detail link is clicked', async () => {
    const onToggle = vi.fn()
    const { container } = renderRow({ onToggle })
    await userEvent.click(screen.getByRole('link', { name: 'open →' }))
    expect(onToggle).not.toHaveBeenCalled()
    expect(container.querySelector('.activity-details')).toBeNull()
  })

  it('navigates to the event detail page when the detail link is clicked', async () => {
    renderWithProviders(
      <Routes>
        <Route
          path="/activity"
          element={
            <table>
              <tbody>
                <ActivityRow entry={entry} expanded={false} onToggle={vi.fn()} />
              </tbody>
            </table>
          }
        />
        <Route path="/observability/events/:id" element={<div>Event detail page</div>} />
      </Routes>,
      { route: '/activity' },
    )
    await userEvent.click(screen.getByRole('link', { name: 'open →' }))
    expect(await screen.findByText('Event detail page')).toBeInTheDocument()
  })

  it('navigates to the event detail page when the row body is clicked', async () => {
    renderWithProviders(
      <Routes>
        <Route
          path="/activity"
          element={
            <table>
              <tbody>
                <ActivityRow entry={entry} expanded={false} onToggle={vi.fn()} />
              </tbody>
            </table>
          }
        />
        <Route path="/observability/events/:id" element={<div>Event detail page</div>} />
      </Routes>,
      { route: '/activity' },
    )
    // Click the connector cell (row body, not the chevron button)
    const connectorCell = screen.getByText('c-111')
    await userEvent.click(connectorCell)
    expect(await screen.findByText('Event detail page')).toBeInTheDocument()
  })

  it('toggles expansion when the chevron button is clicked without navigating', async () => {
    const onToggle = vi.fn()
    renderWithProviders(
      <Routes>
        <Route
          path="/activity"
          element={
            <table>
              <tbody>
                <ActivityRow entry={entry} expanded={false} onToggle={onToggle} />
              </tbody>
            </table>
          }
        />
        <Route path="/observability/events/:id" element={<div>Event detail page</div>} />
      </Routes>,
      { route: '/activity' },
    )
    const chevron = screen.getByRole('button', { name: /expand event details/i })
    await userEvent.click(chevron)
    expect(onToggle).toHaveBeenCalledTimes(1)
    // Should NOT have navigated
    expect(screen.queryByText('Event detail page')).toBeNull()
  })

  it('opens the event detail when Enter is pressed on the row', async () => {
    renderWithProviders(
      <Routes>
        <Route
          path="/activity"
          element={
            <table>
              <tbody>
                <ActivityRow entry={entry} expanded={false} onToggle={vi.fn()} />
              </tbody>
            </table>
          }
        />
        <Route path="/observability/events/:id" element={<div>Event detail page</div>} />
      </Routes>,
      { route: '/activity' },
    )
    const row = screen.getByRole('link', { name: `Open event ${entry.id}` })
    row.focus()
    await userEvent.keyboard('{Enter}')
    expect(await screen.findByText('Event detail page')).toBeInTheDocument()
  })

  it('renders the expanded payload and full-event link when expanded', () => {
    const { container } = renderRow({ expanded: true })
    expect(container.querySelector('.activity-details')).not.toBeNull()
    expect(screen.getByText(/"repository": "ainsel"/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /view full event/i })).toHaveAttribute(
      'href',
      '/observability/events/evt-123',
    )
  })

  it('renders resolved trigger and agent names as links to the right routes', () => {
    renderRow({
      triggerNameById: new Map([['t1', 'On doc issue']]),
      agentNameById: new Map([['doc-writer', 'Doc Writer']]),
    })
    const triggerLink = screen.getByRole('link', { name: 'Open trigger On doc issue' })
    expect(triggerLink).toHaveAttribute('href', '/agents/doc-writer?tab=triggers')
    expect(triggerLink).toHaveTextContent('On doc issue')
    const agentLink = screen.getByRole('link', { name: 'Open agent Doc Writer' })
    expect(agentLink).toHaveAttribute('href', '/agents/doc-writer')
    expect(agentLink).toHaveTextContent('Doc Writer')
  })

  it('falls back to the raw id as label when a name is unresolved, keeping the link', () => {
    // No name maps provided → both fall back to raw ids.
    renderRow()
    const triggerLink = screen.getByRole('link', { name: 'Open trigger t1' })
    expect(triggerLink).toHaveAttribute('href', '/agents/doc-writer?tab=triggers')
    expect(triggerLink).toHaveTextContent('t1')
    const agentLink = screen.getByRole('link', { name: 'Open agent doc-writer' })
    expect(agentLink).toHaveAttribute('href', '/agents/doc-writer')
    expect(agentLink).toHaveTextContent('doc-writer')
  })

  it('does not navigate to the event detail when a trigger/agent link is clicked', async () => {
    renderWithProviders(
      <Routes>
        <Route
          path="/activity"
          element={
            <table>
              <tbody>
                <ActivityRow entry={entry} expanded={false} onToggle={vi.fn()} />
              </tbody>
            </table>
          }
        />
        <Route path="/observability/events/:id" element={<div>Event detail page</div>} />
        <Route path="/agents/:id" element={<div>Agent page</div>} />
      </Routes>,
      { route: '/activity' },
    )
    await userEvent.click(screen.getByRole('link', { name: 'Open agent doc-writer' }))
    expect(await screen.findByText('Agent page')).toBeInTheDocument()
    expect(screen.queryByText('Event detail page')).toBeNull()
  })

  it('renders run state tag next to agent link when runStatus is present', () => {
    const withRun: ActivityEntry = {
      ...entry,
      matches: [{ trigger: 't1', agent: 'doc-writer', runStatus: 'success', durationMs: 1234 }],
    }
    renderWithProviders(
      <table>
        <tbody>
          <ActivityRow entry={withRun} expanded={false} onToggle={vi.fn()} />
        </tbody>
      </table>,
      { route: '/activity' },
    )
    expect(screen.getByText('SUCCESS')).toBeInTheDocument()
  })

  it('renders running tag for in-progress invocations', () => {
    const withRun: ActivityEntry = {
      ...entry,
      matches: [{ trigger: 't1', agent: 'doc-writer', runStatus: 'running' }],
    }
    renderWithProviders(
      <table>
        <tbody>
          <ActivityRow entry={withRun} expanded={false} onToggle={vi.fn()} />
        </tbody>
      </table>,
      { route: '/activity' },
    )
    expect(screen.getByText('RUNNING')).toBeInTheDocument()
  })

  it('renders failure tag and error detail in expanded view', () => {
    const withRun: ActivityEntry = {
      ...entry,
      matches: [{ trigger: 't1', agent: 'doc-writer', runStatus: 'failure', error: 'agent crashed', durationMs: 500 }],
    }
    const { container } = renderWithProviders(
      <table>
        <tbody>
          <ActivityRow entry={withRun} expanded onToggle={vi.fn()} />
        </tbody>
      </table>,
      { route: '/activity' },
    )
    expect(screen.getAllByText('FAILURE').length).toBeGreaterThanOrEqual(1)
    expect(container.textContent).toContain('agent crashed')
    expect(container.textContent).toContain('500ms')
  })

  it('does not render run state tag when runStatus is absent', () => {
    renderRow()
    expect(screen.queryByText('SUCCESS')).toBeNull()
    expect(screen.queryByText('RUNNING')).toBeNull()
    expect(screen.queryByText('FAILURE')).toBeNull()
  })

  it('renders comma-separated links for multiple matches in the expanded view', () => {
    const multi: ActivityEntry = {
      ...entry,
      matches: [
        { trigger: 't2', agent: 'infra-bot' },
        { trigger: 't3', agent: 'sec-bot' },
      ],
    }
    const { container } = renderWithProviders(
      <table>
        <tbody>
          <ActivityRow entry={multi} expanded onToggle={vi.fn()} />
        </tbody>
      </table>,
      { route: '/activity' },
    )
    const matches = container.querySelectorAll('.activity-matches .match')
    expect(matches.length).toBe(2)
    expect(matches[0].textContent).toContain('t2')
    expect(matches[0].textContent).toContain('infra-bot')
    expect(matches[1].textContent).toContain('sec-bot')
    // collapsed cell shows comma-separated agent links
    const collapsedRow = container.querySelector('tr.activity-row') as HTMLElement
    const agentLinks = Array.from(
      collapsedRow.querySelectorAll('a[aria-label^="Open agent"]'),
    )
    expect(agentLinks.map((l) => l.getAttribute('href'))).toEqual([
      '/agents/infra-bot',
      '/agents/sec-bot',
    ])
    expect(collapsedRow.textContent).toContain('infra-bot, sec-bot')
  })

  it('renders a Tokens cell when tokens prop is provided', () => {
    const { container } = renderRow({ expanded: false })
    // Without tokens prop: 5 cells in the row
    const cellsWithout = container.querySelectorAll('tr.activity-row > td')
    expect(cellsWithout.length).toBe(5)
  })

  it('renders a Tokens cell with value when tokens > 0', () => {
    renderWithProviders(
      <table>
        <tbody>
          <ActivityRow entry={entry} expanded={false} onToggle={vi.fn()} tokens={12500} />
        </tbody>
      </table>,
      { route: '/activity' },
    )
    expect(screen.getByText('12,500')).toBeInTheDocument()
  })

  it('renders — when tokens is 0', () => {
    const { container } = renderWithProviders(
      <table>
        <tbody>
          <ActivityRow entry={entry} expanded={false} onToggle={vi.fn()} tokens={0} />
        </tbody>
      </table>,
      { route: '/activity' },
    )
    // 6 cells when tokens prop is provided (5 + tokens)
    const cells = container.querySelectorAll('tr.activity-row > td')
    expect(cells.length).toBe(6)
  })

  it('adjusts colSpan in expanded row when tokens prop is provided', () => {
    const { container } = renderWithProviders(
      <table>
        <tbody>
          <ActivityRow entry={entry} expanded onToggle={vi.fn()} tokens={500} />
        </tbody>
      </table>,
      { route: '/activity' },
    )
    const detailCell = container.querySelector('tr.activity-details > td') as HTMLElement
    expect(detailCell).toHaveAttribute('colspan', '6')
  })
})
