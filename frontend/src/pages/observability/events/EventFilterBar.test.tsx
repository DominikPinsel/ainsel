import { describe, expect, it } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes, useLocation } from 'react-router-dom'
import type { ReactNode } from 'react'
import { EventFilterBar } from './EventFilterBar'
import { renderWithProviders } from '../../../test/renderWithProviders'

function wrap(children: ReactNode, route = '/') {
  return renderWithProviders(
    <Routes>
      <Route path="*" element={children} />
    </Routes>,
    { route },
  )
}

function LocationSpy({ onSearch }: { onSearch: (s: string) => void }) {
  onSearch(useLocation().search)
  return null
}

describe('EventFilterBar', () => {
  it('renders the add button when there are no filters', () => {
    wrap(<EventFilterBar />)
    expect(screen.getByRole('button', { name: /add payload filter/i })).toBeInTheDocument()
  })

  it('does not render filter rows when q param is absent', () => {
    wrap(<EventFilterBar />)
    expect(screen.queryByRole('textbox', { name: /filter 1 field/i })).toBeNull()
  })

  it('renders filter rows from q param on mount', () => {
    wrap(<EventFilterBar />, '/?q=action%3Aeq%3Aopened')
    expect(screen.getByRole('textbox', { name: /filter 1 field/i })).toBeInTheDocument()
  })

  it('adds a filter row when the add button is clicked', async () => {
    wrap(<EventFilterBar />)
    await userEvent.click(screen.getByRole('button', { name: /add payload filter/i }))
    expect(screen.getByRole('textbox', { name: /filter 1 field/i })).toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: /filter 1 value/i })).toBeInTheDocument()
  })

  it('removes a filter row when the × button is clicked', async () => {
    wrap(<EventFilterBar />, '/?q=action%3Aeq%3Aopened')
    await userEvent.click(screen.getByRole('button', { name: /remove filter 1/i }))
    expect(screen.queryByRole('textbox', { name: /filter 1 field/i })).toBeNull()
  })

  it('shows the tip text when at least one filter exists', () => {
    wrap(<EventFilterBar />, '/?q=action%3Aeq%3Aopened')
    expect(screen.getByText(/^Tip: use/i)).toBeInTheDocument()
  })

  it('does not show tip text when no filters exist', () => {
    wrap(<EventFilterBar />)
    expect(screen.queryByText(/^Tip: use/i)).toBeNull()
  })

  it('updates q param when field input changes', async () => {
    let search = ''
    wrap(
      <>
        <EventFilterBar />
        <LocationSpy onSearch={(s) => { search = s }} />
      </>,
      '/?q=action%3Aeq%3Aopened',
    )
    const fieldInput = screen.getByRole('textbox', { name: /filter 1 field/i })
    await userEvent.clear(fieldInput)
    await userEvent.type(fieldInput, 'status')
    expect(search).toContain('q=')
  })
})
