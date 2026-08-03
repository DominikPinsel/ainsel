import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { ScrollToTop } from './ScrollToTop'

function setScrollY(value: number) {
  Object.defineProperty(window, 'scrollY', { value, configurable: true })
  fireEvent.scroll(window)
}

describe('ScrollToTop', () => {
  let scrollTo: ReturnType<typeof vi.fn>

  beforeEach(() => {
    Object.defineProperty(window, 'scrollY', { value: 0, configurable: true })
    scrollTo = vi.fn()
    Object.defineProperty(window, 'scrollTo', {
      value: scrollTo,
      configurable: true,
      writable: true,
    })
  })

  afterEach(() => {
    Object.defineProperty(window, 'scrollY', { value: 0, configurable: true })
  })

  it('is hidden when the page is not scrolled', () => {
    render(<ScrollToTop />)
    expect(screen.queryByRole('button', { name: /back to top/i })).not.toBeInTheDocument()
  })

  it('appears after scrolling past the threshold', () => {
    render(<ScrollToTop />)
    setScrollY(500)
    expect(screen.getByRole('button', { name: /back to top/i })).toBeInTheDocument()
  })

  it('hides again after scrolling back above the threshold', () => {
    render(<ScrollToTop />)
    setScrollY(500)
    expect(screen.getByRole('button', { name: /back to top/i })).toBeInTheDocument()
    setScrollY(100)
    expect(screen.queryByRole('button', { name: /back to top/i })).not.toBeInTheDocument()
  })

  it('scrolls the window to the top on click', () => {
    render(<ScrollToTop />)
    setScrollY(500)
    fireEvent.click(screen.getByRole('button', { name: /back to top/i }))
    expect(scrollTo).toHaveBeenCalledWith({ top: 0, behavior: 'smooth' })
  })

  it('respects a custom threshold', () => {
    render(<ScrollToTop threshold={1000} />)
    setScrollY(500)
    expect(screen.queryByRole('button', { name: /back to top/i })).not.toBeInTheDocument()
    setScrollY(1200)
    expect(screen.getByRole('button', { name: /back to top/i })).toBeInTheDocument()
  })
})
