export function parseApiTimestamp(value?: string | null): Date | null {
  if (!value) return null
  const normalized = value.includes('T') ? value : value.replace(' ', 'T')
  const parsed = new Date(normalized)
  if (Number.isNaN(parsed.getTime())) return null
  return parsed
}

function pad(value: number): string {
  return String(value).padStart(2, '0')
}

export function formatDateTimeLocal(value?: string | null): string | null {
  const parsed = parseApiTimestamp(value)
  if (!parsed) return null
  return `${parsed.getFullYear()}-${pad(parsed.getMonth() + 1)}-${pad(parsed.getDate())} ${pad(parsed.getHours())}:${pad(parsed.getMinutes())}:${pad(parsed.getSeconds())}`
}

export function clampLifecycleTimestamp(
  value: string | null | undefined,
  createdAt: string | null | undefined,
): string | null {
  const lifecycleDate = parseApiTimestamp(value)
  if (!lifecycleDate) return null

  const createdDate = parseApiTimestamp(createdAt)
  if (createdDate && lifecycleDate.getTime() < createdDate.getTime()) {
    return formatDateTimeLocal(createdAt)
  }

  return formatDateTimeLocal(value)
}
