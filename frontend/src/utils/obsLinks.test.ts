import { describe, expect, it } from 'vitest'
import { eventsLink, errorsLink, routingLink, tokensLink } from './obsLinks'

describe('eventsLink', () => {
  it('returns base path with no opts', () => {
    expect(eventsLink()).toBe('/observability/events')
  })

  it('appends status param', () => {
    expect(eventsLink({ status: 'error' })).toBe('/observability/events?status=error')
  })

  it('appends agent param', () => {
    expect(eventsLink({ agent: 'doc-writer' })).toBe('/observability/events?agent=doc-writer')
  })

  it('appends connector and range', () => {
    const url = eventsLink({ connector: 'abc', range: '7d' })
    expect(url).toContain('connector=abc')
    expect(url).toContain('range=7d')
  })

  it('omits params with undefined values', () => {
    const url = eventsLink({ status: undefined, connector: 'abc' })
    expect(url).not.toContain('status')
    expect(url).toContain('connector=abc')
  })

  it('encodes payload filters in q param', () => {
    const url = eventsLink({ filters: [{ field: 'action', op: 'eq', value: 'opened' }] })
    expect(url).toContain('q=')
    expect(url).toContain('action')
  })

  it('omits q param when filters array is empty', () => {
    const url = eventsLink({ filters: [] })
    expect(url).not.toContain('q=')
  })
})

describe('routingLink', () => {
  it('returns base path with no opts', () => {
    expect(routingLink()).toBe('/observability/routing')
  })

  it('appends agent param', () => {
    expect(routingLink({ agent: 'doc-writer' })).toBe('/observability/routing?agent=doc-writer')
  })

  it('appends trigger and range', () => {
    const url = routingLink({ trigger: 'on-push', range: '1h' })
    expect(url).toContain('trigger=on-push')
    expect(url).toContain('range=1h')
  })
})

describe('errorsLink', () => {
  it('returns base path with no opts', () => {
    expect(errorsLink()).toBe('/observability/errors')
  })

  it('appends severity', () => {
    expect(errorsLink({ severity: 'error' })).toBe('/observability/errors?severity=error')
  })

  it('appends source', () => {
    expect(errorsLink({ source: 'router' })).toBe('/observability/errors?source=router')
  })
})

describe('tokensLink', () => {
  it('returns base path with no opts', () => {
    expect(tokensLink()).toBe('/observability/tokens')
  })

  it('appends agent', () => {
    expect(tokensLink({ agent: 'triage-bot' })).toBe('/observability/tokens?agent=triage-bot')
  })

  it('appends range', () => {
    expect(tokensLink({ range: '7d' })).toBe('/observability/tokens?range=7d')
  })
})
