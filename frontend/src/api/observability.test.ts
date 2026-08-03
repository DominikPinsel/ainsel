import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  getObservabilityLogs,
  getObservabilitySummary,
  getObservabilityTimeseries,
  getTokensByEvent,
  getTokensBySubject,
  getTokensSummary,
} from './observability'

const mockFetch = () => globalThis.fetch as ReturnType<typeof vi.fn>

describe('api/observability', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn())
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('summary GETs /observability/metrics/summary', async () => {
    mockFetch().mockResolvedValue(
      new Response(
        JSON.stringify({
          eventsConsumed: 100,
          triggersMatched: 90,
          eventsRouted: 88,
          routingErrors: 3,
          updatedAt: '2026-05-21T00:00:00Z',
        }),
        { status: 200 },
      ),
    )
    const out = await getObservabilitySummary()
    const [url] = mockFetch().mock.calls[0]
    expect(url).toContain('/observability/metrics/summary')
    expect(out.routingErrors).toBe(3)
  })

  it('timeseries GETs with range + metric params', async () => {
    mockFetch().mockResolvedValue(
      new Response(
        JSON.stringify({
          metric: 'events_routed',
          range: '24h',
          step: '1h',
          points: [],
        }),
        { status: 200 },
      ),
    )
    await getObservabilityTimeseries({ range: '24h', metric: 'events_routed' })
    const [url] = mockFetch().mock.calls[0]
    expect(url).toContain('/observability/metrics/timeseries')
    expect(url).toContain('range=24h')
    expect(url).toContain('metric=events_routed')
  })

  it('tokens summary GETs /tokens/summary', async () => {
    mockFetch().mockResolvedValue(
      new Response(
        JSON.stringify({
          totalTokens: 1200000,
          inputTokens: 800000,
          outputTokens: 400000,
        }),
        { status: 200 },
      ),
    )
    const out = await getTokensSummary()
    expect(mockFetch().mock.calls[0][0]).toContain('/observability/metrics/tokens/summary')
    expect(out.totalTokens).toBe(1200000)
  })

  it('summary passes range param when provided', async () => {
    mockFetch().mockResolvedValue(
      new Response(
        JSON.stringify({
          eventsConsumed: 10,
          triggersMatched: 8,
          eventsRouted: 7,
          routingErrors: 0,
          updatedAt: '2026-05-21T00:00:00Z',
        }),
        { status: 200 },
      ),
    )
    await getObservabilitySummary('6h')
    const [url] = mockFetch().mock.calls[0]
    expect(url).toContain('/observability/metrics/summary')
    expect(url).toContain('range=6h')
  })

  it('tokens summary passes range param when provided', async () => {
    mockFetch().mockResolvedValue(
      new Response(
        JSON.stringify({
          totalTokens: 500,
          inputTokens: 300,
          outputTokens: 200,
        }),
        { status: 200 },
      ),
    )
    await getTokensSummary('1h')
    const [url] = mockFetch().mock.calls[0]
    expect(url).toContain('/observability/metrics/tokens/summary')
    expect(url).toContain('range=1h')
  })

  it('tokens by-subject GETs with range param', async () => {
    mockFetch().mockResolvedValue(
      new Response(JSON.stringify({ range: '24h', rows: [] }), { status: 200 }),
    )
    await getTokensBySubject('24h')
    const [url] = mockFetch().mock.calls[0]
    expect(url).toContain('/observability/metrics/tokens/by-subject')
    expect(url).toContain('range=24h')
  })

  it('tokens by-event GETs with range param', async () => {
    mockFetch().mockResolvedValue(
      new Response(JSON.stringify({ range: '24h', rows: [] }), { status: 200 }),
    )
    await getTokensByEvent('24h')
    const [url] = mockFetch().mock.calls[0]
    expect(url).toContain('/observability/metrics/tokens/by-event')
    expect(url).toContain('range=24h')
  })

  it('logs GETs /observability/logs with filter params', async () => {
    mockFetch().mockResolvedValue(
      new Response(JSON.stringify({ logs: [], total: 0, query: '' }), { status: 200 }),
    )
    await getObservabilityLogs({ app: 'hub', range: '6h', limit: 200 })
    const [url] = mockFetch().mock.calls[0]
    expect(url).toContain('/observability/logs')
    expect(url).toContain('app=hub')
    expect(url).toContain('range=6h')
    expect(url).toContain('limit=200')
  })

  it('normalizes labels into top-level fields on log entries', async () => {
    mockFetch().mockResolvedValue(
      new Response(
        JSON.stringify({
          logs: [
            {
              timestamp: '2026-07-09T07:00:00Z',
              message: 'agent started',
              agentName: 'Doc Writer',
              labels: { agent: 'doc-writer', app: 'doc-writer', level: 'info' },
            },
            {
              timestamp: '2026-07-09T07:01:00Z',
              message: 'no labels here',
            },
          ],
          total: 2,
          query: '',
        }),
        { status: 200 },
      ),
    )
    const logs = await getObservabilityLogs({ range: '1h' })
    expect(logs).toHaveLength(2)

    // First entry: labels should be promoted to top-level fields
    expect(logs[0].agent).toBe('doc-writer')
    expect(logs[0].app).toBe('doc-writer')
    expect(logs[0].level).toBe('info')
    expect(logs[0].agentName).toBe('Doc Writer')

    // Second entry: no labels, fields stay undefined
    expect(logs[1].agent).toBeUndefined()
    expect(logs[1].app).toBeUndefined()
    expect(logs[1].level).toBeUndefined()
  })
})