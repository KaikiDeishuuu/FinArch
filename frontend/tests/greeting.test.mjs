import test from 'node:test'
import assert from 'node:assert/strict'
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'
import { transformSync } from 'esbuild'

const tsSource = readFileSync(new URL('../src/utils/greeting.ts', import.meta.url), 'utf8')
const { code } = transformSync(tsSource, { loader: 'ts', format: 'esm', target: 'es2022' })
const tempDir = mkdtempSync(join(tmpdir(), 'finarch-greeting-test-'))
const compiledPath = join(tempDir, 'greeting.mjs')
writeFileSync(compiledPath, code)
const { formatGreeting } = await import(pathToFileURL(compiledPath).href)

process.on('exit', () => {
  rmSync(tempDir, { recursive: true, force: true })
})

test('zh-CN with username', () => {
  assert.equal(formatGreeting({ locale: 'zh-CN', greeting: '下午好', message: '项目进展如何', username: '昊伟' }), '下午好，项目进展如何，昊伟')
})

test('zh-CN without username', () => {
  assert.equal(formatGreeting({ locale: 'zh-CN', greeting: '下午好', message: '项目进展如何' }), '下午好，项目进展如何')
})

test('en with username', () => {
  assert.equal(formatGreeting({ locale: 'en', greeting: 'Good afternoon', message: 'how is your project going', username: 'Haowei' }), 'Good afternoon, how is your project going, Haowei.')
})

test('en without username', () => {
  assert.equal(formatGreeting({ locale: 'en', greeting: 'Good afternoon', message: 'how is your project going' }), 'Good afternoon, how is your project going.')
})

test('malformed input punctuation is stripped', () => {
  assert.equal(formatGreeting({ locale: 'zh-CN', greeting: '下午好，', message: '项目进展如何？，', username: '昊伟.' }), '下午好，项目进展如何，昊伟')
  assert.equal(formatGreeting({ locale: 'en', greeting: 'Good afternoon?!', message: 'how is your project going，', username: 'Haowei...' }), 'Good afternoon, how is your project going, Haowei.')
})
