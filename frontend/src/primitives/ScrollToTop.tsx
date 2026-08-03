import { useEffect, useState } from 'react'

type ScrollToTopProps = {
  /** Window scroll distance (px) past which the button becomes visible. */
  threshold?: number
}

/**
 * Floating "back to top" control tied to window scroll. Hidden until the page
 * is scrolled past `threshold`, then rendered bottom-right; clicking smoothly
 * scrolls the window back to the top.
 */
export function ScrollToTop({ threshold = 400 }: ScrollToTopProps) {
  const [visible, setVisible] = useState(false)

  useEffect(() => {
    const onScroll = () => setVisible(window.scrollY > threshold)
    onScroll()
    window.addEventListener('scroll', onScroll, { passive: true })
    return () => window.removeEventListener('scroll', onScroll)
  }, [threshold])

  if (!visible) return null

  return (
    <button
      type="button"
      className="btn to-top"
      aria-label="Back to top"
      onClick={() => window.scrollTo({ top: 0, behavior: 'smooth' })}
    >
      ↑ Top
    </button>
  )
}
