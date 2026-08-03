import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { vi } from 'vitest'
import { DualListPicker } from './DualListPicker'

type Skill = { id: string; name: string; description: string }

const sample: Skill[] = [
  { id: 'aws-iam', name: 'aws-iam', description: 'identity mgmt' },
  { id: 'aws-s3', name: 'aws-s3', description: 'object storage' },
  { id: 'k8s-deploy', name: 'k8s-deploy', description: 'kubernetes ops' },
]

const baseProps = {
  items: sample,
  selectedIds: ['aws-iam'],
  onChange: () => {},
  getId: (s: Skill) => s.id,
  getLabel: (s: Skill) => s.name,
  getDescription: (s: Skill) => s.description,
}

describe('DualListPicker — rendering', () => {
  it('renders both panes with default titles and counts', () => {
    render(<DualListPicker {...baseProps} />)
    expect(screen.getByText(/Available \(2\)/)).toBeInTheDocument()
    expect(screen.getByText(/Enabled \(1\)/)).toBeInTheDocument()
  })

  it('renders rows in the correct pane based on selectedIds', () => {
    render(<DualListPicker {...baseProps} />)
    const available = screen.getByRole('listbox', { name: /available/i })
    const enabled = screen.getByRole('listbox', { name: /enabled/i })
    expect(available).toHaveTextContent('aws-s3')
    expect(available).toHaveTextContent('k8s-deploy')
    expect(enabled).toHaveTextContent('aws-iam')
  })

  it('uses custom titles when provided', () => {
    render(
      <DualListPicker
        {...baseProps}
        availableTitle="Catalog"
        enabledTitle="On this image"
      />,
    )
    expect(screen.getByText(/Catalog \(2\)/)).toBeInTheDocument()
    expect(screen.getByText(/On this image \(1\)/)).toBeInTheDocument()
  })

  it('shows loading placeholders and disables search when isLoading', () => {
    render(<DualListPicker {...baseProps} isLoading />)
    expect(screen.getAllByText(/Loading…/)).toHaveLength(2)
    for (const input of screen.getAllByRole('searchbox')) {
      expect(input).toBeDisabled()
    }
  })

  it('shows emptyLabel in the available pane when items is empty', () => {
    render(
      <DualListPicker
        {...baseProps}
        items={[]}
        selectedIds={[]}
        emptyLabel="Nothing here yet."
      />,
    )
    expect(screen.getByText('Nothing here yet.')).toBeInTheDocument()
  })
})

function Wrapper(props: Partial<React.ComponentProps<typeof DualListPicker<Skill>>>) {
  const [ids, setIds] = useState<string[]>(props.selectedIds ?? ['aws-iam'])
  const onChangeOverride = props.onChange
  return (
    <DualListPicker
      items={sample}
      getId={(s) => s.id}
      getLabel={(s) => s.name}
      getDescription={(s) => s.description}
      {...props}
      selectedIds={ids}
      onChange={(next) => {
        setIds(next)
        onChangeOverride?.(next)
      }}
    />
  )
}

describe('DualListPicker — selection and move', () => {
  it('clicking a row toggles its highlight without calling onChange', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<Wrapper onChange={onChange} />)
    const row = screen.getByRole('option', { name: /aws-s3/ })
    await user.click(row)
    expect(row).toHaveAttribute('aria-selected', 'true')
    expect(onChange).not.toHaveBeenCalled()
  })

  it('clicking → moves highlighted rows from available to enabled', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<Wrapper onChange={onChange} />)
    await user.click(screen.getByRole('option', { name: /aws-s3/ }))
    await user.click(screen.getByRole('button', { name: /add selected/i }))
    expect(onChange).toHaveBeenLastCalledWith(['aws-iam', 'aws-s3'])
    // Selection should be cleared after the move
    const enabled = screen.getByRole('listbox', { name: /enabled/i })
    const moved = within(enabled).getByRole('option', { name: /aws-s3/ })
    expect(moved).toHaveAttribute('aria-selected', 'false')
  })

  it('clicking ← moves highlighted rows from enabled to available', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<Wrapper onChange={onChange} />)
    const enabled = screen.getByRole('listbox', { name: /enabled/i })
    await user.click(within(enabled).getByRole('option', { name: /aws-iam/ }))
    await user.click(screen.getByRole('button', { name: /remove selected/i }))
    expect(onChange).toHaveBeenLastCalledWith([])
  })

  it('arrow buttons are disabled when no rows are highlighted on that side', () => {
    render(<Wrapper />)
    expect(screen.getByRole('button', { name: /add selected/i })).toBeDisabled()
    expect(screen.getByRole('button', { name: /remove selected/i })).toBeDisabled()
  })
})

describe('DualListPicker — search', () => {
  it('typing in available search hides non-matching rows', async () => {
    const user = userEvent.setup()
    render(<Wrapper />)
    const search = screen.getByRole('searchbox', { name: /search available/i })
    await user.type(search, 'k8s')
    const available = screen.getByRole('listbox', { name: /available/i })
    expect(within(available).getByText('k8s-deploy')).toBeInTheDocument()
    expect(within(available).queryByText('aws-s3')).toBeNull()
  })

  it('search also matches the description text', async () => {
    const user = userEvent.setup()
    render(<Wrapper selectedIds={[]} />)
    const search = screen.getByRole('searchbox', { name: /search available/i })
    await user.type(search, 'identity')
    const available = screen.getByRole('listbox', { name: /available/i })
    expect(within(available).getByText('aws-iam')).toBeInTheDocument()
  })

  it('selection highlight persists when query changes', async () => {
    const user = userEvent.setup()
    render(<Wrapper selectedIds={[]} />)
    const aws = screen.getByRole('option', { name: /aws-s3/ })
    await user.click(aws)
    expect(aws).toHaveAttribute('aria-selected', 'true')
    const search = screen.getByRole('searchbox', { name: /search available/i })
    await user.type(search, 'k8s')
    expect(screen.queryByRole('option', { name: /aws-s3/ })).toBeNull()
    await user.clear(search)
    expect(screen.getByRole('option', { name: /aws-s3/ })).toHaveAttribute('aria-selected', 'true')
  })
})

describe('DualListPicker — bulk move', () => {
  it('Add all visible moves only rows matching the current query', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<Wrapper selectedIds={[]} onChange={onChange} />)
    const search = screen.getByRole('searchbox', { name: /search available/i })
    await user.type(search, 'aws')
    await user.click(screen.getByRole('button', { name: /add all visible/i }))
    expect(onChange).toHaveBeenLastCalledWith(['aws-iam', 'aws-s3'])
  })

  it('Remove all visible removes only enabled rows matching the enabled query', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<Wrapper selectedIds={['aws-iam', 'aws-s3', 'k8s-deploy']} onChange={onChange} />)
    const search = screen.getByRole('searchbox', { name: /search enabled/i })
    await user.type(search, 'aws')
    await user.click(screen.getByRole('button', { name: /remove all visible/i }))
    expect(onChange).toHaveBeenLastCalledWith(['k8s-deploy'])
  })

  it('bulk buttons are disabled when their pane has no visible rows', () => {
    render(<Wrapper selectedIds={[]} items={[]} />)
    expect(screen.getByRole('button', { name: /add all visible/i })).toBeDisabled()
  })
})

describe('DualListPicker — range selection', () => {
  it('shift-click selects a contiguous range across visible rows', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<Wrapper selectedIds={[]} onChange={onChange} />)
    const first = screen.getByRole('option', { name: /aws-iam/ })
    const last = screen.getByRole('option', { name: /k8s-deploy/ })
    await user.click(first)
    await user.keyboard('{Shift>}')
    await user.click(last)
    await user.keyboard('{/Shift}')
    expect(first).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByRole('option', { name: /aws-s3/ })).toHaveAttribute('aria-selected', 'true')
    expect(last).toHaveAttribute('aria-selected', 'true')
  })
})

describe('DualListPicker — missing-from-catalog', () => {
  it('renders selectedIds not in items with (missing) and allows removal', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<Wrapper selectedIds={['deleted-skill']} onChange={onChange} />)
    const enabled = screen.getByRole('listbox', { name: /enabled/i })
    expect(within(enabled).getByText('deleted-skill')).toBeInTheDocument()
    expect(within(enabled).getByText(/\(missing\)/)).toBeInTheDocument()
    await user.click(within(enabled).getByRole('option', { name: /deleted-skill/ }))
    await user.click(screen.getByRole('button', { name: /remove selected/i }))
    expect(onChange).toHaveBeenLastCalledWith([])
  })
})
