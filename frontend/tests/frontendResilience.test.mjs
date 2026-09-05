import test from 'node:test'
import assert from 'node:assert/strict'
import { buildSync } from 'esbuild'
import { mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

const tempDir = mkdtempSync(join(tmpdir(), 'finarch-frontend-resilience-'))

async function importTypeScript(relativePath, outputName) {
  const entry = fileURLToPath(new URL(relativePath, import.meta.url))
  const outfile = join(tempDir, outputName + '.mjs')
  buildSync({
    entryPoints: [entry],
    outfile,
    bundle: true,
    platform: 'node',
    format: 'esm',
    target: 'es2022',
  })
  return import(pathToFileURL(outfile).href)
}

const { shouldRotateIdempotencyKey } = await importTypeScript(
  '../src/utils/idempotency.ts',
  'idempotency',
)
const {
  parseAuthSessionPayload,
  SESSION_STORAGE_LOCK_PREFIX,
  withAbortableSessionLock,
  withAbortDeadline,
  withStorageSessionLock,
} = await importTypeScript('../src/utils/sessionLifecycle.ts', 'sessionLifecycle')
const { runBoundedMatch } = await importTypeScript(
  '../src/workers/match.worker.ts',
  'matchWorker',
)

process.on('exit', () => {
  rmSync(tempDir, { recursive: true, force: true })
})

function responseFailure(status) {
  return { response: { status } }
}

test('rotates transaction idempotency keys only after definitive client rejections', () => {
  for (const status of [400, 401, 403, 404, 409, 413, 422]) {
    assert.equal(shouldRotateIdempotencyKey(responseFailure(status)), true, String(status))
  }
  for (const failure of [
    new Error('network unavailable'),
    {},
    responseFailure(408),
    responseFailure(425),
    responseFailure(429),
    responseFailure(500),
    responseFailure(502),
    responseFailure(504),
  ]) {
    assert.equal(shouldRotateIdempotencyKey(failure), false)
  }
})

function validSession(overrides = {}) {
  return {
    token: 'access-token',
    expires_at: '2030-01-02T03:04:05.123+01:00',
    user_id: 'user-1',
    email: 'user@example.com',
    username: 'user',
    nickname: 'User',
    role: 'user',
    ...overrides,
  }
}

test('accepts only non-empty auth DTOs with a future RFC3339 expiry', () => {
  const now = Date.parse('2029-01-01T00:00:00Z')
  assert.deepEqual(parseAuthSessionPayload(validSession(), now), validSession())

  for (const payload of [
    validSession({ token: '   ' }),
    validSession({ nickname: '' }),
    validSession({ expires_at: '2030-01-02' }),
    validSession({ expires_at: '2030-02-30T03:04:05Z' }),
    validSession({ expires_at: '2028-01-01T00:00:00Z' }),
  ]) {
    assert.throws(() => parseAuthSessionPayload(payload, now), /Invalid authentication response/)
  }
})

test('aborts a stalled session HTTP operation at its deadline', async () => {
  let observedAbort = false
  await assert.rejects(
    withAbortDeadline(
      (signal) => new Promise((_, reject) => {
        signal.addEventListener('abort', () => {
          observedAbort = true
          reject(signal.reason)
        }, { once: true })
      }),
      10,
    ),
    (error) => error?.name === 'TimeoutError',
  )
  assert.equal(observedAbort, true)
})

test('bounds a stalled Web Lock wait with the same abort deadline', async () => {
  let lockSignal
  const stalledLocks = {
    request(_name, options) {
      lockSignal = options.signal
      return new Promise(() => undefined)
    },
  }

  await assert.rejects(
    withAbortDeadline(
      (signal) => withAbortableSessionLock(stalledLocks, 'test-session-lock', signal, async () => 1),
      10,
    ),
    (error) => error?.name === 'TimeoutError',
  )
  assert.equal(lockSignal.aborted, true)
})

class MemoryLockStorage {
  #values = new Map()

  get length() {
    return this.#values.size
  }

  key(index) {
    return [...this.#values.keys()][index] ?? null
  }

  getItem(key) {
    return this.#values.get(key) ?? null
  }

  setItem(key, value) {
    this.#values.set(key, String(value))
  }

  removeItem(key) {
    this.#values.delete(key)
  }

  values() {
    return [...this.#values.values()]
  }
}

class ShiftingMemoryLockStorage extends MemoryLockStorage {
  staleKey = null
  shifted = false

  key(index) {
    const key = super.key(index)
    if (!this.shifted && index === 0 && key === this.staleKey) {
      this.shifted = true
      this.removeItem(key)
    }
    return key
  }
}

function deferred() {
  let resolve
  const promise = new Promise((done) => {
    resolve = done
  })
  return { promise, resolve }
}

test('localStorage bakery fallback serialises concurrent cookie operations', async () => {
  const storage = new MemoryLockStorage()
  const firstStarted = deferred()
  const releaseFirst = deferred()
  const order = []
  let active = 0
  let maxActive = 0

  const operation = (name, gate) => async () => {
    active += 1
    maxActive = Math.max(maxActive, active)
    order.push(name + ':start')
    if (name === 'first') firstStarted.resolve()
    if (gate) await gate.promise
    order.push(name + ':end')
    active -= 1
    return name
  }

  const first = withAbortDeadline(
    (signal) => withStorageSessionLock(
      storage,
      'cookie-lock',
      signal,
      operation('first', releaseFirst),
      { ownerId: 'tab-a', leaseMs: 1_000, pollIntervalMs: 1 },
    ),
    500,
  )
  await firstStarted.promise
  const second = withAbortDeadline(
    (signal) => withStorageSessionLock(
      storage,
      'cookie-lock',
      signal,
      operation('second'),
      { ownerId: 'tab-b', leaseMs: 1_000, pollIntervalMs: 1 },
    ),
    500,
  )

  await new Promise((resolve) => setTimeout(resolve, 10))
  assert.deepEqual(order, ['first:start'])
  assert.equal(maxActive, 1)

  releaseFirst.resolve()
  assert.deepEqual(await Promise.all([first, second]), ['first', 'second'])
  assert.deepEqual(order, ['first:start', 'first:end', 'second:start', 'second:end'])
  assert.equal(maxActive, 1)
  assert.equal(storage.length, 0)
})

test('localStorage bakery fallback aborts a bounded lock wait', async () => {
  const storage = new MemoryLockStorage()
  const holderStarted = deferred()
  const releaseHolder = deferred()
  const holderController = new AbortController()
  const holder = withStorageSessionLock(
    storage,
    'timeout-lock',
    holderController.signal,
    async () => {
      holderStarted.resolve()
      await releaseHolder.promise
    },
    { ownerId: 'holder', leaseMs: 1_000, pollIntervalMs: 1 },
  )
  await holderStarted.promise

  let contenderRan = false
  await assert.rejects(
    withAbortDeadline(
      (signal) => withStorageSessionLock(
        storage,
        'timeout-lock',
        signal,
        async () => {
          contenderRan = true
        },
        { ownerId: 'contender', leaseMs: 1_000, pollIntervalMs: 1 },
      ),
      15,
    ),
    (error) => error?.name === 'TimeoutError',
  )
  assert.equal(contenderRan, false)

  releaseHolder.resolve()
  await holder
  await new Promise((resolve) => setTimeout(resolve, 0))
  assert.equal(storage.length, 0)
})

test('localStorage bakery snapshot cannot skip a holder when an earlier key is removed', async () => {
  const storage = new ShiftingMemoryLockStorage()
  const name = 'shifting-lock'
  const prefix = SESSION_STORAGE_LOCK_PREFIX + encodeURIComponent(name) + ':'
  const staleKey = prefix + encodeURIComponent('000-stale')
  const holderKey = prefix + encodeURIComponent('zzz-holder')
  storage.staleKey = staleKey
  storage.setItem(staleKey, JSON.stringify({
    owner: '000-stale', ticket: 0, choosing: false, expiresAt: Date.now() - 1,
  }))
  storage.setItem(holderKey, JSON.stringify({
    owner: 'zzz-holder', ticket: 1, choosing: false, expiresAt: Date.now() + 1_000,
  }))

  let contenderRan = false
  await assert.rejects(
    withAbortDeadline(
      (signal) => withStorageSessionLock(
        storage,
        name,
        signal,
        async () => {
          contenderRan = true
        },
        { ownerId: 'aaa-contender', leaseMs: 1_000, pollIntervalMs: 1 },
      ),
      15,
    ),
    (error) => error?.name === 'TimeoutError',
  )
  assert.equal(storage.shifted, true)
  assert.equal(contenderRan, false)
  assert.notEqual(storage.getItem(holderKey), null)
})

test('localStorage bakery fallback reclaims an expired crash lease without storing credentials', async () => {
  const storage = new MemoryLockStorage()
  const name = 'crash-lock'
  const staleKey = SESSION_STORAGE_LOCK_PREFIX + encodeURIComponent(name) + ':' +
    encodeURIComponent('crashed-tab')
  storage.setItem(staleKey, JSON.stringify({
    owner: 'crashed-tab',
    ticket: 1,
    choosing: true,
    expiresAt: Date.now() - 1,
  }))

  const controller = new AbortController()
  const secretMarker = 'must-never-enter-local-storage'
  const result = await withStorageSessionLock(
    storage,
    name,
    controller.signal,
    async () => {
      for (const raw of storage.values()) {
        assert.equal(raw.includes(secretMarker), false)
        assert.deepEqual(
          Object.keys(JSON.parse(raw)).sort(),
          ['choosing', 'expiresAt', 'owner', 'ticket'],
        )
      }
      return secretMarker
    },
    { ownerId: 'replacement-tab', leaseMs: 1_000, pollIntervalMs: 1 },
  )

  assert.equal(result, secretMarker)
  assert.equal(storage.getItem(staleKey), null)
  assert.equal(storage.length, 0)
})

function item(id, amountCents = 100) {
  return {
    id,
    amountCents,
    occurredTs: Math.floor(Date.now() / 1000),
  }
}

const stableClock = () => 0

test('subset matching reports candidate and result truncation', () => {
  const candidateBound = runBoundedMatch(
    [item('a'), item('b'), item('c')],
    100,
    0,
    1,
    10,
    { maxCandidates: 2, maxScannedCandidates: 10, clock: stableClock },
  )
  assert.equal(candidateBound.truncated, true)
  assert.equal(candidateBound.results.length, 2)

  const resultBound = runBoundedMatch(
    [item('a'), item('b'), item('c')],
    100,
    0,
    1,
    10,
    { maxResults: 1, clock: stableClock },
  )
  assert.equal(resultBound.truncated, true)
  assert.equal(resultBound.results.length, 1)
})

test('subset matching stops at node and wall-clock budgets', () => {
  const nodeBound = runBoundedMatch(
    [item('a'), item('b'), item('c')],
    300,
    0,
    3,
    10,
    { maxNodes: 1, clock: stableClock },
  )
  assert.equal(nodeBound.truncated, true)

  let tick = 0
  const timeBound = runBoundedMatch(
    [item('a'), item('b'), item('c')],
    300,
    0,
    3,
    10,
    { maxDurationMs: 1, clock: () => tick++ * 2 },
  )
  assert.equal(timeBound.truncated, true)
})
