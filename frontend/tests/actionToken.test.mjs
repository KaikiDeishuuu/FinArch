import test from 'node:test'
import assert from 'node:assert/strict'
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'
import { transformSync } from 'esbuild'

const source = readFileSync(new URL('../src/utils/actionToken.ts', import.meta.url), 'utf8')
const { code } = transformSync(source, { loader: 'ts', format: 'esm', target: 'es2022' })
const tempDir = mkdtempSync(join(tmpdir(), 'finarch-action-token-test-'))
const compiledPath = join(tempDir, 'actionToken.mjs')
writeFileSync(compiledPath, code)
const { clearActionTokenFromURL, readActionToken } = await import(pathToFileURL(compiledPath).href)

process.on('exit', () => {
  rmSync(tempDir, { recursive: true, force: true })
})

function captureHistory() {
  const calls = []
  return {
    state: { navigation: 1 },
    calls,
    replaceState(state, title, url) {
      calls.push({ state, title, url })
    },
  }
}

test('prefers fragment action tokens and removes all token copies from the URL', () => {
  const history = captureHistory()
  const location = {
    pathname: '/verify-email',
    search: '?token=legacy&lang=zh',
    hash: '#token=fragment%2Etoken&step=confirm',
  }

  assert.equal(readActionToken(location), 'fragment.token')
  clearActionTokenFromURL(location, history)
  assert.deepEqual(history.calls, [{
    state: { navigation: 1 },
    title: '',
    url: '/verify-email?lang=zh#step=confirm',
  }])
})

test('accepts a legacy query token, clears it, and preserves an unrelated anchor', () => {
  const history = captureHistory()
  const location = {
    pathname: '/reset-password',
    search: '?next=login&token=old%20token',
    hash: '#password-form',
  }

  assert.equal(readActionToken(location), 'old token')
  clearActionTokenFromURL(location, history)
  assert.equal(history.calls[0].url, '/reset-password?next=login#password-form')
})

test('does not rewrite a URL that contains no token parameter', () => {
  const history = captureHistory()
  const location = { pathname: '/verify-email', search: '', hash: '' }
  assert.equal(readActionToken(location), '')
  clearActionTokenFromURL(location, history)
  assert.equal(history.calls.length, 0)
})
