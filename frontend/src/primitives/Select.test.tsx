import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Select } from './Select'

const options = [
  { value: 'a', label: 'Apple' },
  { value: 'b', label: 'Banana' },
  { value: 'c', label: 'Cherry' },
]

describe('Select', () => {
  it('renders all options', () => {
    render(<Select id="x" value="a" onChange={() => {}} options={options} />)
    expect(screen.getByRole('option', { name: 'Apple' })).toBeInTheDocument()
    expect(screen.getByRole('option', { name: 'Banana' })).toBeInTheDocument()
    expect(screen.getByRole('option', { name: 'Cherry' })).toBeInTheDocument()
  })

  it('reflects the controlled value', () => {
    render(<Select id="x" value="b" onChange={() => {}} options={options} />)
    expect((screen.getByRole('combobox') as HTMLSelectElement).value).toBe('b')
  })

  it('fires onChange with the new value', async () => {
    const onChange = vi.fn()
    render(<Select id="x" value="a" onChange={onChange} options={options} />)
    await userEvent.selectOptions(screen.getByRole('combobox'), 'c')
    expect(onChange).toHaveBeenCalledWith('c')
  })

  it('renders an empty placeholder option when emptyLabel given', () => {
    render(<Select id="x" value="" onChange={() => {}} options={options} emptyLabel="Any" />)
    expect(screen.getByRole('option', { name: 'Any' })).toBeInTheDocument()
  })

  it('applies size modifier', () => {
    render(<Select id="x" value="a" onChange={() => {}} options={options} size="sm" />)
    expect(screen.getByRole('combobox')).toHaveClass('select', 'sm')
  })
})
