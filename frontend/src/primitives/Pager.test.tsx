import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Pager } from './Pager'

describe('Pager', () => {
  const defaults = {
    page: 1,
    pageSize: 20,
    total: 147,
    pageSizeOptions: [10, 20, 50] as const,
  }

  it('renders page range and total in the info area', () => {
    render(<Pager {...defaults} onPageChange={() => {}} onPageSizeChange={() => {}} />)
    const info = screen.getByText(/of/).parentElement as HTMLElement
    expect(info.textContent).toMatch(/01/)
    expect(info.textContent).toMatch(/20/)
    expect(info.textContent).toMatch(/147/)
  })

  it('shows zeros when total is zero', () => {
    render(
      <Pager {...defaults} total={0} onPageChange={() => {}} onPageSizeChange={() => {}} />,
    )
    const info = screen.getByText(/of/).parentElement as HTMLElement
    expect(info.textContent).toMatch(/of\s+0/)
  })

  it('clamps last-row when on the partial last page', () => {
    render(
      <Pager
        page={8}
        pageSize={20}
        total={147}
        pageSizeOptions={[10, 20, 50]}
        onPageChange={() => {}}
        onPageSizeChange={() => {}}
      />,
    )
    const info = screen.getByText(/of/).parentElement as HTMLElement
    expect(info.textContent).toMatch(/141/)
    expect(info.textContent).toMatch(/147/)
  })

  it('disables prev on first page', () => {
    render(<Pager {...defaults} onPageChange={() => {}} onPageSizeChange={() => {}} />)
    const prev = screen.getByRole('button', { name: /previous/i })
    expect(prev).toBeDisabled()
  })

  it('disables next on last page', () => {
    render(
      <Pager
        page={8}
        pageSize={20}
        total={147}
        pageSizeOptions={[10, 20, 50]}
        onPageChange={() => {}}
        onPageSizeChange={() => {}}
      />,
    )
    const next = screen.getByRole('button', { name: /next/i })
    expect(next).toBeDisabled()
  })

  it('calls onPageChange with the next page when clicking next', async () => {
    const onPageChange = vi.fn()
    render(<Pager {...defaults} onPageChange={onPageChange} onPageSizeChange={() => {}} />)
    await userEvent.click(screen.getByRole('button', { name: /next/i }))
    expect(onPageChange).toHaveBeenCalledWith(2)
  })

  it('calls onPageSizeChange when the size select changes', async () => {
    const onPageSizeChange = vi.fn()
    render(<Pager {...defaults} onPageChange={() => {}} onPageSizeChange={onPageSizeChange} />)
    await userEvent.selectOptions(screen.getByRole('combobox'), '50')
    expect(onPageSizeChange).toHaveBeenCalledWith(50)
  })

  it('marks current page button as active', () => {
    render(
      <Pager
        page={3}
        pageSize={20}
        total={60}
        pageSizeOptions={[10, 20, 50]}
        onPageChange={() => {}}
        onPageSizeChange={() => {}}
      />,
    )
    const three = screen.getByRole('button', { name: 'Page 3' })
    expect(three).toHaveClass('active')
  })
})
