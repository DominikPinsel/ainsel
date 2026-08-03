export function abbreviateNumber(n: number, fractionDigits = 1): string {
  if (!Number.isFinite(n)) return String(n)
  const abs = Math.abs(n)
  const sign = n < 0 ? '-' : ''
  if (abs < 1000) return `${sign}${abs}`
  if (abs < 1_000_000)
    return `${sign}${(abs / 1000).toFixed(fractionDigits).replace(/\.0$/, '')}K`
  if (abs < 1_000_000_000)
    return `${sign}${(abs / 1_000_000).toFixed(fractionDigits).replace(/\.0$/, '')}M`
  return `${sign}${(abs / 1_000_000_000).toFixed(fractionDigits).replace(/\.0$/, '')}B`
}

export function formatPercent(p: number, fractionDigits = 1): string {
  if (!Number.isFinite(p)) return '—'
  return `${(p * 100).toFixed(fractionDigits)}%`
}
