import test from 'node:test'
import assert from 'node:assert/strict'
import { buildSync } from 'esbuild'
import { mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

const tempDir = mkdtempSync(join(tmpdir(), 'finarch-financial-correctness-'))

async function importTypeScript(relativePath, outputName) {
  const entry = fileURLToPath(new URL(relativePath, import.meta.url))
  const outfile = join(tempDir, `${outputName}.mjs`)
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

const {
  accountBalanceToCNY,
  aggregateAccountBalanceHistories,
  transactionAmountToCNY,
} = await importTypeScript('../src/utils/financeAmounts.ts', 'financeAmounts')
const {
  fallbackRatesForBase,
  rateBetweenCurrencies,
} = await importTypeScript('../src/utils/exchangeRates.ts', 'exchangeRates')
const {
  attachmentOCRText,
  hasOCRSuggestion,
} = await importTypeScript('../src/utils/ocr.ts', 'ocr')
const {
  isCurrentMatchResponse,
} = await importTypeScript('../src/workers/matchProtocol.ts', 'matchProtocol')

process.on('exit', () => {
  rmSync(tempDir, { recursive: true, force: true })
})

const rates = { CNY: 1, USD: 7, EUR: 8 }

test('transaction CNY conversion prefers the backend-booked base amount', () => {
  const value = transactionAmountToCNY({
    amount_yuan: 100,
    currency: 'EUR',
    base_amount_cents: 1_000,
    base_currency: 'USD',
  }, rates)
  assert.equal(value, 70)
})

test('legacy transaction conversion falls back to original currency', () => {
  assert.equal(transactionAmountToCNY({ amount_yuan: 10, currency: 'EUR' }, rates), 80)
})

test('account balances are converted from each account currency', () => {
  assert.equal(accountBalanceToCNY({ balance_yuan: 10, currency: 'USD' }, rates), 70)
})

test('mixed-currency account histories aggregate in CNY and carry balances forward', () => {
  const points = aggregateAccountBalanceHistories([
    {
      account: { id: 'usd', currency: 'USD' },
      points: [
        { date: '2026-01-01', balance: 10 },
        { date: '2026-01-02', balance: 15 },
      ],
    },
    {
      account: { id: 'eur', currency: 'EUR' },
      points: [{ date: '2026-01-01', balance: 20 }],
    },
  ], rates)

  assert.deepEqual(points, [
    { date: '2026-01-01', balance: 230 },
    { date: '2026-01-02', balance: 265 },
  ])
})

test('fallback quotes use a cross rate for non-USD base currencies', () => {
  const ratesToCNY = { CNY: 1, USD: 7.2, EUR: 8 }
  assert.equal(rateBetweenCurrencies('EUR', 'USD', ratesToCNY), 8 / 7.2)

  const quotes = fallbackRatesForBase('EUR', ['EUR', 'USD', 'JPY'])
  assert.equal(quotes.EUR, 1)
  assert.notEqual(quotes.USD, 1)
  assert.notEqual(quotes.JPY, 1)
})

test('empty OCR objects do not count as structured suggestions', () => {
  assert.equal(hasOCRSuggestion({}), false)
  assert.equal(hasOCRSuggestion({ confidence: 0.99 }), false)
  assert.equal(hasOCRSuggestion({ merchant: 'Example Store' }), true)
})

test('OCR text can be read from either DTO field', () => {
  assert.equal(attachmentOCRText({ ocr_text: '  receipt text  ' }), 'receipt text')
  assert.equal(attachmentOCRText({ ocr_result: { text: 'markdown', suggestion: {} } }), 'markdown')
})

test('match responses are accepted only for the active request', () => {
  assert.equal(isCurrentMatchResponse({ requestId: 4, ok: true }, 4), true)
  assert.equal(isCurrentMatchResponse({ requestId: 3, ok: true }, 4), false)
})
