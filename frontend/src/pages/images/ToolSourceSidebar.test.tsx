import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ToolSourceSidebar } from './ToolSourceSidebar'
import type { ToolFormValue } from './ImageDetail.types'

const native: ToolFormValue = {
  name: 'run_shell', kind: 'shell', enabled: true, isNew: false, description: '', examples: [],
}
const mcpA: ToolFormValue = {
  name: 'mcp__example-mcp__list', kind: 'mcp', mcpSource: 'example-mcp', enabled: true, isNew: false, description: '', examples: [],
}
const mcpB: ToolFormValue = {
  name: 'mcp__example-mcp__add', kind: 'mcp', mcpSource: 'example-mcp', enabled: false, isNew: true, description: '', examples: [],
}
const mcpC: ToolFormValue = {
  name: 'mcp__scanner__scan', kind: 'mcp', mcpSource: 'scanner', enabled: true, isNew: false, description: '', examples: [],
}

const tools = [native, mcpA, mcpB, mcpC]

describe('ToolSourceSidebar', () => {
  it('renders All, Native, and one entry per MCP server', () => {
    render(
      <ToolSourceSidebar
        tools={tools}
        activeSource="all"
        onSourceChange={() => {}}
        onRefresh={() => {}}
        isRefreshing={false}
        canRefresh={false}
      />
    )
    expect(screen.getByRole('button', { name: /all/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /native/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /example-mcp/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /scanner/i })).toBeInTheDocument()
  })

  it('calls onSourceChange with the clicked source', async () => {
    const onChange = vi.fn()
    render(
      <ToolSourceSidebar
        tools={tools}
        activeSource="all"
        onSourceChange={onChange}
        onRefresh={() => {}}
        isRefreshing={false}
        canRefresh={false}
      />
    )
    await userEvent.click(screen.getByRole('button', { name: /example-mcp/i }))
    expect(onChange).toHaveBeenCalledWith('example-mcp')
  })

  it('shows isNew indicator on a source that has new tools', () => {
    render(
      <ToolSourceSidebar
        tools={tools}
        activeSource="all"
        onSourceChange={() => {}}
        onRefresh={() => {}}
        isRefreshing={false}
        canRefresh={false}
      />
    )
    // example-mcp has mcpB which isNew=true → ★ present
    const exampleMcpBtn = screen.getByRole('button', { name: /example-mcp/i })
    expect(exampleMcpBtn.textContent).toContain('★')
    // scanner has no new tools → no ★
    const scannerBtn = screen.getByRole('button', { name: /scanner/i })
    expect(scannerBtn.textContent).not.toContain('★')
  })

  it('shows Refresh button only when canRefresh=true', () => {
    const { rerender } = render(
      <ToolSourceSidebar
        tools={[]}
        activeSource="all"
        onSourceChange={() => {}}
        onRefresh={() => {}}
        isRefreshing={false}
        canRefresh={false}
      />
    )
    expect(screen.queryByRole('button', { name: /refresh/i })).not.toBeInTheDocument()

    rerender(
      <ToolSourceSidebar
        tools={[]}
        activeSource="all"
        onSourceChange={() => {}}
        onRefresh={() => {}}
        isRefreshing={false}
        canRefresh={true}
      />
    )
    expect(screen.getByRole('button', { name: /refresh/i })).toBeInTheDocument()
  })
})
