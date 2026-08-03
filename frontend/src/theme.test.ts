import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { applyTheme, toggleTheme } from './theme'

function mockMatchMedia(prefersDark: boolean) {
  vi.stubGlobal(
    'matchMedia',
    vi.fn((query: string) => ({
      matches: prefersDark && query === '(prefers-color-scheme: dark)',
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  )
}

describe('applyTheme', () => {
  beforeEach(() => {
    mockMatchMedia(false)
    localStorage.clear()
    document.documentElement.removeAttribute('data-theme')
    document.documentElement.style.colorScheme = ''
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('sets data-theme="light" and colorScheme="light" for light theme', () => {
    applyTheme('light')
    expect(document.documentElement.dataset.theme).toBe('light')
    expect(document.documentElement.style.colorScheme).toBe('light')
  })

  it('sets data-theme="dark" and colorScheme="dark" for dark theme', () => {
    applyTheme('dark')
    expect(document.documentElement.dataset.theme).toBe('dark')
    expect(document.documentElement.style.colorScheme).toBe('dark')
  })

  it('sets data-theme="peat" and colorScheme="dark" for peat theme', () => {
    applyTheme('peat')
    expect(document.documentElement.dataset.theme).toBe('peat')
    expect(document.documentElement.style.colorScheme).toBe('dark')
  })

  it('sets data-theme="tallow" and colorScheme="dark" for tallow theme', () => {
    applyTheme('tallow')
    expect(document.documentElement.dataset.theme).toBe('tallow')
    expect(document.documentElement.style.colorScheme).toBe('dark')
  })

  it('falls back to light when theme is null and system prefers light', () => {
    mockMatchMedia(false)
    applyTheme(null)
    expect(document.documentElement.dataset.theme).toBe('light')
  })

  it('falls back to dark when theme is null and system prefers dark', () => {
    mockMatchMedia(true)
    applyTheme(null)
    expect(document.documentElement.dataset.theme).toBe('dark')
  })
})

describe('toggleTheme', () => {
  beforeEach(() => {
    mockMatchMedia(false)
    localStorage.clear()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    localStorage.clear()
  })

  it('toggles from light to dark', () => {
    document.documentElement.dataset.theme = 'light'
    toggleTheme()
    expect(localStorage.getItem('ainsel-theme')).toBe('dark')
  })

  it('toggles from dark to light', () => {
    document.documentElement.dataset.theme = 'dark'
    toggleTheme()
    expect(localStorage.getItem('ainsel-theme')).toBe('light')
  })

  it('toggles from peat to dark (peat is treated as non-dark by the toggle)', () => {
    document.documentElement.dataset.theme = 'peat'
    toggleTheme()
    expect(localStorage.getItem('ainsel-theme')).toBe('dark')
  })
})
