import { useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useEventsPage } from '../../api/events'
import type { ActivityEntry, ActivityStatus, RunStatus } from '../../api/events'
import { useConnectors } from '../../api/connectors'
import { useTriggers } from '../../api/triggers'
import { useAgents } from '../../api/agents'
import { Pager } from '../../primitives/Pager'
import { Panel } from '../../primitives/Panel'
import { Input } from '../../primitives/Input'
import { Select } from '../../primitives/Select'
import { Titleblock } from '../../layout/Titleblock'
import { ActivityRow } from './ActivityRow'

const STATUS_OPTIONS = [
  { value: 'matched', label: 'Matched' },
  { value: 'unmatched', label: 'Unmatched' },
  { value: 'error', label: 'Error' },
] as const

// Outcome filter values. These mirror the RunStatus union reported per
// match (see frontend/src/api/events.ts); an event matches the filter when
// any of its matches has the selected outcome.
const OUTCOME_OPTIONS = [
  { value: 'success', label: 'Success' },
  { value: 'failure', label: 'Failure' },
  { value: 'timeout', label: 'Timeout' },
  { value: 'running', label: 'Running' },
] as const

const PAGE_SIZE_OPTIONS = [25, 50, 100] as const

export function Activity() {
  const [filterStatus, setFilterStatus] = useState<string>('')
  const [filterConnector, setFilterConnector] = useState<string>('')
  const [filterAgent, setFilterAgent] = useState<string>('')
  const [filterOutcome, setFilterOutcome] = useState<string>('')
  const [expanded, setExpanded] = useState<Set<string>>(new Set())

  const [params, setParams] = useSearchParams()
  const searchQuery = params.get('q') ?? ''
  const page = Math.max(1, Number(params.get('page')) || 1)
  const pageSizeParam = Number(params.get('pageSize'))
  const pageSize = (PAGE_SIZE_OPTIONS as readonly number[]).includes(pageSizeParam)
    ? pageSizeParam
    : 25

  // Pagination and the status/connector/agent filters are server-side so
  // the full event history is queryable, not just the most recent window.
  // Outcome and free-text search (which rely on invocation state and
  // resolved display names) still apply client-side to the loaded page.
  const { data, isLoading, error } = useEventsPage({
    limit: pageSize,
    offset: (page - 1) * pageSize,
    status: filterStatus ? (filterStatus as ActivityStatus) : undefined,
    connector: filterConnector || undefined,
    agent: filterAgent || undefined,
  })
  const { data: connectorData } = useConnectors({ pageSize: 200 })
  const { data: triggerData } = useTriggers({ pageSize: 200 })
  const { data: agentData } = useAgents({ pageSize: 200 })

  const rows = useMemo(() => data?.events ?? [], [data])
  const total = data?.total ?? 0

  const setPage = (p: number) => {
    setParams((prev) => { const n = new URLSearchParams(prev); n.set('page', String(p)); return n }, { replace: true })
  }

  const setPageSize = (n: number) => {
    setParams((prev) => { const next = new URLSearchParams(prev); next.set('pageSize', String(n)); next.set('page', '1'); return next }, { replace: true })
  }

  const handleSearchChange = (v: string) => {
    setParams((prev) => {
      const n = new URLSearchParams(prev)
      if (v) n.set('q', v)
      else n.delete('q')
      n.set('page', '1')
      return n
    }, { replace: true })
  }

  const handleFilterStatus = (v: string) => {
    setFilterStatus(v)
    setParams((prev) => { const n = new URLSearchParams(prev); n.set('page', '1'); return n }, { replace: true })
  }

  const handleFilterConnector = (v: string) => {
    setFilterConnector(v)
    setParams((prev) => { const n = new URLSearchParams(prev); n.set('page', '1'); return n }, { replace: true })
  }

  const handleFilterAgent = (v: string) => {
    setFilterAgent(v)
    setParams((prev) => { const n = new URLSearchParams(prev); n.set('page', '1'); return n }, { replace: true })
  }

  const handleFilterOutcome = (v: string) => {
    setFilterOutcome(v)
    setParams((prev) => { const n = new URLSearchParams(prev); n.set('page', '1'); return n }, { replace: true })
  }

  const toggleExpanded = (id: string) =>
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })

  const connectorNameById = useMemo(() => {
    const map = new Map<string, string>()
    for (const c of connectorData?.items ?? []) {
      map.set(c.id, c.name)
    }
    return map
  }, [connectorData])

  const triggerNameById = useMemo(() => {
    const map = new Map<string, string>()
    for (const t of triggerData?.items ?? []) {
      map.set(t.id, t.name)
    }
    return map
  }, [triggerData])

  const agentNameById = useMemo(() => {
    const map = new Map<string, string>()
    for (const a of agentData?.items ?? []) {
      map.set(a.id, a.name)
    }
    return map
  }, [agentData])

  const connectorOptions = useMemo(
    () =>
      (connectorData?.items ?? [])
        .map((c) => ({ value: c.id, label: c.name }))
        .sort((a, b) => a.label.localeCompare(b.label)),
    [connectorData],
  )

  const agentOptions = useMemo(
    () =>
      (agentData?.items ?? [])
        .map((a) => ({ value: a.id, label: a.name }))
        .sort((a, b) => a.label.localeCompare(b.label)),
    [agentData],
  )

  const searchTerms = useMemo(
    () => searchQuery.toLowerCase().split(/\s+/).filter(Boolean),
    [searchQuery],
  )

  const filtered: ActivityEntry[] = useMemo(() => {
    const buildHaystack = (r: ActivityEntry): string => {
      const parts: string[] = []
      if (r.connector) {
        parts.push(r.connector)
        const name = connectorNameById.get(r.connector)
        if (name) parts.push(name)
      }
      if (r.status) parts.push(r.status)
      for (const m of r.matches ?? []) {
        if (m.trigger) {
          parts.push(m.trigger)
          const tn = triggerNameById.get(m.trigger)
          if (tn) parts.push(tn)
        }
        if (m.agent) {
          parts.push(m.agent)
          const an = agentNameById.get(m.agent)
          if (an) parts.push(an)
        }
        if (m.runStatus) parts.push(m.runStatus)
      }
      if (r.payload != null) {
        try { parts.push(JSON.stringify(r.payload)) } catch { /* ignore */ }
      }
      return parts.join(' ').toLowerCase()
    }

    return rows.filter((r) => {
      if (filterOutcome && !(r.matches ?? []).some((m) => m.runStatus === (filterOutcome as RunStatus))) return false
      if (searchTerms.length > 0) {
        const haystack = buildHaystack(r)
        if (!searchTerms.every((t) => haystack.includes(t))) return false
      }
      return true
    })
  }, [rows, filterOutcome, searchTerms, connectorNameById, triggerNameById, agentNameById])

  const sorted: ActivityEntry[] = useMemo(
    () =>
      [...filtered].sort(
        (a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime(),
      ),
    [filtered],
  )

  const localFilterActive = filterOutcome !== '' || searchTerms.length > 0
  const anyFilterActive =
    localFilterActive || filterStatus !== '' || filterConnector !== '' || filterAgent !== ''
  const panelTitle = localFilterActive
    ? `Activity · ${sorted.length} of ${rows.length} on page · ${total} total`
    : `Activity · ${total} total`

  return (
    <>
      <Titleblock
        crumbs={
          <>
            Operations / <b>Activity</b>
          </>
        }
        title={
          <>
            Activity <em>Stream</em>
          </>
        }
      />
      <div style={{ padding: '28px 32px' }}>
        <div className="toolbar">
          <div className="field">
            <span className="label">Search</span>
            <Input
              aria-label="Search events"
              placeholder="e.g. pull_request 123"
              value={searchQuery}
              onChange={(e) => handleSearchChange(e.target.value)}
            />
          </div>
          <div className="field">
            <span className="label">Status</span>
            <Select
              aria-label="Filter by status"
              value={filterStatus}
              onChange={handleFilterStatus}
              options={STATUS_OPTIONS}
              emptyLabel="Any status"
            />
          </div>
          <div className="field">
            <span className="label">Outcome</span>
            <Select
              aria-label="Filter by outcome"
              value={filterOutcome}
              onChange={handleFilterOutcome}
              options={OUTCOME_OPTIONS}
              emptyLabel="Any outcome"
            />
          </div>
          <div className="field">
            <span className="label">Connector</span>
            <Select
              aria-label="Filter by connector"
              value={filterConnector}
              onChange={handleFilterConnector}
              options={connectorOptions}
              emptyLabel="Any connector"
            />
          </div>
          <div className="field">
            <span className="label">Agent</span>
            <Select
              aria-label="Filter by agent"
              value={filterAgent}
              onChange={handleFilterAgent}
              options={agentOptions}
              emptyLabel="Any agent"
            />
          </div>
        </div>

        <Panel
          title={panelTitle}
          className="cropped"
        >
          {isLoading ? (
            <div className="label" style={{ padding: 14 }}>
              Loading…
            </div>
          ) : error ? (
            <div className="label" style={{ padding: 14, color: 'var(--signal)' }}>
              Failed to load activity.
            </div>
          ) : sorted.length === 0 ? (
            <div className="label" style={{ padding: 14 }}>
              {rows.length === 0
                ? anyFilterActive
                  ? 'No events match the filter.'
                  : total === 0
                    ? 'No recent events.'
                    : 'No events on this page.'
                : 'No events match the filter.'}
            </div>
          ) : (
            <>
              <div className="reg-wrap">
                <table className="reg">
                  <thead>
                    <tr>
                      <th>When</th>
                      <th>Connector</th>
                      <th>Trigger</th>
                      <th>Agent</th>
                      <th>Status</th>
                    </tr>
                  </thead>
                  <tbody>
                    {sorted.map((e) => (
                      <ActivityRow
                        key={e.id}
                        entry={e}
                        connectorName={e.connector ? connectorNameById.get(e.connector) : undefined}
                        triggerNameById={triggerNameById}
                        agentNameById={agentNameById}
                        expanded={expanded.has(e.id)}
                        onToggle={() => toggleExpanded(e.id)}
                      />
                    ))}
                  </tbody>
                </table>
              </div>
              <Pager
                page={page}
                pageSize={pageSize}
                total={total}
                pageSizeOptions={PAGE_SIZE_OPTIONS}
                onPageChange={setPage}
                onPageSizeChange={setPageSize}
              />
            </>
          )}
        </Panel>
      </div>
    </>
  )
}
