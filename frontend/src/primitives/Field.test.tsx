import { render, screen } from '@testing-library/react'
import { Field } from './Field'

describe('Field', () => {
  it('renders label associated with the inner control', () => {
    render(
      <Field label="Username" htmlFor="u">
        <input id="u" />
      </Field>,
    )
    const label = screen.getByText('Username')
    expect(label.tagName).toBe('LABEL')
    expect(label).toHaveAttribute('for', 'u')
    expect(label).toHaveClass('field-label')
  })

  it('renders error message when error prop set', () => {
    render(
      <Field label="X" htmlFor="x" error="Required">
        <input id="x" />
      </Field>,
    )
    expect(screen.getByText('Required')).toHaveClass('field-error')
  })

  it('omits error node when no error', () => {
    render(
      <Field label="X" htmlFor="x">
        <input id="x" />
      </Field>,
    )
    expect(document.querySelector('.field-error')).toBeNull()
  })
})
