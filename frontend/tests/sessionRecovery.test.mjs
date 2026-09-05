import test from 'node:test'
import assert from 'node:assert/strict'
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'
import { transformSync } from 'esbuild'

const tsSource = readFileSync(new URL('../src/utils/sessionRecovery.ts', import.meta.url), 'utf8')
const { code } = transformSync(tsSource, { loader: 'ts', format: 'esm', target: 'es2022' })
const tempDir = mkdtempSync(join(tmpdir(), 'finarch-session-recovery-test-'))
const compiledPath = join(tempDir, 'sessionRecovery.mjs')
writeFileSync(compiledPath, code)
const {
  getApiFailureCode,
  getSessionRecoveryDelay,
  isDefinitiveSessionInvalid,
  isRefreshableAccessFailure,
  SESSION_RECOVERY_MAX_ATTEMPTS,
} = await import(pathToFileURL(compiledPath).href)

process.on('exit', () => {
  rmSync(tempDir, { recursive: true, force: true })
})

function failure(status, code, nested = true) {
  return {
    response: {
      status,
      data: nested ? { success: false, error: { code, message: 'test' } } : { code, message: 'test' },
    },
  }
}

test('extracts modern nested and legacy top-level API failure codes', () => {
  assert.equal(getApiFailureCode(failure(401, 'access_expired')), 'access_expired')
  assert.equal(getApiFailureCode(failure(401, 40101, false)), '40101')
  assert.equal(getApiFailureCode({ response: { status: 401, data: { error: {} } } }), null)
})

test('refreshes only explicit access-session failures', () => {
  for (const code of ['access_expired', 'access_invalid', 'session_invalid']) {
    assert.equal(isRefreshableAccessFailure(failure(401, code)), true, code)
  }
  for (const code of ['40101', 'auth_invalid_token', 'invalid_session']) {
    assert.equal(isRefreshableAccessFailure(failure(401, code, code !== '40101')), true, code)
  }

  assert.equal(isRefreshableAccessFailure(failure(401, 'not_authorized')), false)
  assert.equal(isRefreshableAccessFailure(failure(401, 'invalid_credentials')), false)
  assert.equal(isRefreshableAccessFailure(failure(403, 'session_invalid')), false)
  assert.equal(isRefreshableAccessFailure({ request: {} }), false)
})

test('treats only an explicit refresh-session rejection as logged out', () => {
  assert.equal(isDefinitiveSessionInvalid(failure(401, 'session_invalid')), true)
  assert.equal(isDefinitiveSessionInvalid(failure(401, 'invalid_session')), true)

  assert.equal(isDefinitiveSessionInvalid(failure(401, 'access_expired')), false)
  assert.equal(isDefinitiveSessionInvalid(failure(401, 'not_authorized')), false)
  assert.equal(isDefinitiveSessionInvalid(failure(429, 'session_rate_limited')), false)
  assert.equal(isDefinitiveSessionInvalid(failure(503, 'system_unavailable')), false)
  assert.equal(isDefinitiveSessionInvalid(new Error('network unavailable')), false)
})

test('uses finite attempts with capped exponential retry delays', () => {
  assert.equal(SESSION_RECOVERY_MAX_ATTEMPTS, 5)
  assert.deepEqual(
    [1, 2, 3, 4, 5, 20].map(getSessionRecoveryDelay),
    [500, 1_000, 2_000, 4_000, 8_000, 8_000],
  )
})
