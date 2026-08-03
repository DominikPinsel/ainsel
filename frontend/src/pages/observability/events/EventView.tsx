import { useMemo, type ReactNode } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useConversations } from '../../../api/conversations'
import { useEvent } from '../../../api/events'
import { useInvocations, invocationStatusVariant, type InvocationEntry } from '../../../api/invocations'
import { Titleblock } from '../../../layout/Titleblock'
import { Panel } from '../../../primitives/Panel'
import { ScrollToTop } from '../../../primitives/ScrollToTop'
import { SectionStatus } from '../../../primitives/SectionStatus'
import { Tag } from '../../../primitives/Tag'
import { formatISO } from '../../../utils/time'
import { ConversationTranscript } from './ConversationTranscript'

function StatusTag({ status }: { status: 'matched' | 'unmatched' | 'error' }) {
  if (status === 'matched') return <Tag variant="ok">MATCH</Tag>
  if (status === 'error') return <Tag variant="err">ERR</Tag>
  return <Tag variant="stale">SKIP</Tag>
}

function formatDuration(ms: number | undefined): string {
  return ms !== undefined ? `${(ms / 1000).toFixed(1)}s` : '—'
}

function Kpi({
  label,
  children,
  size = 18,
}: {
  label: string
  children: ReactNode
  size?: number
}) {
  return (
    <div className="kpi" style={{ cursor: 'default' }}>
      <div className="hd">
        <span className="label">{label}</span>
      </div>
      <div className="figure" style={{ fontSize: size }}>
        {children}
      </div>
    </div>
  )
}

function InvocationDetail({ invocation }: { invocation: InvocationEntry }) {
  const conversations = useConversations({ invocation: invocation.id })
  const messages = useMemo(() => conversations.data?.messages ?? [], [conversations.data])
  const total = conversations.data?.total
  const tokens = useMemo(
    () =>
      messages.reduce(
        (sum, m) =>
          m.role === 'assistant' ? sum + (m.inputTokens ?? 0) + (m.outputTokens ?? 0) : sum,
        0,
      ),
    [messages],
  )

  return (
    <Panel
      title={`Invocation ${invocation.id} · ${invocation.agentName ?? invocation.agent}`}
      className="cropped"
    >
      <div className="kpi-row" style={{ gridTemplateColumns: 'repeat(5, 1fr)' }}>
        <Kpi label="Agent">{invocation.agentName ?? invocation.agent}</Kpi>
        <Kpi label="Trigger">{invocation.triggerName ?? invocation.trigger ?? '—'}</Kpi>
        <Kpi label="Status">
          <Tag variant={invocationStatusVariant(invocation.status)}>{invocation.status}</Tag>
        </Kpi>
        <Kpi label="Duration" size={28}>
          {formatDuration(invocation.durationMs)}
        </Kpi>
        <Kpi label="Tokens" size={28}>
          {tokens > 0 ? tokens : '—'}
        </Kpi>
      </div>
      <div style={{ marginTop: 16 }}>
        {conversations.isLoading ? (
          <SectionStatus state="loading" />
        ) : conversations.error ? (
          <SectionStatus state="error" />
        ) : (
          <ConversationTranscript messages={messages} total={total} />
        )}
      </div>
    </Panel>
  )
}

export function EventView() {
  const { id } = useParams<{ id: string }>()
  const { data, isLoading, error } = useEvent(id ?? '')
  const invocations = useInvocations({ event: id, pageSize: 50 })

  return (
    <>
      <Titleblock
        crumbs={
          <>
            Operations / <Link to="/observability">Observability</Link> /{' '}
            <Link to="/observability/events">Events</Link> / <b>Detail</b>
          </>
        }
        title={<>Event <em>Detail</em></>}
      />
      <div style={{ padding: '28px 32px', display: 'grid', gap: 24 }}>
        {isLoading ? (
          <Panel className="cropped">
            <SectionStatus state="loading" />
          </Panel>
        ) : error ? (
          <Panel className="cropped">
            <SectionStatus state="error" />
          </Panel>
        ) : data ? (
          <>
            <div className="kpi-row" style={{ gridTemplateColumns: 'repeat(4, 1fr)' }}>
              <div className="kpi">
                <div className="hd">
                  <span className="label">When</span>
                </div>
                <div className="figure" style={{ fontSize: 18 }}>{formatISO(data.timestamp)}</div>
              </div>
              <div className="kpi">
                <div className="hd">
                  <span className="label">Connector</span>
                </div>
                <div className="figure" style={{ fontSize: 18 }}>{data.connector ?? '—'}</div>
              </div>
              <div className="kpi">
                <div className="hd">
                  <span className="label">Status</span>
                </div>
                <div className="figure">
                  <StatusTag status={data.status} />
                </div>
              </div>
              <div className="kpi">
                <div className="hd">
                  <span className="label">Event ID</span>
                </div>
                <div className="figure" style={{ fontSize: 14 }}>
                  {data.id}
                </div>
              </div>
            </div>

            {data.matches && data.matches.length > 0 ? (
              <Panel title="Matches" className="cropped">
                <div className="info-grid" style={{ gridTemplateColumns: '1fr 1fr' }}>
                  <div>
                    <div className="k">Trigger</div>
                    <div className="v">Agent</div>
                  </div>
                </div>
                {data.matches.map((m, i) => (
                  <div
                    key={i}
                    className="info-grid"
                    style={{
                      gridTemplateColumns: '1fr 1fr',
                      borderTop: '1px solid var(--border)',
                      paddingTop: 8,
                      marginTop: 8,
                    }}
                  >
                    <div>
                      <div className="v">{m.trigger}</div>
                    </div>
                    <div>
                      <div className="v">{m.agent}</div>
                    </div>
                  </div>
                ))}
              </Panel>
            ) : null}

            {invocations.isLoading ? (
              <Panel className="cropped">
                <SectionStatus state="loading" />
              </Panel>
            ) : invocations.error ? (
              <Panel className="cropped">
                <SectionStatus state="error" />
              </Panel>
            ) : (invocations.data?.items.length ?? 0) === 0 ? (
              <Panel title="Invocations" className="cropped">
                <div className="label" style={{ padding: '4px 0' }}>
                  No invocations recorded for this event.
                </div>
              </Panel>
            ) : (
              <>
                <div className="scroll-cap" style={{ display: 'grid', gap: 24 }}>
                  {invocations.data?.items.map((inv) => (
                    <InvocationDetail key={inv.id} invocation={inv} />
                  ))}
                </div>
                {(invocations.data?.total ?? 0) > (invocations.data?.items.length ?? 0) ? (
                  <div className="label" style={{ padding: '6px 2px', color: 'var(--ink-3)' }}>
                    Showing {invocations.data?.items.length} of {invocations.data?.total} invocations.
                  </div>
                ) : null}
              </>
            )}

            {data.payload != null ? (
              <Panel title="Payload" className="cropped">
                <div className="scroll-cap">
                  <pre className="code">
                    <code>{JSON.stringify(data.payload, null, 2)}</code>
                  </pre>
                </div>
              </Panel>
            ) : null}
          </>
        ) : null}
      </div>
      <ScrollToTop />
    </>
  )
}
