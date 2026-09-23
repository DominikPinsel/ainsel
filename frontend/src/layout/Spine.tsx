import { useEffect, useReducer } from 'react'
import { NavLink, Link } from 'react-router-dom'
import { useAgents } from '../api/agents'
import { getAgentRecents, RECENT_AGENT_LIMIT } from '../agentRecents'
import { PREF_CHANGE_EVENT } from '../prefs'
import { Dot } from '../primitives/Dot'
import './Spine.css'

type NavItem = { idx: string; name: string; to: string; tag?: string }

const SECTIONS: { label: string; items: NavItem[] }[] = [
  {
    label: 'Operations',
    items: [
      { idx: '01', name: 'Dashboard', to: '/dashboard' },
      { idx: '06', name: 'Chat', to: '/chat' },
      { idx: '12', name: 'Channels', to: '/channels' },
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
    // Shared catalogs an agent draws from. Agent-scoped configuration lives on
    // the agent itself; these pages manage the reusable building blocks.
    label: 'Library',
    items: [
      { idx: '15', name: 'Images', to: '/agent-images' },
      { idx: '16', name: 'Personas', to: '/personas' },
      { idx: '17', name: 'Skills', to: '/skills' },
      { idx: '18', name: 'MCPs', to: '/settings' },
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
]

type SpineProps = {
  operator: string
  /** Recents bucket for the signed-in account; see recentsScope(). */
  recentsScope?: string
  /** Mobile drawer state; on desktop the spine is always visible. */
  open?: boolean
  onClose?: () => void
}

/**
 * Quick links under the Agents nav entry: the agents this browser opened
 * most recently, oldest-dropped, up to RECENT_AGENT_LIMIT. Before you have
 * clicked anything — or if you have clicked fewer than the limit — the
 * remaining slots are filled by the hub's recently-updated agents, so the
 * section is a stable set of shortcuts rather than something that appears and
 * disappears as you browse. Filler is rendered dimmer (see .padded) to keep
 * "yours" distinguishable from "whatever else is there".
 *
 * The list API is already scoped to what the caller can read, so "of the user"
 * falls out of the backend's access filtering.
 */
function RecentAgents({ scope }: { scope: string }) {
  const { data } = useAgents({ pageSize: 200 })
  // Recents live in localStorage, which React has no reason to re-render for;
  // the write side broadcasts PREF_CHANGE_EVENT, so subscribe to it.
  const [, rerender] = useReducer((n: number) => n + 1, 0)
  useEffect(() => {
    const handler = () => rerender()
    window.addEventListener(PREF_CHANGE_EVENT, handler)
    return () => window.removeEventListener(PREF_CHANGE_EVENT, handler)
  }, [])

  const items = data?.items ?? []
  const byId = new Map(items.map((a) => [a.id, a]))
  // Storage holds ids, but the hub decides what this caller can read: resolve
  // every remembered id through `byId` so an agent that was deleted, or that
  // you have lost access to since, can never render a link to a 404.
  const clicked = getAgentRecents(scope).flatMap((id) => {
    const agent = byId.get(id)
    return agent ? [{ agent, padded: false }] : []
  })
  const seen = new Set(clicked.map((entry) => entry.agent.id))
  const filler = [...items]
    .sort((a, b) => (b.updatedAt ?? '').localeCompare(a.updatedAt ?? ''))
    .filter((a) => !seen.has(a.id))
    .slice(0, Math.max(0, RECENT_AGENT_LIMIT - clicked.length))
    .map((agent) => ({ agent, padded: true }))

  const recent = [...clicked, ...filler]
  if (recent.length === 0) return null
  return (
    <div className="nav-recent" aria-label="Recent agents">
      {recent.map(({ agent: a, padded }) => (
        <NavLink
          key={a.id}
          to={`/agents/${encodeURIComponent(a.id)}`}
          className={({ isActive }) => {
            const classes = ['nav-sublink']
            if (isActive) classes.push('active')
            if (padded) classes.push('padded')
            return classes.join(' ')
          }}
        >
          <Dot state={a.status?.ready ? 'ok' : 'warn'} aria-label={a.name + ' status'} />
          <span className="name">{a.name}</span>
        </NavLink>
      ))}
    </div>
  )
}

export function Spine({
  operator,
  recentsScope = 'anon',
  open = false,
  onClose,
}: SpineProps) {
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
                  {item.to === '/agents' ? <RecentAgents scope={recentsScope} /> : null}
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
