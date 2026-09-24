import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import {
  isAsleep,
  useAgent,
  useDeleteAgent
} from '../../api/agents'
import { ApiError } from '../../api/client'
import { useAgentPersona } from '../../api/personas'
import { recordAgentView, recentsScope } from '../../agentRecents'
import { useAuth } from '../../auth/AuthProvider'
import { Button } from '../../primitives/Button'
import { ConfirmModal } from '../../primitives/ConfirmModal'
import { Dot } from '../../primitives/Dot'
import { Markdown } from '../../primitives/Markdown'
import { Panel } from '../../primitives/Panel'
import { Tabs } from '../../primitives/Tabs'
import { Tag } from '../../primitives/Tag'
import { Titleblock } from '../../layout/Titleblock'
import { AgentChannelTab } from '../channels/AgentChannelTab'
import { AgentPersonaSection } from './AgentPersonaSection'
import {
  AgentImageSection,
  AgentToolsSection,
  AgentMCPSection,
  AgentSkillsSection,
} from './AgentImageSection'
import { AgentEnvSection } from './AgentEnvSection'

const TABS = [
  { value: 'overview', label: 'Overview' },
  { value: 'channel', label: 'Channel' },
  { value: 'persona', label: 'Persona' },
  { value: 'runtime', label: 'Runtime' },
  { value: 'tools', label: 'Tools' },
  { value: 'skills', label: 'Skills' },
] as const

const TAB_VALUES: string[] = TABS.map((t) => t.value)

export function AgentDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const tabParam = searchParams.get('tab')
  const tab = tabParam && TAB_VALUES.includes(tabParam) ? tabParam : 'overview'
  const onTabChange = (next: string) => {
    setSearchParams(next === 'overview' ? {} : { tab: next }, { replace: true })
  }
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)
  const { data, isLoading, error } = useAgent(id)
  const remove = useDeleteAgent()
  const { user } = useAuth()

  // Feed the nav's recent-agents list. Keyed on the loaded agent's id, not the
  // raw route param, so an unresolvable URL cannot be remembered as "viewed".
  // (The nav also filters these against what the hub returns you, so the
  // worst case for a stale id is an unused entry that the cap ages out.)
  useEffect(() => {
    if (data?.id) recordAgentView(recentsScope(user), data.id)
  }, [data?.id, user])

  const onConfirmDelete = async () => {
    if (!id) return
    setDeleteError(null)
    try {
      await remove.mutateAsync(id)
      setConfirmOpen(false)
      navigate('/agents', { replace: true })
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : 'Failed to delete agent.'
      setDeleteError(msg)
    }
  }

  // Zero pods means different things: an opted-in agent that drained its queue
  // is asleep by design, while the same count on any other agent is a pod that
  // cannot be scheduled. Only the operator's own verdict tells them apart, so the
  // UI must not infer it from the count.
  const queueScaled = data?.status?.mode === 'queue'
  const asleep = isAsleep(data?.status)
  return (
    <>
      <Titleblock
        crumbs={
          <>
            Fleet / <Link to="/agents">Agents</Link> /{' '}
            <b>{data?.name ?? id ?? '—'}</b>
          </>
        }
        title={
          <>
            {data?.name ?? <em>Agent</em>}
          </>
        }
        actions={
          <>
            <Button
              onClick={() => id && navigate(`/agents/${encodeURIComponent(id)}/edit`)}
            >
              Edit
            </Button>
            <Button variant="danger" onClick={() => setConfirmOpen(true)}>
              Delete
            </Button>
          </>
        }
      />
      <div style={{ padding: '28px 32px' }}>
        <Tabs value={tab} onChange={onTabChange} tabs={TABS} aria-label="Agent sections" />
        <div style={{ marginTop: 24 }}>
          {isLoading ? <p className="label">Loading…</p> : null}
          {error ? (
            <p className="label" style={{ color: 'var(--signal)' }}>
              Failed to load agent.
            </p>
          ) : null}
          {data && tab === 'overview' ? (
            <div style={{ display: 'grid', gap: 24 }}>
              <Panel title="General" className="cropped">
                <div className="info-grid">
                  <div>
                    <div className="k">Name</div>
                    <div className="v">{data.name}</div>
                  </div>
                  <div>
                    <div className="k">Model</div>
                    <div className="v">{data.llm?.model ?? '—'}</div>
                  </div>
                  <div>
                    <div className="k">Image</div>
                    <div className="v">
                      {data.imageRef ? (
                        <Button variant="ghost" onClick={() => onTabChange('runtime')}>
                          {data.imageRef.displayName ?? data.imageRef.name}
                        </Button>
                      ) : (
                        '—'
                      )}
                    </div>
                  </div>
                </div>
                {data.description ? (
                  <p style={{ marginTop: 8, color: 'var(--ink-2)' }}>{data.description}</p>
                ) : null}
              </Panel>

              <Panel title="Runtime Status" className="cropped">
                <div className="info-grid">
                  <div>
                    <div className="k">Ready</div>
                    <div className="v">
                      <Dot state={asleep ? 'stale' : data.status?.ready ? 'ok' : 'warn'} />{' '}
                      {asleep ? 'Asleep' : data.status?.ready ? 'Ready' : 'Pending'}
                    </div>
                  </div>
                  <div>
                    <div className="k">Containers</div>
                    <div className="v">
                      {asleep ? (
                        <>
                          0 of up to {data.replicas ?? '—'} · wakes on event
                        </>
                      ) : (
                        <>
                          {data.status?.replicas ?? '—'}
                          {queueScaled ? ` of up to ${data.replicas ?? '—'}` : ''}
                        </>
                      )}
                    </div>
                  </div>
                  {!queueScaled ? (
                    <div>
                      <div className="k">Configured Replicas</div>
                      <div className="v">{data.replicas ?? '—'}</div>
                    </div>
                  ) : null}
                </div>
              </Panel>

              {data.enabledTools && data.enabledTools.length > 0 ? (
                <Panel title={`Enabled Tools · ${data.enabledTools.length}`} className="cropped">
                  <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                    {data.enabledTools.map((t) => (
                      <Tag key={t}>{t}</Tag>
                    ))}
                  </div>
                </Panel>
              ) : null}

              <PersonaPanel
                agentId={data.id}
                onConfigure={() => onTabChange('persona')}
              />
            </div>
          ) : null}

          {data && tab === 'persona' ? <AgentPersonaSection agent={data} /> : null}

          {data && tab === 'runtime' ? (
            <>
              <AgentImageSection agent={data} />
              <AgentEnvSection agent={data} />
            </>
          ) : null}

          {data && tab === 'tools' ? (
            <>
              <AgentToolsSection agent={data} />
              <AgentMCPSection agent={data} />
            </>
          ) : null}

          {data && tab === 'skills' ? <AgentSkillsSection agent={data} /> : null}

          {data && tab === 'channel' ? (
            <AgentChannelTab agentId={data.id} agentName={data.name} />
          ) : null}
        </div>
      </div>

      <ConfirmModal
        open={confirmOpen}
        title="Delete agent?"
        body={
          <>
            <b>{data?.name ?? id}</b> will be permanently removed. Its inbox
            channel — subscriptions and schedules — goes with it.
          </>
        }
        confirmLabel={remove.isPending ? 'Deleting…' : 'Delete'}
        destructive
        error={deleteError}
        onConfirm={onConfirmDelete}
        onCancel={() => {
          setDeleteError(null)
          setConfirmOpen(false)
        }}
      />
    </>
  )
}

function PersonaPanel({
  agentId,
  onConfigure,
}: {
  agentId: string
  onConfigure: () => void
}) {
  // Agent-scoped on purpose: a persona an agent owns has no library permission
  // record, so reading it through /personas/{id} would be denied for
  // non-admins. This endpoint is gated by the agent instead, and reports
  // whether the persona is private to it or a shared template.
  const { data: view, isLoading } = useAgentPersona(agentId)

  if (isLoading) {
    return (
      <Panel title="Persona" className="cropped">
        <p className="label">Loading persona…</p>
      </Panel>
    )
  }

  const persona = view?.persona

  if (!persona) {
    return (
      <Panel title="Persona" className="cropped">
        <p style={{ marginTop: 0, color: 'var(--ink-2)' }}>
          {view?.ref
            ? `The linked persona (${view.ref}) no longer exists.`
            : 'No persona yet — this agent runs without one.'}
        </p>
        <p style={{ marginTop: 12 }}>
          <Button variant="ghost" onClick={onConfigure}>
            Configure persona →
          </Button>
        </p>
      </Panel>
    )
  }

  // 600 chars keeps the preview tight; full text is one click away.
  const preview =
    persona.text.length > 600 ? persona.text.slice(0, 600) + '…' : persona.text

  return (
    <Panel
      title={`Persona · ${persona.name}${view?.owned ? ' · own' : ' · shared template'}`}
      className="cropped"
    >
      {persona.description ? (
        <p style={{ marginTop: 0, marginBottom: 12, color: 'var(--ink-2)' }}>
          {persona.description}
        </p>
      ) : null}
      <Markdown source={preview} />
      <p style={{ marginTop: 12 }}>
        <Button variant="ghost" onClick={onConfigure}>
          Configure persona →
        </Button>
      </p>
    </Panel>
  )
}
