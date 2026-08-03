import { useMemo, useState } from 'react'
import { ServiceUnavailableError } from '../../api/client'
import type { LogEntry, Range } from '../../api/observability'
import { useObservabilityLogs } from '../../api/observability'
import { Check } from '../../primitives/Check'
import { Panel } from '../../primitives/Panel'
import { SectionStatus, type SectionState } from '../../primitives/SectionStatus'
import { Select } from '../../primitives/Select'
import { formatRelative } from '../../utils/time'

const LEVEL_OPTIONS = [
  { value: 'debug', label: 'Debug' },
  { value: 'info', label: 'Info' },
  { value: 'warn', label: 'Warn' },
  { value: 'error', label: 'Error' },
] as const

type LogStreamProps = { range: Range }

function deriveState(query: ReturnType<typeof useObservabilityLogs>): SectionState {
  if (query.isLoading) return 'loading'
  if (query.error instanceof ServiceUnavailableError) return 'unavailable'
  if (query.error) return 'error'
  return 'ready'
}

export function LogStream({ range }: LogStreamProps) {
  const [filterAgent, setFilterAgent] = useState('')
  const [filterLevel, setFilterLevel] = useState('')
  const [autoRefresh, setAutoRefresh] = useState(false)

  const query = useObservabilityLogs(
    {
      range,
      agent: filterAgent || undefined,
      limit: 200,
    },
    { refetchInterval: autoRefresh ? 10_000 : false },
  )

  const state = deriveState(query)
  const rows = useMemo(() => query.data ?? [], [query.data])

  const agentOptions = useMemo(
    () =>
      Array.from(
        new Set(rows.map((r) => r.agent).filter((v): v is string => !!v)),
      )
        .sort()
        .map((v) => {
          const label = rows.find((r) => r.agent === v)?.agentName ?? v
          return { value: v, label }
        }),
    [rows],
  )

  const filtered: LogEntry[] = useMemo(
    () =>
      filterLevel
        ? rows.filter((r) => r.level === filterLevel)
        : rows,
    [rows, filterLevel],
  )

  return (
    <Panel
      title="Logs"
      right={
        <label className="auto-refresh-toggle">
          <Check
            checked={autoRefresh}
            onChange={setAutoRefresh}
            aria-label="Auto-refresh logs every 10 seconds"
          />
          Auto-refresh · 10s
        </label>
      }
      className="cropped"
    >
      <div
        className="toolbar"
        style={{ borderBottom: 0, padding: '10px 14px 14px' }}
      >
        <div className="field">
          <span className="label">Agent</span>
          <Select
            aria-label="Filter logs by agent"
            value={filterAgent}
            onChange={setFilterAgent}
            options={agentOptions}
            emptyLabel="Any agent"
          />
        </div>
        <div className="field">
          <span className="label">Level</span>
          <Select
            aria-label="Filter logs by level"
            value={filterLevel}
            onChange={setFilterLevel}
            options={LEVEL_OPTIONS}
            emptyLabel="Any level"
          />
        </div>
      </div>
      <SectionStatus
        state={state}
        detail={
          state === 'error' && query.error instanceof Error ? query.error.message : undefined
        }
        onRetry={() => query.refetch()}
      />
      {state === 'ready' ? (
        filtered.length === 0 ? (
          <div className="label" style={{ padding: 14 }}>
            No log entries.
          </div>
        ) : (
          <div style={{ maxHeight: 420, overflowY: 'auto' }}>
            {filtered.map((row, i) => (
              <div key={`${row.timestamp}-${i}`} className="log-row">
                <span className="ts">{formatRelative(row.timestamp)}</span>
                <span className={`lvl ${row.level ?? 'unknown'}`}>{row.level ?? '—'}</span>
                <span className="app">
                  {row.app ?? '—'}
                  {row.agent ? ` · ${row.agentName ?? row.agent}` : ''}
                </span>
                <span className="msg">{row.message}</span>
              </div>
            ))}
          </div>
        )
      ) : null}
    </Panel>
  )
}