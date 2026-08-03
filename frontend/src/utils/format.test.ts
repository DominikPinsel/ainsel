import { describe, expect, it } from 'vitest'
import { abbreviateNumber, formatPercent } from './format'

describe('abbreviateNumber', () => {
  it('returns the raw number under 1000', () => {
    expect(abbreviateNumber(0)).toBe('0')
    expect(abbreviateNumber(999)).toBe('999')
  })

  it('thousands → K', () => {
    expect(abbreviateNumber(1000)).toBe('1K')
    expect(abbreviateNumber(1234)).toBe('1.2K')
    expect(abbreviateNumber(12_400)).toBe('12.4K')
  })

  it('millions → M', () => {
    expect(abbreviateNumber(1_200_000)).toBe('1.2M')
  })

  it('billions → B', () => {
    expect(abbreviateNumber(3_000_000_000)).toBe('3B')
  })

  it('negatives', () => {
    expect(abbreviateNumber(-12_400)).toBe('-12.4K')
  })

  it('handles non-finite', () => {
    expect(abbreviateNumber(Number.NaN)).toBe('NaN')
  })
})

describe('formatPercent', () => {
  it('multiplies by 100 with one decimal', () => {
    expect(formatPercent(0.021)).toBe('2.1%')
  })

  it('—  for non-finite', () => {
    expect(formatPercent(Number.NaN)).toBe('—')
  })
})
