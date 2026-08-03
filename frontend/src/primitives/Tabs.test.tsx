import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Tabs } from './Tabs'

const tabs = [
  { value: 'overview', label: 'Overview' },
  { value: 'status', label: 'Status' },
]

describe('Tabs', () => {
  it('renders all tabs', () => {
    render(<Tabs value="overview" onChange={() => {}} tabs={tabs} />)
    expect(screen.getByRole('tab', { name: 'Overview' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Status' })).toBeInTheDocument()
  })

  it('marks the active tab', () => {
    render(<Tabs value="status" onChange={() => {}} tabs={tabs} />)
    const status = screen.getByRole('tab', { name: 'Status' })
    expect(status).toHaveAttribute('aria-selected', 'true')
    expect(status).toHaveClass('tab', 'active')
  })

  it('calls onChange when a tab is clicked', async () => {
    const onChange = vi.fn()
    render(<Tabs value="overview" onChange={onChange} tabs={tabs} />)
    await userEvent.click(screen.getByRole('tab', { name: 'Status' }))
    expect(onChange).toHaveBeenCalledWith('status')
  })
})
