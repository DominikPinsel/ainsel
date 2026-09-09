import { NavLink, Link } from 'react-router-dom'
import { useAgents } from '../api/agents'
import { Dot } from '../primitives/Dot'
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
      { idx: '22', name: 'Connectors', to: '/connectors' },
    ],
  },
  {
    label: 'Setup',
    items: [
      { idx: '15', name: 'MCPs', to: '/settings' },
      { idx: '16', name: 'Skills', to: '/skills' },
    ],
  },
]

/** How many recently updated agents to surface under the Agents entry. */
const RECENT_AGENT_COUNT = 3

/**
 * The agents the user worked on most recently, shown as quick links under
 * the Agents nav entry. The list API is already scoped to what the caller
 * can read, so "of the user" falls out of the backend's access filtering;
 * recency comes from the hub-maintained updatedAt.
 */
function RecentAgents() {
  const { data } = useAgents({ pageSize: 200 })
  const recent = [...(data?.items ?? [])]
    .sort((a, b) => (b.updatedAt ?? '').localeCompare(a.updatedAt ?? ''))
    .slice(0, RECENT_AGENT_COUNT)
  if (recent.length === 0) return null
  return (
    <div className="nav-recent" aria-label="Recently updated agents">
      {recent.map((a) => (
        <NavLink
          key={a.id}
          to={`/agents/${encodeURIComponent(a.id)}`}
          className={({ isActive }) =>
            isActive ? 'nav-sublink active' : 'nav-sublink'
          }
        >
          <Dot state={a.status?.ready ? 'ok' : 'warn'} aria-label={a.name + ' status'} />
          <span className="name">{a.name}</span>
        </NavLink>
      ))}
    </div>
  )
}

type SpineProps = {
  operator: string
  /** Mobile drawer state; on desktop the spine is always visible. */
  open?: boolean
  onClose?: () => void
}

export function Spine({ operator, open = false, onClose }: SpineProps) {
  return (
    <>
      {open ? <div className="spine-backdrop" aria-hidden="true" onClick={onClose} /> : null}
      <aside id="spine" className={open ? 'spine open' : 'spine'}>
        <div className="brand">
          <div className="brand-mark">
            <span />
          </div>
          <h1>
            AInsel <em>Hub</em>
          </h1>
          {onClose ? (
            <button className="spine-close" aria-label="Close menu" onClick={onClose}>
              <span aria-hidden="true">&times;</span>
            </button>
          ) : null}
        </div>

        <nav className="nav" aria-label="Primary">
          {SECTIONS.map((section) => (
            <div key={section.label}>
              <div className="nav-section">
                <span className="label">{section.label}</span>
              </div>
              {section.items.map((item) => (
                <div key={item.to}>
                  <NavLink
                    to={item.to}
                    className={({ isActive }) =>
                      isActive ? 'nav-link active' : 'nav-link'
                    }
                  >
                    <span className="idx">{item.idx}</span>
                    <span className="name">{item.name}</span>
                    {item.tag ? <span className="nav-tag">{item.tag}</span> : <span />}
                  </NavLink>
                  {item.to === '/agents' ? <RecentAgents /> : null}
                </div>
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
    </>
  )
}
