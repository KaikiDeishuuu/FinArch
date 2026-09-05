/**
 * A fresh key is safe only when the server definitively rejected the request
 * before creating the resource. Timeouts, throttling and server/proxy failures
 * leave the outcome unknown, so their retries must keep the original key.
 */
export function shouldRotateIdempotencyKey(error: unknown): boolean {
  const status = (error as { response?: { status?: unknown } } | null)?.response?.status
  if (typeof status !== 'number' || status < 400 || status >= 500) return false
  return status !== 408 && status !== 425 && status !== 429
}
