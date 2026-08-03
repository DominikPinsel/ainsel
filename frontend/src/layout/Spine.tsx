import { NavLink, Link } from 'react-router-dom'
import './Spine.css'

type NavItem = { idx: string; name: string; to: string; tag?: string }

const SECTIONS: { label: string; items: NavItem[] }[] = [
  {
    label: 'Operations',
    items: [
      { idx: '01', name: 'Dashboard', to: '/dashboard' },
      { idx: '06', name: 'Chat', to: '/chat' },
      { idx: '13', name: 'Activity', to: '/activity' },
      { idx: '14', name: 'Observability', to: '/observability' },
    ],
  },
  {
    label: 'Fleet',
    items: [
      { idx: '02', name: 'Agents', to: '/agents' },
      { idx: '03', name: 'Personas', to: '/personas' },
      { idx: '05', name: 'Agent Images', to: '/agent-images' },
      { idx: '07', name: 'Connectors', to: '/connectors' },
      { idx: '08', name: 'Skills', to: '/skills' },
    ],
  },
  {
    label: 'Docs',
    items: [
      { idx: '10', name: 'Docs', to: '/docs' },
    ],
  },
  {
    label: 'Admin',
    items: [
      { idx: '20', name: 'Users', to: '/users' },
      { idx: '21', name: 'Groups', to: '/groups' },
    ],
  },
  {
    label: 'Setup',
    items: [
      { idx: '15', name: 'Settings', to: '/settings' },
    ],
  },
]

export function Spine({ operator }: { operator: string }) {
  return (
    <aside className="spine">
      <div className="brand">
        <div className="brand-mark">
          <span />
        </div>
        <h1>
          AInsel <em>Hub</em>
        </h1>
      </div>

      <nav className="nav" aria-label="Primary">
        {SECTIONS.map((section) => (
          <div key={section.label}>
            <div className="nav-section">
              <span className="label">{section.label}</span>
            </div>
            {section.items.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                className={({ isActive }) => (isActive ? 'nav-link active' : 'nav-link')}
              >
                <span className="idx">{item.idx}</span>
                <span className="name">{item.name}</span>
                {item.tag ? <span className="nav-tag">{item.tag}</span> : <span />}
              </NavLink>
            ))}
          </div>
        ))}
      </nav>

      <div className="spine-footer">
        <Link to="/profile" className="row spine-profile-link">
          <span>Operator</span>
          <b>{operator}</b>
        </Link>
      </div>
    </aside>
  )
}