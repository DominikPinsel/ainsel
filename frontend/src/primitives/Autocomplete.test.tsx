import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { Autocomplete, type AutocompleteOption } from './Autocomplete'

const options: AutocompleteOption[] = [
  { value: 'a', label: 'Alice' },
  { value: 'b', label: 'Bob' },
  { value: 'c', label: 'Charlie' },
]

function ControlledAutocomplete(props: {
  options: AutocompleteOption[]
  filter?: (option: AutocompleteOption, query: string) => boolean
  initialValue?: string
}) {
  const [value, setValue] = useState(props.initialValue ?? '')
  return (
    <>
      <Autocomplete
        value={value}
        onChange={setValue}
        options={props.options}
        filter={props.filter}
        aria-label="Pick a user"
      />
      <span data-testid="value">{value}</span>
    </>
  )
}

describe('Autocomplete', () => {
  it('renders the controlled label in the input', () => {
    render(<ControlledAutocomplete initialValue="b" options={options} />)
    expect(screen.getByRole('combobox')).toHaveValue('Bob')
  })

  it('opens the dropdown on focus and lists all options', async () => {
    render(<ControlledAutocomplete options={options} />)
    await userEvent.click(screen.getByRole('combobox'))
    expect(screen.getByRole('option', { name: 'Alice' })).toBeInTheDocument()
    expect(screen.getByRole('option', { name: 'Bob' })).toBeInTheDocument()
    expect(screen.getByRole('option', { name: 'Charlie' })).toBeInTheDocument()
  })

  it('filters options while typing', async () => {
    render(<ControlledAutocomplete options={options} />)
    const input = screen.getByRole('combobox')
    await userEvent.click(input)
    await userEvent.type(input, 'li')
    expect(screen.queryByRole('option', { name: 'Alice' })).toBeInTheDocument()
    expect(screen.queryByRole('option', { name: 'Bob' })).not.toBeInTheDocument()
    expect(screen.queryByRole('option', { name: 'Charlie' })).toBeInTheDocument()
  })

  it('selects an option with the mouse and updates the value', async () => {
    render(<ControlledAutocomplete options={options} />)
    await userEvent.click(screen.getByRole('combobox'))
    await userEvent.click(screen.getByRole('option', { name: 'Bob' }))
    expect(screen.getByRole('combobox')).toHaveValue('Bob')
    expect(screen.getByTestId('value')).toHaveTextContent('b')
  })

  it('navigates options with arrow keys and selects with enter', async () => {
    render(<ControlledAutocomplete options={options} />)
    const input = screen.getByRole('combobox')
    await userEvent.click(input)
    await userEvent.type(input, '{ArrowDown}{ArrowDown}{Enter}')
    expect(screen.getByRole('combobox')).toHaveValue('Charlie')
    expect(screen.getByTestId('value')).toHaveTextContent('c')
  })

  it('closes the dropdown on escape', async () => {
    render(<ControlledAutocomplete options={options} />)
    const input = screen.getByRole('combobox')
    await userEvent.click(input)
    expect(screen.getAllByRole('option').length).toBeGreaterThan(0)
    await userEvent.type(input, '{Escape}')
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument()
  })

  it('closes the dropdown when clicking outside', async () => {
    render(
      <div>
        <ControlledAutocomplete options={options} />
        <button>Outside</button>
      </div>,
    )
    await userEvent.click(screen.getByRole('combobox'))
    expect(screen.getByRole('listbox')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Outside' }))
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument()
  })

  it('shows an empty message when no options match', async () => {
    render(<ControlledAutocomplete options={options} />)
    const input = screen.getByRole('combobox')
    await userEvent.click(input)
    await userEvent.type(input, 'zzz')
    expect(screen.getByText('No matches.')).toBeInTheDocument()
    expect(screen.queryByRole('option')).not.toBeInTheDocument()
  })

  it('uses a custom filter when provided', async () => {
    const customOptions = [
      { value: 'a', label: 'Alice' },
      { value: 'b', label: 'Bob' },
    ]
    const filter = (option: AutocompleteOption, query: string) =>
      option.value === query.toLowerCase()
    render(<ControlledAutocomplete options={customOptions} filter={filter} />)
    const input = screen.getByRole('combobox')
    await userEvent.click(input)
    await userEvent.type(input, 'b')
    expect(screen.queryByRole('option', { name: 'Bob' })).toBeInTheDocument()
    expect(screen.queryByRole('option', { name: 'Alice' })).not.toBeInTheDocument()
  })

  it('clears selection when the user edits the input after a selection', async () => {
    render(<ControlledAutocomplete initialValue="b" options={options} />)
    const input = screen.getByRole('combobox')
    await userEvent.clear(input)
    await userEvent.type(input, 'a')
    expect(screen.getByTestId('value')).toBeEmptyDOMElement()
    expect(input).toHaveValue('a')
  })

  it('resets the input when the controlled value is cleared externally', async () => {
    function Example() {
      const [value, setValue] = useState('b')
      return (
        <>
          <Autocomplete
            value={value}
            onChange={setValue}
            options={options}
            aria-label="Pick a user"
          />
          <button onClick={() => setValue('')}>Clear</button>
        </>
      )
    }
    render(<Example />)
    expect(screen.getByRole('combobox')).toHaveValue('Bob')
    await userEvent.click(screen.getByRole('button', { name: 'Clear' }))
    expect(screen.getByRole('combobox')).toHaveValue('')
  })
})
