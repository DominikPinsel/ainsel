import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ToolList } from './ToolList'
import type { ToolFormValue } from './ImageDetail.types'

function makeNative(name: string): ToolFormValue {
  return { name, kind: 'shell', enabled: true, isNew: false, description: '', examples: [] }
}
function makeMCP(server: string, toolName: string, opts: Partial<ToolFormValue> = {}): ToolFormValue {
  return {
    name: `mcp__${server}__${toolName}`,
    kind: 'mcp',
    mcpSource: server,
    enabled: true,
    isNew: false,
    description: '',
    examples: [],
    ...opts,
  }
}

const tools = [
  makeNative('run_shell'),
  makeMCP('example-mcp', 'list'),
  makeMCP('example-mcp', 'add', { enabled: false }),
  makeMCP('example-mcp', 'delete', { isNew: true, enabled: false }),
]

const noop = () => {}

describe('ToolList', () => {
  it('shows all tools when activeSource is "all"', () => {
    render(
      <ToolList
        tools={tools}
        activeSource="all"
        selectedIndex={null}
        onSelect={noop}
        onToggle={noop}
        onToggleAll={noop}
        onAdd={noop}
      />
    )
    expect(screen.getByText('run_shell')).toBeInTheDocument()
    expect(screen.getByText('list')).toBeInTheDocument()
    expect(screen.getByText('add')).toBeInTheDocument()
  })

  it('filters to native tools when activeSource is "native"', () => {
    render(
      <ToolList
        tools={tools}
        activeSource="native"
        selectedIndex={null}
        onSelect={noop}
        onToggle={noop}
        onToggleAll={noop}
        onAdd={noop}
      />
    )
    expect(screen.getByText('run_shell')).toBeInTheDocument()
    expect(screen.queryByText('list')).not.toBeInTheDocument()
  })

  it('filters to a specific MCP server when activeSource is a server name', () => {
    render(
      <ToolList
        tools={tools}
        activeSource="example-mcp"
        selectedIndex={null}
        onSelect={noop}
        onToggle={noop}
        onToggleAll={noop}
        onAdd={noop}
      />
    )
    expect(screen.queryByText('run_shell')).not.toBeInTheDocument()
    expect(screen.getByText('list')).toBeInTheDocument()
    expect(screen.getByText('add')).toBeInTheDocument()
  })

  it('calls onToggle with correct index and new value when checkbox is clicked', async () => {
    const onToggle = vi.fn()
    render(
      <ToolList
        tools={tools}
        activeSource="all"
        selectedIndex={null}
        onSelect={noop}
        onToggle={onToggle}
        onToggleAll={noop}
        onAdd={noop}
      />
    )
    // The "list" tool is at index 1 in tools array, currently enabled
    const listCheckbox = screen.getByRole('checkbox', { name: /toggle list/i })
    await userEvent.click(listCheckbox)
    expect(onToggle).toHaveBeenCalledWith(1, false)
  })

  it('shows "new" badge on isNew tools', () => {
    render(
      <ToolList
        tools={tools}
        activeSource="all"
        selectedIndex={null}
        onSelect={noop}
        onToggle={noop}
        onToggleAll={noop}
        onAdd={noop}
      />
    )
    expect(screen.getByText('new')).toBeInTheDocument()
  })

  it('shows bulk buttons only when source has more than 15 tools', () => {
    render(
      <ToolList
        tools={tools}
        activeSource="example-mcp"
        selectedIndex={null}
        onSelect={noop}
        onToggle={noop}
        onToggleAll={noop}
        onAdd={noop}
      />
    )
    expect(screen.queryByRole('button', { name: /all on/i })).not.toBeInTheDocument()

    const manyTools = Array.from({ length: 16 }, (_, i) => makeMCP('big', `tool${i}`))
    render(
      <ToolList
        tools={manyTools}
        activeSource="big"
        selectedIndex={null}
        onSelect={noop}
        onToggle={noop}
        onToggleAll={noop}
        onAdd={noop}
      />
    )
    expect(screen.getByRole('button', { name: /all on/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /all off/i })).toBeInTheDocument()
  })

  it('does not call onSelect when checkbox is toggled', async () => {
    const onSelect = vi.fn()
    const onToggle = vi.fn()
    render(
      <ToolList
        tools={tools}
        activeSource="all"
        selectedIndex={null}
        onSelect={onSelect}
        onToggle={onToggle}
        onToggleAll={noop}
        onAdd={noop}
      />
    )
    await userEvent.click(screen.getByRole('checkbox', { name: /toggle list/i }))
    expect(onSelect).not.toHaveBeenCalled()
    expect(onToggle).toHaveBeenCalledTimes(1)
  })

  it('calls onToggleAll with all source indices when All on is clicked', async () => {
    const onToggleAll = vi.fn()
    const manyTools = Array.from({ length: 16 }, (_, i) => makeMCP('big', `tool${i}`))
    render(
      <ToolList
        tools={manyTools}
        activeSource="big"
        selectedIndex={null}
        onSelect={noop}
        onToggle={noop}
        onToggleAll={onToggleAll}
        onAdd={noop}
      />
    )
    await userEvent.click(screen.getByRole('button', { name: /all on/i }))
    expect(onToggleAll).toHaveBeenCalledWith(
      Array.from({ length: 16 }, (_, i) => i),
      true
    )
  })

  it('search filters tool names within the active source', async () => {
    render(
      <ToolList
        tools={tools}
        activeSource="example-mcp"
        selectedIndex={null}
        onSelect={noop}
        onToggle={noop}
        onToggleAll={noop}
        onAdd={noop}
      />
    )
    const search = screen.getByRole('textbox')
    await userEvent.type(search, 'lis')
    expect(screen.getByText('list')).toBeInTheDocument()
    expect(screen.queryByText('add')).not.toBeInTheDocument()
  })
})
