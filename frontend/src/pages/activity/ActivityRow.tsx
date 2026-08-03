import { Link, useNavigate } from 'react-router-dom'
import { Fragment } from 'react'
import type { KeyboardEvent, MouseEvent } from 'react'
import type { ActivityEntry, ActivityMatch, RunStatus } from '../../api/events'
import { Tag } from '../../primitives/Tag'
import { formatRelative } from '../../utils/time'

type ActivityRowProps = {
  entry: ActivityEntry
  connectorName?: string
  triggerNameById?: Map<string, string>
  agentNameById?: Map<string, string>
  expanded: boolean
  onToggle: () => void
  /** When provided, renders an extra Tokens <td> and adjusts colSpan. */
  tokens?: number
}

const EMPTY_NAMES = new Map<string, string>()

function StatusTag({ status }: { status: ActivityEntry['status'] }) {
  if (status === 'matched') return <Tag variant="ok">MATCH</Tag>
  if (status === 'error') return <Tag variant="err">ERR</Tag>
  return <Tag variant="stale">SKIP</Tag>
}

function RunStateTag({ status }: { status: RunStatus }) {
  if (status === 'running') return <Tag variant="warn">RUNNING</Tag>
  if (status === 'success') return <Tag variant="ok">SUCCESS</Tag>
  if (status === 'failure') return <Tag variant="err">FAILURE</Tag>
  if (status === 'timeout') return <Tag variant="err">TIMEOUT</Tag>
  return <Tag variant="stale">—</Tag>
}

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  const s = ms / 1000
  if (s < 60) return `${s.toFixed(1)}s`
  const m = Math.floor(s / 60)
  const rem = s - m * 60
  return `${m}m ${rem.toFixed(0)}s`
}

// The agent ref carried by a match equals the agent id used by /agents/:id.
function agentPath(agentRef: string) {
  return `/agents/${encodeURIComponent(agentRef)}`
}

// Triggers have no standalone page; they live in the owning agent's Triggers tab.
function triggerPath(agentRef: string) {
  return `/agents/${encodeURIComponent(agentRef)}?tab=triggers`
}

const stopPropagation = (e: MouseEvent) => e.stopPropagation()

function TriggerLinks({
  matches,
  names,
}: {
  matches: ActivityMatch[]
  names: Map<string, string>
}) {
  return (
    <>
      {matches.map((m, i) => {
        const label = names.get(m.trigger) ?? m.trigger
        return (
          <Fragment key={i}>
            {i > 0 ? ', ' : ''}
            <Link
              to={triggerPath(m.agent)}
              onClick={stopPropagation}
              aria-label={`Open trigger ${label}`}
            >
              {label}
            </Link>
          </Fragment>
        )
      })}
    </>
  )
}

function AgentLinks({
  matches,
  names,
}: {
  matches: ActivityMatch[]
  names: Map<string, string>
}) {
  return (
    <>
      {matches.map((m, i) => {
        const label = names.get(m.agent) ?? m.agent
        return (
          <Fragment key={i}>
            {i > 0 ? ', ' : ''}
            <Link
              to={agentPath(m.agent)}
              onClick={stopPropagation}
              aria-label={`Open agent ${label}`}
            >
              {label}
            </Link>
            {m.runStatus ? (
              <> <RunStateTag status={m.runStatus} /></>
            ) : null}
          </Fragment>
        )
      })}
    </>
  )
}

export function ActivityRow({
  entry,
  connectorName,
  triggerNameById,
  agentNameById,
  expanded,
  onToggle,
  tokens,
}: ActivityRowProps) {
  const triggerNames = triggerNameById ?? EMPTY_NAMES
  const agentNames = agentNameById ?? EMPTY_NAMES
  const navigate = useNavigate()
  const detailPath = `/observability/events/${encodeURIComponent(entry.id)}`
  const matches = entry.matches ?? []
  const hasMatches = matches.length > 0

  const openDetail = () => navigate(detailPath)
  const handleRowKeyDown = (e: KeyboardEvent<HTMLTableRowElement>) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault()
      openDetail()
    }
  }
  const handleChevronKeyDown = (e: KeyboardEvent<HTMLButtonElement>) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault()
      e.stopPropagation()
      onToggle()
    }
  }

  return (
    <Fragment>
      <tr
        className={expanded ? 'activity-row expanded' : 'activity-row'}
        onClick={openDetail}
        onKeyDown={handleRowKeyDown}
        role="link"
        tabIndex={0}
        aria-label={`Open event ${entry.id}`}
      >
        <td className="num" style={{ width: 160 }}>
          <button
            type="button"
            className="chev-btn"
            aria-label={expanded ? 'Collapse event details' : 'Expand event details'}
            aria-expanded={expanded}
            onClick={(e) => {
              e.stopPropagation()
              onToggle()
            }}
            onKeyDown={handleChevronKeyDown}
          >
            <span className="chev">▸</span>
          </button>{' '}
          {formatRelative(entry.timestamp)}{' '}
          <Link
            to={detailPath}
            className="label"
            style={{ color: 'var(--ink-3)', textDecoration: 'none' }}
            onClick={(e) => e.stopPropagation()}
          >
            open →
          </Link>
        </td>
        <td>{connectorName ?? entry.connector ?? '—'}</td>
        <td>{hasMatches ? <TriggerLinks matches={matches} names={triggerNames} /> : '—'}</td>
        <td>{hasMatches ? <AgentLinks matches={matches} names={agentNames} /> : '—'}</td>
        <td>
          <StatusTag status={entry.status} />
        </td>
        {tokens !== undefined ? (
          <td className="num">{tokens > 0 ? tokens.toLocaleString() : '—'}</td>
        ) : null}
      </tr>
      {expanded ? (
        <tr className="activity-details">
          <td colSpan={tokens !== undefined ? 6 : 5}>
            {hasMatches ? (
              <div className="activity-matches">
                {matches.map((m, i) => {
                  const triggerLabel = triggerNames.get(m.trigger) ?? m.trigger
                  const agentLabel = agentNames.get(m.agent) ?? m.agent
                  return (
                    <div key={i} className="match">
                      Trigger{' '}
                      <b>
                        <Link
                          to={triggerPath(m.agent)}
                          onClick={stopPropagation}
                          aria-label={`Open trigger ${triggerLabel}`}
                        >
                          {triggerLabel}
                        </Link>
                      </b>{' '}
                      → agent{' '}
                      <b>
                        <Link
                          to={agentPath(m.agent)}
                          onClick={stopPropagation}
                          aria-label={`Open agent ${agentLabel}`}
                        >
                          {agentLabel}
                        </Link>
                      </b>
                      {m.runStatus ? (
                        <span style={{ marginLeft: 8 }}>
                          <RunStateTag status={m.runStatus} />
                          {m.durationMs != null ? (
                            <span className="label" style={{ marginLeft: 6 }}>
                              {formatDuration(m.durationMs)}
                            </span>
                          ) : null}
                        </span>
                      ) : null}
                      {m.error ? (
                        <div className="label" style={{ color: 'var(--signal)', marginTop: 2 }}>
                          Error: {m.error}
                        </div>
                      ) : null}
                    </div>
                  )
                })}
              </div>
            ) : null}
            {entry.payload != null ? (
              <>
                <div className="label" style={{ padding: '8px 0 4px' }}>Original Event</div>
                <pre className="code">
                  <code>{JSON.stringify(entry.payload, null, 2)}</code>
                </pre>
              </>
            ) : null}
            <div style={{ padding: '8px 0' }}>
              <Link to={detailPath}>
                View full event →
              </Link>
            </div>
          </td>
        </tr>
      ) : null}
    </Fragment>
  )
}
