export function parseTimestamp(value?: string): Date | null {
  if (!value) return null
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? null : date
}

export function formatAbsoluteTimestamp(value?: string): string {
  if (!value) return '-'
  const date = parseTimestamp(value)
  return date ? date.toLocaleString() : value
}

export function formatRelativeTimestamp(value?: string, nowMs: number = Date.now()): string {
  if (!value) return '-'
  const date = parseTimestamp(value)
  if (!date) return value

  const diffMs = nowMs - date.getTime()
  const future = diffMs < 0
  const absSeconds = Math.floor(Math.abs(diffMs) / 1000)

  if (absSeconds < 30) return future ? 'in moments' : 'just now'

  const units: Array<{ limit: number; seconds: number; suffix: string }> = [
    { limit: 3600, seconds: 60, suffix: 'm' },
    { limit: 86400, seconds: 3600, suffix: 'h' },
    { limit: 604800, seconds: 86400, suffix: 'd' },
    { limit: Number.POSITIVE_INFINITY, seconds: 604800, suffix: 'w' }
  ]

  for (const unit of units) {
    if (absSeconds < unit.limit) {
      const valueRounded = Math.max(1, Math.round(absSeconds / unit.seconds))
      return future ? `in ${valueRounded}${unit.suffix}` : `${valueRounded}${unit.suffix} ago`
    }
  }

  return formatAbsoluteTimestamp(value)
}
