import { secureRandomHex } from './secureRandom'

export const SESSION_REQUEST_TIMEOUT_MS = 8_000
export const SESSION_STORAGE_LOCK_PREFIX = 'finarch:session-cookie-lock:v1:'
export const SESSION_STORAGE_LOCK_LEASE_MS = SESSION_REQUEST_TIMEOUT_MS * 4
export const SESSION_STORAGE_LOCK_POLL_MS = 25

export interface AuthSessionPayload {
  token: string
  expires_at: string
  user_id: string
  email: string
  username: string
  nickname: string
  role: string
}

const RFC3339_PATTERN = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(?:Z|([+-])(\d{2}):(\d{2}))$/

function isLeapYear(year: number): boolean {
  return year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0)
}

function isValidRFC3339(value: string): boolean {
  const match = RFC3339_PATTERN.exec(value)
  if (!match) return false

  const year = Number(match[1])
  const month = Number(match[2])
  const day = Number(match[3])
  const hour = Number(match[4])
  const minute = Number(match[5])
  const second = Number(match[6])
  const offsetHour = match[8] === undefined ? 0 : Number(match[8])
  const offsetMinute = match[9] === undefined ? 0 : Number(match[9])
  const daysInMonth = [31, isLeapYear(year) ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31]

  return year > 0 &&
    month >= 1 && month <= 12 &&
    day >= 1 && day <= daysInMonth[month - 1] &&
    hour <= 23 && minute <= 59 && second <= 59 &&
    offsetHour <= 23 && offsetMinute <= 59 &&
    Number.isFinite(Date.parse(value))
}

export function parseAuthSessionPayload(payload: unknown, nowMs = Date.now()): AuthSessionPayload {
  if (!payload || typeof payload !== 'object' || Array.isArray(payload)) {
    throw new Error('Invalid authentication response')
  }

  const value = payload as Partial<AuthSessionPayload>
  const required: Array<keyof AuthSessionPayload> = [
    'token',
    'expires_at',
    'user_id',
    'email',
    'username',
    'nickname',
    'role',
  ]
  if (required.some((field) => typeof value[field] !== 'string' || value[field].trim() === '')) {
    throw new Error('Invalid authentication response')
  }

  const expiresAt = value.expires_at as string
  const parsedExpiry = Date.parse(expiresAt)
  if (!isValidRFC3339(expiresAt) || parsedExpiry <= nowMs) {
    throw new Error('Invalid authentication response')
  }
  return value as AuthSessionPayload
}

function timeoutError(): DOMException {
  return new DOMException('Session request timed out', 'TimeoutError')
}

export class SessionCookieLockUnavailableError extends Error {
  constructor(message = 'Session cookie lock is unavailable') {
    super(message)
    this.name = 'SessionCookieLockUnavailableError'
  }
}

export interface SessionLockStorage {
  readonly length: number
  key(index: number): string | null
  getItem(key: string): string | null
  setItem(key: string, value: string): void
  removeItem(key: string): void
}

export interface StorageSessionLockOptions {
  ownerId?: string
  leaseMs?: number
  pollIntervalMs?: number
  now?: () => number
}

interface StorageBakeryRecord {
  owner: string
  ticket: number
  choosing: boolean
  expiresAt: number
}

interface ActiveStorageBakeryRecord {
  key: string
  value: StorageBakeryRecord
}

interface StorageBakerySnapshot {
  stable: boolean
  allKeys: string[]
  records: Map<string, string>
}

const SESSION_STORAGE_SNAPSHOT_MAX_ATTEMPTS = 8

let sessionLockOwnerSequence = 0

function createSessionLockOwner(): string {
  sessionLockOwnerSequence += 1
  try {
    return secureRandomHex(16) + '-' + sessionLockOwnerSequence
  } catch {
    throw new SessionCookieLockUnavailableError('Secure randomness is unavailable for the session cookie lock')
  }
}

function lockStoragePrefix(name: string): string {
  return SESSION_STORAGE_LOCK_PREFIX + encodeURIComponent(name) + ':'
}

function isStorageBakeryRecord(value: unknown): value is StorageBakeryRecord {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const record = value as Partial<StorageBakeryRecord>
  return typeof record.owner === 'string' && record.owner.length > 0 &&
    Number.isSafeInteger(record.ticket) && (record.ticket ?? -1) >= 0 &&
    typeof record.choosing === 'boolean' &&
    typeof record.expiresAt === 'number' && Number.isFinite(record.expiresAt)
}

function writeStorageBakeryRecord(
  storage: SessionLockStorage,
  key: string,
  record: StorageBakeryRecord,
): void {
  try {
    storage.setItem(key, JSON.stringify(record))
  } catch {
    throw new SessionCookieLockUnavailableError()
  }
}

function captureStorageBakerySnapshot(
  storage: SessionLockStorage,
  prefix: string,
): StorageBakerySnapshot {
  const keys = new Set<string>()
  let lengthBefore: number
  let lengthAfter: number
  try {
    lengthBefore = storage.length
    for (let index = 0; index < lengthBefore; index += 1) {
      const key = storage.key(index)
      if (key !== null) keys.add(key)
    }
    // A removal can shift an unvisited key to a lower index. The reverse pass
    // plus the stable double-snapshot check prevents that key being skipped.
    for (let index = storage.length - 1; index >= 0; index -= 1) {
      const key = storage.key(index)
      if (key !== null) keys.add(key)
    }
    lengthAfter = storage.length
  } catch {
    throw new SessionCookieLockUnavailableError()
  }

  const allKeys = [...keys].sort()
  const records = new Map<string, string>()
  for (const key of allKeys) {
    if (!key.startsWith(prefix)) continue
    try {
      const raw = storage.getItem(key)
      if (raw !== null) records.set(key, raw)
    } catch {
      throw new SessionCookieLockUnavailableError()
    }
  }
  let finalLength: number
  try {
    finalLength = storage.length
  } catch {
    throw new SessionCookieLockUnavailableError()
  }
  return {
    stable: lengthBefore === lengthAfter && lengthAfter === finalLength && allKeys.length === finalLength,
    allKeys,
    records,
  }
}

function sameStorageBakerySnapshot(left: StorageBakerySnapshot, right: StorageBakerySnapshot): boolean {
  if (!left.stable || !right.stable || left.allKeys.length !== right.allKeys.length || left.records.size !== right.records.size) {
    return false
  }
  for (let index = 0; index < left.allKeys.length; index += 1) {
    if (left.allKeys[index] !== right.allKeys[index]) return false
  }
  for (const [key, raw] of left.records) {
    if (right.records.get(key) !== raw) return false
  }
  return true
}

function readStableStorageBakerySnapshot(
  storage: SessionLockStorage,
  prefix: string,
): StorageBakerySnapshot {
  let previous: StorageBakerySnapshot | null = null
  for (let attempt = 0; attempt < SESSION_STORAGE_SNAPSHOT_MAX_ATTEMPTS; attempt += 1) {
    const current = captureStorageBakerySnapshot(storage, prefix)
    if (previous && sameStorageBakerySnapshot(previous, current)) return current
    previous = current.stable ? current : null
  }
  throw new SessionCookieLockUnavailableError('Session cookie lock storage is changing too quickly')
}

function readActiveStorageBakeryRecords(
  storage: SessionLockStorage,
  prefix: string,
  now: number,
): ActiveStorageBakeryRecord[] {
  const snapshot = readStableStorageBakerySnapshot(storage, prefix)
  const active: ActiveStorageBakeryRecord[] = []
  for (const [key, raw] of snapshot.records) {

    let parsed: unknown
    try {
      parsed = JSON.parse(raw)
    } catch {
      throw new SessionCookieLockUnavailableError('Session cookie lock data is invalid')
    }
    if (!isStorageBakeryRecord(parsed)) {
      throw new SessionCookieLockUnavailableError('Session cookie lock data is invalid')
    }
    if (parsed.expiresAt <= now) {
      try {
        // Do not remove a lease that was renewed after this read.
        if (storage.getItem(key) === raw) storage.removeItem(key)
      } catch {
        throw new SessionCookieLockUnavailableError()
      }
      continue
    }
    active.push({ key, value: parsed })
  }
  return active
}

function removeOwnedStorageBakeryRecord(
  storage: SessionLockStorage,
  key: string,
  owner: string,
): void {
  try {
    const raw = storage.getItem(key)
    if (raw === null) return
    const parsed: unknown = JSON.parse(raw)
    if (isStorageBakeryRecord(parsed) && parsed.owner === owner) {
      storage.removeItem(key)
    }
  } catch {
    // A failed release leaves only a short-lived non-secret lease. A later
    // contender reclaims it after expiresAt rather than risking overlap.
  }
}

function abortableLockDelay(delayMs: number, signal: AbortSignal): Promise<void> {
  signal.throwIfAborted()
  return new Promise((resolve, reject) => {
    const timer = globalThis.setTimeout(() => {
      signal.removeEventListener('abort', abort)
      resolve()
    }, delayMs)
    const abort = () => {
      globalThis.clearTimeout(timer)
      reject(signal.reason ?? new DOMException('Aborted', 'AbortError'))
    }
    signal.addEventListener('abort', abort, { once: true })
  })
}

function storageBakeryRecordPrecedes(
  candidate: StorageBakeryRecord,
  ticket: number,
  owner: string,
): boolean {
  return candidate.ticket > 0 &&
    (candidate.ticket < ticket || (candidate.ticket === ticket && candidate.owner < owner))
}

/**
 * Cross-context fallback for browsers without Web Locks.
 *
 * This is Lamport's bakery algorithm over one localStorage record per
 * contender. Records contain only a random owner, ticket and lease deadline;
 * no cookie, token or user data is persisted. The caller must bound the whole
 * operation with an AbortSignal deadline shorter than the lease.
 */
export async function withStorageSessionLock<T>(
  storage: SessionLockStorage,
  name: string,
  signal: AbortSignal,
  operation: () => Promise<T>,
  options: StorageSessionLockOptions = {},
): Promise<T> {
  signal.throwIfAborted()
  const owner = options.ownerId ?? createSessionLockOwner()
  if (owner.trim() === '') throw new SessionCookieLockUnavailableError()
  const leaseMs = Math.max(1, options.leaseMs ?? SESSION_STORAGE_LOCK_LEASE_MS)
  const pollIntervalMs = Math.max(1, options.pollIntervalMs ?? SESSION_STORAGE_LOCK_POLL_MS)
  const now = options.now ?? Date.now
  const prefix = lockStoragePrefix(name)
  const key = prefix + encodeURIComponent(owner)
  let record: StorageBakeryRecord = {
    owner,
    ticket: 0,
    choosing: true,
    expiresAt: now() + leaseMs,
  }

  writeStorageBakeryRecord(storage, key, record)
  try {
    const contenders = readActiveStorageBakeryRecords(storage, prefix, now())
    const maxTicket = contenders.reduce(
      (max, contender) => Math.max(max, contender.value.ticket),
      0,
    )
    if (maxTicket >= Number.MAX_SAFE_INTEGER) {
      throw new SessionCookieLockUnavailableError('Session cookie lock ticket space is exhausted')
    }
    record = {
      owner,
      ticket: maxTicket + 1,
      choosing: false,
      expiresAt: now() + leaseMs,
    }
    writeStorageBakeryRecord(storage, key, record)

    for (;;) {
      signal.throwIfAborted()
      const currentTime = now()
      if (currentTime >= record.expiresAt) {
        throw new SessionCookieLockUnavailableError('Session cookie lock lease expired while waiting')
      }
      if (record.expiresAt-currentTime <= leaseMs / 2) {
        record = { ...record, expiresAt: currentTime + leaseMs }
        writeStorageBakeryRecord(storage, key, record)
      }

      const active = readActiveStorageBakeryRecords(storage, prefix, currentTime)
      const own = active.find((candidate) => candidate.key === key)
      if (!own || own.value.owner !== owner || own.value.ticket !== record.ticket) {
        throw new SessionCookieLockUnavailableError('Session cookie lock lease was lost')
      }
      const blocked = active.some(({ value }) =>
        value.owner !== owner &&
        (value.choosing || storageBakeryRecordPrecedes(value, record.ticket, owner)),
      )
      if (!blocked) break
      await abortableLockDelay(pollIntervalMs, signal)
    }

    const criticalSectionTime = now()
    if (criticalSectionTime >= record.expiresAt) {
      throw new SessionCookieLockUnavailableError('Session cookie lock lease expired before use')
    }
    record = { ...record, expiresAt: criticalSectionTime + leaseMs }
    writeStorageBakeryRecord(storage, key, record)
    signal.throwIfAborted()
    return await operation()
  } finally {
    removeOwnedStorageBakeryRecord(storage, key, owner)
  }
}

/** Run an asynchronous operation behind one deadline, including lock wait. */
export async function withAbortDeadline<T>(
  operation: (signal: AbortSignal) => Promise<T>,
  timeoutMs = SESSION_REQUEST_TIMEOUT_MS,
  parentSignal?: AbortSignal,
): Promise<T> {
  const controller = new AbortController()
  const forwardAbort = () => controller.abort(parentSignal?.reason)
  if (parentSignal?.aborted) forwardAbort()
  else parentSignal?.addEventListener('abort', forwardAbort, { once: true })

  const timer = globalThis.setTimeout(() => controller.abort(timeoutError()), timeoutMs)
  let removeAbortRaceListener: () => void = () => undefined
  const abortRace = new Promise<never>((_, reject) => {
    const rejectAbort = () => reject(controller.signal.reason ?? new DOMException('Aborted', 'AbortError'))
    if (controller.signal.aborted) {
      rejectAbort()
      return
    }
    controller.signal.addEventListener('abort', rejectAbort, { once: true })
    removeAbortRaceListener = () => controller.signal.removeEventListener('abort', rejectAbort)
  })

  try {
    const pending = Promise.resolve().then(() => operation(controller.signal))
    return await Promise.race([pending, abortRace])
  } finally {
    globalThis.clearTimeout(timer)
    removeAbortRaceListener()
    parentSignal?.removeEventListener('abort', forwardAbort)
  }
}

/** Acquire the browser session-cookie lock with an abortable wait. */
export async function withAbortableSessionLock<T>(
  locks: LockManager | undefined,
  name: string,
  signal: AbortSignal,
  operation: () => Promise<T>,
): Promise<T> {
  const run = () => {
    signal.throwIfAborted()
    return operation()
  }
  if (!locks) return run()
  return await locks.request(name, { mode: 'exclusive', signal }, run)
}
