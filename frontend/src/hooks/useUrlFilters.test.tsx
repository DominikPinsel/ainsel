import { describe, expect, it } from 'vitest'
import { act, render, renderHook } from '@testing-library/react'
import { MemoryRouter, useLocation } from 'react-router-dom'
import type { ReactNode } from 'react'
import { useUrlFilters } from './useUrlFilters'

type F = { agent?: string; connector?: string; page?: string }

function wrapper(initial: string) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <MemoryRouter initialEntries={[initial]}>{children}</MemoryRouter>
  }
}

describe('useUrlFilters', () => {
  it('returns undefined for absent params', () => {
    const { result } = renderHook(
      () => useUrlFilters<F>(['agent', 'connector', 'page']),
      { wrapper: wrapper('/triggers') },
    )
    expect(result.current.filters).toEqual({})
  })

  it('parses existing query params', () => {
    const { result } = renderHook(
      () => useUrlFilters<F>(['agent', 'connector', 'page']),
      { wrapper: wrapper('/triggers?agent=doc-writer&page=2') },
    )
    expect(result.current.filters.agent).toBe('doc-writer')
    expect(result.current.filters.page).toBe('2')
    expect(result.current.filters.connector).toBeUndefined()
  })

  it('setFilters merges and removes undefined keys', () => {
    let lastSearch = ''
    function Spy() {
      lastSearch = useLocation().search
      return null
    }
    let api: ReturnType<typeof useUrlFilters<F>>
    function Probe() {
      api = useUrlFilters<F>(['agent', 'connector', 'page'])
      return null
    }
    render(
      <MemoryRouter initialEntries={['/triggers?agent=doc-writer&page=2']}>
        <Probe />
        <Spy />
      </MemoryRouter>,
    )
    act(() => {
      api!.setFilters({ agent: undefined, connector: 'monorepo' })
    })
    expect(lastSearch).not.toContain('agent=')
    expect(lastSearch).toContain('connector=monorepo')
    expect(lastSearch).toContain('page=2')
  })

  it('reset clears all listed keys but keeps unrelated ones', () => {
    let lastSearch = ''
    function Spy() {
      lastSearch = useLocation().search
      return null
    }
    let api: ReturnType<typeof useUrlFilters<F>>
    function Probe() {
      api = useUrlFilters<F>(['agent', 'connector'])
      return null
    }
    render(
      <MemoryRouter initialEntries={['/triggers?agent=a&connector=c&page=2']}>
        <Probe />
        <Spy />
      </MemoryRouter>,
    )
    act(() => api!.reset())
    expect(lastSearch).not.toContain('agent=')
    expect(lastSearch).not.toContain('connector=')
    expect(lastSearch).toContain('page=2')
  })
})
