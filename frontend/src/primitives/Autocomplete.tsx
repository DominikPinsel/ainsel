import { useCallback, useEffect, useId, useMemo, useRef, useState } from 'react'

export type AutocompleteOption = {
  value: string
  label: string
}

type AutocompleteProps = {
  id?: string
  value: string
  onChange: (value: string) => void
  options: readonly AutocompleteOption[]
  placeholder?: string
  disabled?: boolean
  size?: 'md' | 'sm'
  className?: string
  inputClassName?: string
  'aria-label'?: string
  filter?: (option: AutocompleteOption, query: string) => boolean
}

const defaultFilter = (option: AutocompleteOption, query: string) =>
  option.label.toLowerCase().includes(query.toLowerCase())

export function Autocomplete({
  id: idProp,
  value,
  onChange,
  options,
  placeholder,
  disabled,
  size = 'md',
  className,
  inputClassName,
  'aria-label': ariaLabel,
  filter = defaultFilter,
}: AutocompleteProps) {
  const generatedId = useId()
  const id = idProp ?? generatedId
  const inputRef = useRef<HTMLInputElement>(null)
  const listRef = useRef<HTMLUListElement>(null)
  const containerRef = useRef<HTMLDivElement>(null)

  const selectedOption = useMemo(() => options.find((o) => o.value === value), [options, value])
  const selectedLabel = selectedOption?.label

  const [query, setQuery] = useState(() => selectedOption?.label ?? '')
  const [open, setOpen] = useState(false)
  const [highlightedIndex, setHighlightedIndex] = useState(0)
  const isClearingFromInputRef = useRef(false)

  const filtered = useMemo(() => options.filter((o) => filter(o, query)), [options, query, filter])

  // Keep highlighted index within bounds when the filtered list changes.
  useEffect(() => {
    setHighlightedIndex((idx) => Math.max(0, Math.min(idx, Math.max(0, filtered.length - 1))))
  }, [filtered.length])

  // Close the dropdown when clicking outside the component.
  useEffect(() => {
    function onDocClick(e: MouseEvent) {
      if (!containerRef.current?.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    if (open) {
      document.addEventListener('mousedown', onDocClick)
      return () => document.removeEventListener('mousedown', onDocClick)
    }
  }, [open])

  const selectValue = useCallback(
    (nextValue: string) => {
      onChange(nextValue)
      setQuery(options.find((o) => o.value === nextValue)?.label ?? '')
      setOpen(false)
      inputRef.current?.focus()
    },
    [onChange, options],
  )

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLInputElement>) => {
      if (e.key === 'ArrowDown') {
        e.preventDefault()
        if (!open) {
          setOpen(true)
          setHighlightedIndex(0)
        } else {
          setHighlightedIndex((idx) => Math.min(filtered.length - 1, idx + 1))
        }
      } else if (e.key === 'ArrowUp') {
        e.preventDefault()
        if (open) {
          setHighlightedIndex((idx) => Math.max(0, idx - 1))
        }
      } else if (e.key === 'Enter') {
        e.preventDefault()
        if (open && filtered.length > 0) {
          selectValue(filtered[highlightedIndex].value)
        }
      } else if (e.key === 'Escape') {
        e.preventDefault()
        setOpen(false)
      }
    },
    [filtered, highlightedIndex, open, selectValue],
  )

  const handleInputChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      setQuery(e.target.value)
      setOpen(true)
      // Clear selection when the user changes the typed text so callers can
      // disable actions until a valid suggestion is picked again.
      if (value) {
        isClearingFromInputRef.current = true
        onChange('')
      }
    },
    [onChange, value],
  )

  const handleInputFocus = useCallback(() => {
    setOpen(true)
  }, [])

  const handleOptionClick = useCallback(
    (nextValue: string) => {
      selectValue(nextValue)
    },
    [selectValue],
  )

  // Sync the input text with the controlled value. When the value is set but
  // the matching option label is not yet available (async options), the effect
  // will update the input as soon as the label resolves.
  useEffect(() => {
    if (isClearingFromInputRef.current) {
      isClearingFromInputRef.current = false
      return
    }

    if (value && selectedLabel !== undefined) {
      setQuery(selectedLabel)
    } else if (!value) {
      setQuery('')
    }
  }, [value, selectedLabel])

  const wrapperCls = ['autocomplete', size === 'sm' && 'sm', className].filter(Boolean).join(' ')
  const inputCls = ['input', 'autocomplete-input', inputClassName].filter(Boolean).join(' ')

  return (
    <div ref={containerRef} className={wrapperCls}>
      <input
        ref={inputRef}
        id={id}
        type="text"
        role="combobox"
        className={inputCls}
        value={query}
        onChange={handleInputChange}
        onKeyDown={handleKeyDown}
        onFocus={handleInputFocus}
        disabled={disabled}
        placeholder={placeholder}
        aria-label={ariaLabel}
        aria-autocomplete="list"
        aria-controls={`${id}-listbox`}
        aria-expanded={open}
        aria-activedescendant={
          open && filtered.length > 0 ? `${id}-option-${highlightedIndex}` : undefined
        }
        autoComplete="off"
      />
      {open ? (
        <ul ref={listRef} id={`${id}-listbox`} role="listbox" className="autocomplete-listbox">
          {filtered.length === 0 ? (
            <li className="autocomplete-empty" role="status">
              No matches.
            </li>
          ) : (
            filtered.map((option, idx) => (
              <li
                key={option.value}
                id={`${id}-option-${idx}`}
                role="option"
                aria-selected={idx === highlightedIndex}
                className={['autocomplete-option', idx === highlightedIndex && 'active']
                  .filter(Boolean)
                  .join(' ')}
                onMouseEnter={() => setHighlightedIndex(idx)}
                onClick={() => handleOptionClick(option.value)}
              >
                {option.label}
              </li>
            ))
          )}
        </ul>
      ) : null}
    </div>
  )
}
