export function formatRelative(
  input: string | null | undefined,
  nowMs: number = Date.now(),
): string {
  if (!input) return '—'
  const t = Date.parse(input)
  if (Number.isNaN(t)) return '—'
  const delta = Math.max(0, nowMs - t)
  const s = Math.floor(delta / 1000)
  if (s < 5) return 'just now'
  if (s < 60) return `${s}s ago`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m ago`
  const h = Math.floor(m / 60)
  const remMin = m % 60
  if (h < 24) return `${h}h ${remMin}m`
  const d = Math.floor(h / 24)
  const remHour = h % 24
  return `${d}d ${remHour}h`
}

export function formatISO(input: string | null | undefined): string {
  if (!input) return '—'
  const t = Date.parse(input)
  if (Number.isNaN(t)) return '—'
  const d = new Date(t)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}-${pad(d.getUTCDate())} ${pad(
    d.getUTCHours(),
  )}:${pad(d.getUTCMinutes())}:${pad(d.getUTCSeconds())}Z`
}
