import { render, screen, within } from '@testing-library/react'
import { RegisterTable } from './RegisterTable'

type Row = { id: string; name: string; status: 'ok' | 'err' }

const columns = [
  { key: 'name', header: 'Name', cell: (r: Row) => r.name },
  { key: 'status', header: 'Status', cell: (r: Row) => r.status },
] as const

describe('RegisterTable', () => {
  it('renders headers and rows with row numbers', () => {
    const rows: Row[] = [
      { id: 'a', name: 'alpha', status: 'ok' },
      { id: 'b', name: 'beta', status: 'err' },
    ]
    render(<RegisterTable rows={rows} columns={columns} rowKey={(r) => r.id} />)
    expect(screen.getByText('Name')).toBeInTheDocument()
    expect(screen.getByText('alpha')).toBeInTheDocument()
    expect(screen.getByText('beta')).toBeInTheDocument()
    expect(screen.getByText('01')).toBeInTheDocument()
    expect(screen.getByText('02')).toBeInTheDocument()
  })

  it('renders empty-state message when no rows', () => {
    render(
      <RegisterTable
        rows={[] as Row[]}
        columns={columns}
        rowKey={(r) => r.id}
        emptyLabel="No connectors configured."
      />,
    )
    expect(screen.getByText('No connectors configured.')).toBeInTheDocument()
  })

  it('applies row-err class when rowClassName returns it', () => {
    const rows: Row[] = [{ id: 'x', name: 'broken', status: 'err' }]
    const { container } = render(
      <RegisterTable
        rows={rows}
        columns={columns}
        rowKey={(r) => r.id}
        rowClassName={(r) => (r.status === 'err' ? 'row-err' : undefined)}
      />,
    )
    const tr = within(container).getByText('broken').closest('tr') as HTMLElement
    expect(tr).toHaveClass('row-err')
  })

  it('skips the rowno column when withRowNumbers={false}', () => {
    const rows: Row[] = [{ id: 'a', name: 'alpha', status: 'ok' }]
    const { container } = render(
      <RegisterTable
        rows={rows}
        columns={columns}
        rowKey={(r) => r.id}
        withRowNumbers={false}
      />,
    )
    expect(container.querySelector('.rowno')).toBeNull()
  })
})
