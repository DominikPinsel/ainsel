import { useEffect, useState } from 'react'
import { Outlet, useLocation } from 'react-router-dom'
import { Spine } from './Spine'
import { useAuth } from '../auth/AuthProvider'
import { ReportButton } from '../components/ReportButton'
import './Layout.css'

export function Layout() {
  const { user } = useAuth()
  const location = useLocation()
  const [menuOpen, setMenuOpen] = useState(false)

  // Close the drawer whenever the route changes (mobile nav selection).
  useEffect(() => {
    setMenuOpen(false)
  }, [location.pathname])

  // Close the drawer on Escape and lock body scroll while it is open.
  useEffect(() => {
    if (!menuOpen) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setMenuOpen(false)
    }
    window.addEventListener('keydown', onKey)
    const prevOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      window.removeEventListener('keydown', onKey)
      document.body.style.overflow = prevOverflow
    }
  }, [menuOpen])

  return (
    <div className={menuOpen ? 'shell menu-open' : 'shell'}>
      <a className="skip-link" href="#content">
        Skip to content
      </a>

      <header className="mobilebar">
        <button
          className="menu-btn"
          aria-label="Menu"
          aria-expanded={menuOpen}
          aria-controls="spine"
          onClick={() => setMenuOpen((v) => !v)}
        >
          <span className="menu-icon" aria-hidden="true" />
        </button>
        <span className="mobilebar-title">
          AInsel <em>Hub</em>
        </span>
      </header>

      <Spine operator={user?.username ?? 'anon'} open={menuOpen} onClose={() => setMenuOpen(false)} />
      <main id="content" className="canvas">
        <Outlet />
      </main>
      <ReportButton />
    </div>
  )
}
