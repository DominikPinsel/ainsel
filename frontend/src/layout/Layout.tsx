import { Outlet } from 'react-router-dom'
import { Spine } from './Spine'
import { useAuth } from '../auth/AuthProvider'
import { ReportButton } from '../components/ReportButton'
import './Layout.css'

export function Layout() {
  const { user } = useAuth()
  return (
    <div className="shell">
      <a className="skip-link" href="#content">
        Skip to content
      </a>
      <Spine operator={user?.username ?? 'anon'} />
      <main id="content" className="canvas">
        <Outlet />
      </main>
      <ReportButton />
    </div>
  )
}
