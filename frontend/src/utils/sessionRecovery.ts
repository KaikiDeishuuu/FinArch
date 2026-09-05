type ApiFailure = {
  response?: {
    status?: number
    data?: unknown
  }
}

const ACCESS_RECOVERY_CODES = new Set([
  'access_expired',
  'access_invalid',
  'session_invalid',
  // Compatibility with responses emitted by older FinArch releases.
  '40101',
  'auth_invalid_token',
  'invalid_session',
])

const DEFINITIVE_SESSION_INVALID_CODES = new Set([
  'session_invalid',
  // Compatibility with the early browser-session E2E contract.
  'invalid_session',
])

export const SESSION_RECOVERY_MAX_ATTEMPTS = 5
const SESSION_RECOVERY_BASE_DELAY_MS = 500
const SESSION_RECOVERY_MAX_DELAY_MS = 8_000

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

export function getApiFailureCode(error: unknown): string | null {
  const data = (error as ApiFailure | null | undefined)?.response?.data
  if (!isRecord(data)) return null

  const nestedError = data.error
  const rawCode = isRecord(nestedError) ? nestedError.code : data.code
  if (typeof rawCode !== 'string' && typeof rawCode !== 'number') return null
  return String(rawCode)
}

function getApiFailureStatus(error: unknown): number | undefined {
  return (error as ApiFailure | null | undefined)?.response?.status
}

/** Whether a protected request failed specifically because its access session is unusable. */
export function isRefreshableAccessFailure(error: unknown): boolean {
  return getApiFailureStatus(error) === 401 && ACCESS_RECOVERY_CODES.has(getApiFailureCode(error) ?? '')
}

/** Whether the refresh endpoint explicitly confirmed that no browser session remains. */
export function isDefinitiveSessionInvalid(error: unknown): boolean {
  return getApiFailureStatus(error) === 401 && DEFINITIVE_SESSION_INVALID_CODES.has(getApiFailureCode(error) ?? '')
}

/** Capped exponential delay after the given consecutive transient failure. */
export function getSessionRecoveryDelay(failureCount: number): number {
  const exponent = Math.max(0, Math.floor(failureCount) - 1)
  return Math.min(SESSION_RECOVERY_BASE_DELAY_MS * (2 ** exponent), SESSION_RECOVERY_MAX_DELAY_MS)
}
