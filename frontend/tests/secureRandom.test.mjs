import test from 'node:test'
import assert from 'node:assert/strict'
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'
import { transformSync } from 'esbuild'

const tsSource = readFileSync(new URL('../src/utils/secureRandom.ts', import.meta.url), 'utf8')
const { code } = transformSync(tsSource, { loader: 'ts', format: 'esm', target: 'es2022' })
const tempDir = mkdtempSync(join(tmpdir(), 'finarch-secure-random-test-'))
const compiledPath = join(tempDir, 'secureRandom.mjs')
writeFileSync(compiledPath, code)
const { secureRandomHex, secureRandomInt } = await import(pathToFileURL(compiledPath).href)

process.on('exit', () => {
  rmSync(tempDir, { recursive: true, force: true })
})

test('secureRandomHex returns expected hex length', () => {
  const token = secureRandomHex(16)
  assert.equal(token.length, 32)
  assert.match(token, /^[0-9a-f]+$/)
})

test('secureRandomInt returns values in range', () => {
  for (let i = 0; i < 500; i += 1) {
    const value = secureRandomInt(7)
    assert.equal(Number.isInteger(value), true)
    assert.equal(value >= 0 && value < 7, true)
  }
})

test('secureRandomInt rejects invalid maxExclusive', () => {
  assert.throws(() => secureRandomInt(0), /positive integer/)
  assert.throws(() => secureRandomInt(-1), /positive integer/)
  assert.throws(() => secureRandomInt(1.2), /positive integer/)
})
