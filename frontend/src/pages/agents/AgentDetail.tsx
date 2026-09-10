import { useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { useAgent, useDeleteAgent } from '../../api/agents'
import { ApiError } from '../../api/client'
import { usePersona } from '../../api/personas'
import { Button } from '../../primitives/Button'
import { ConfirmModal } from '../../primitives/ConfirmModal'
import { Dot } from '../../primitives/Dot'
import { Markdown } from '../../primitives/Markdown'
import { Panel } from '../../primitives/Panel'
import { Tabs } from '../../primitives/Tabs'
import { Tag } from '../../primitives/Tag'
import { Titleblock } from '../../layout/Titleblock'
import { AgentTriggers } from './AgentTriggers'
import { AgentSchedules } from './AgentSchedules'
import { AgentPersonaSection } from './AgentPersonaSection'
import {
  AgentImageSection,
  AgentToolsSection,
  AgentMCPSection,
  AgentSkillsSection,
} from './AgentImageSection'

const TABS = [
  { value: 'overview', label: 'Overview' },
  { value: 'persona', label: 'Persona' },
  { value: 'image', label: 'Image' },
  { value: 'tools', label: 'Tools' },
  { value: 'skills', label: 'Skills' },
  { value: 'triggers', label: 'Triggers' },
  { value: 'schedule', label: 'Schedule' },
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
                        <Button variant="ghost" onClick={() => onTabChange('image')}>
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
                      <Dot state={data.status?.ready ? 'ok' : 'warn'} />{' '}
                      {data.status?.ready ? 'Ready' : 'Pending'}
                    </div>
                  </div>
                  <div>
                    <div className="k">Replicas</div>
                    <div className="v">{data.status?.replicas ?? '—'}</div>
                  </div>
                  <div>
                    <div className="k">Configured Replicas</div>
                    <div className="v">{data.replicas ?? '—'}</div>
                  </div>
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

              {data.persona?.id ? (
                <PersonaPanel
                  personaId={data.persona.id}
                  onConfigure={() => onTabChange('persona')}
                />
              ) : null}
            </div>
          ) : null}

          {data && tab === 'persona' ? <AgentPersonaSection agent={data} /> : null}

          {data && tab === 'image' ? <AgentImageSection agent={data} /> : null}

          {data && tab === 'tools' ? (
            <>
              <AgentToolsSection agent={data} />
              <AgentMCPSection agent={data} />
            </>
          ) : null}

          {data && tab === 'skills' ? <AgentSkillsSection agent={data} /> : null}

          {data && tab === 'triggers' ? (
            <AgentTriggers agentId={data.id} agentName={data.name} />
          ) : null}

          {data && tab === 'schedule' ? (
            <AgentSchedules agentId={data.id} />
          ) : null}
        </div>
      </div>

      <ConfirmModal
        open={confirmOpen}
        title="Delete agent?"
        body={
          <>
            <b>{data?.name ?? id}</b> will be permanently removed. Triggers
            referencing this agent will become invalid.
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
  personaId,
  onConfigure,
}: {
  personaId: string
  onConfigure: () => void
}) {
  const { data, isLoading, error } = usePersona(personaId)

  if (isLoading) {
    return (
      <Panel title="Persona" className="cropped">
        <p className="label">Loading persona…</p>
      </Panel>
    )
  }
  if (error instanceof ApiError && error.status === 404) {
    return (
      <Panel title="Persona" className="cropped">
        <p className="label" style={{ color: 'var(--signal)' }}>
          Persona not found (id: {personaId}).
        </p>
      </Panel>
    )
  }
  if (!data) return null

  // 600 chars keeps the preview tight; full text is one click away.
  const preview = data.text.length > 600 ? data.text.slice(0, 600) + '…' : data.text

  return (
    <Panel title={`Persona · ${data.name}`} className="cropped">
      {data.description ? (
        <p style={{ marginTop: 0, marginBottom: 12, color: 'var(--ink-2)' }}>
          {data.description}
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
