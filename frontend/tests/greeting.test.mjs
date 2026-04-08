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

test('formats zh-CN greeting when username is present', () => {
  assert.equal(
    formatGreeting({ locale: 'zh-CN', greeting: '下午好', message: '项目进展如何', username: '测试用户' }),
    '下午好，项目进展如何，测试用户',
  )
})

test('formats zh-CN greeting when username is undefined', () => {
  assert.equal(
    formatGreeting({ locale: 'zh-CN', greeting: '下午好', message: '项目进展如何' }),
    '下午好，项目进展如何',
  )
})

test('formats zh-CN greeting when username is empty string', () => {
  assert.equal(
    formatGreeting({ locale: 'zh-CN', greeting: '下午好', message: '项目进展如何', username: '   ' }),
    '下午好，项目进展如何',
  )
})

test('formats en greeting when username is present', () => {
  assert.equal(
    formatGreeting({ locale: 'en', greeting: 'Good afternoon', message: 'how is your project going', username: 'Test User' }),
    'Good afternoon, how is your project going, Test User.',
  )
})

test('formats en greeting when username is undefined', () => {
  assert.equal(
    formatGreeting({ locale: 'en', greeting: 'Good afternoon', message: 'how is your project going' }),
    'Good afternoon, how is your project going.',
  )
})

test('formats en greeting when username is empty string', () => {
  assert.equal(
    formatGreeting({ locale: 'en', greeting: 'Good afternoon', message: 'how is your project going', username: '   ' }),
    'Good afternoon, how is your project going.',
  )
})

test('strips malformed punctuation before formatting', () => {
  assert.equal(
    formatGreeting({ locale: 'zh-CN', greeting: '下午好，', message: '项目进展如何？，', username: '测试用户.' }),
    '下午好，项目进展如何，测试用户',
  )

  assert.equal(
    formatGreeting({ locale: 'en', greeting: 'Good afternoon?!', message: 'how is your project going，', username: 'Test User...' }),
    'Good afternoon, how is your project going, Test User.',
  )
})

test('safeguard: source greeting logic does not contain hardcoded real usernames', () => {
  const dashboardSource = readFileSync(new URL('../src/pages/DashboardPage.tsx', import.meta.url), 'utf8')
  const greetingUtilSource = readFileSync(new URL('../src/utils/greeting.ts', import.meta.url), 'utf8')
  const forbiddenNameRegex = /(昊伟|Haowei)/

  assert.equal(forbiddenNameRegex.test(dashboardSource), false)
  assert.equal(forbiddenNameRegex.test(greetingUtilSource), false)
})
