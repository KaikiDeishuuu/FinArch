const UINT32_MAX_PLUS_ONE = 0x1_0000_0000

export function secureRandomHex(bytes = 16): string {
  if (!Number.isInteger(bytes) || bytes <= 0) {
    throw new Error('bytes must be a positive integer')
  }

  const buffer = new Uint8Array(bytes)
  crypto.getRandomValues(buffer)
  return Array.from(buffer, (b) => b.toString(16).padStart(2, '0')).join('')
}

export function secureRandomInt(maxExclusive: number): number {
  if (!Number.isInteger(maxExclusive) || maxExclusive <= 0) {
    throw new Error('maxExclusive must be a positive integer')
  }

  const cutoff = Math.floor(UINT32_MAX_PLUS_ONE / maxExclusive) * maxExclusive
  const buf = new Uint32Array(1)

  while (true) {
    crypto.getRandomValues(buf)
    const candidate = buf[0]
    if (candidate < cutoff) {
      return candidate % maxExclusive
    }
  }
}
