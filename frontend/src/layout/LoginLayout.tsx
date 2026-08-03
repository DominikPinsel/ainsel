import { Outlet } from 'react-router-dom'

export function LoginLayout() {
  return (
    <div style={{ minHeight: '100vh', display: 'grid', placeItems: 'center', padding: 24 }}>
      <Outlet />
    </div>
  )
}
