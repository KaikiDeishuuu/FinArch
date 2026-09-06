import test from 'node:test'
import assert from 'node:assert/strict'
import { buildSync } from 'esbuild'
import { mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

const tempDir = mkdtempSync(join(tmpdir(), 'finarch-category-chart-'))
const outfile = join(tempDir, 'categoryChart.mjs')
buildSync({
  entryPoints: [fileURLToPath(new URL('../src/utils/categoryChart.ts', import.meta.url))],
  outfile,
  bundle: true,
  platform: 'node',
  format: 'esm',
  target: 'es2022',
})

const {
  MAX_CATEGORY_SERIES,
  OTHER_CATEGORY_ID,
  buildCategoryChartRows,
  categoryColorIndex,
} = await import(pathToFileURL(outfile).href)

process.on('exit', () => {
  rmSync(tempDir, { recursive: true, force: true })
})

test('category color follows canonical identity instead of rank or locale', () => {
  const category = '耗材'
  assert.equal(categoryColorIndex(category), categoryColorIndex(category))
  assert.ok(categoryColorIndex(category) < MAX_CATEGORY_SERIES - 1)

  const rankedFirst = buildCategoryChartRows([
    { category, total: 100, count: 1 },
    { category: '材料', total: 50, count: 1 },
  ])
  const rankedSecond = buildCategoryChartRows([
    { category: '材料', total: 500, count: 1 },
    { category, total: 100, count: 1 },
  ])
  assert.equal(
    rankedFirst.find((row) => row.category === category)?.colorIndex,
    rankedSecond.find((row) => row.category === category)?.colorIndex,
  )
})

test('visible category colors are unique and collisions fold into overflow', () => {
  const bySlot = new Map()
  let collision

  for (let index = 0; index < 500 && !collision; index += 1) {
    const category = `custom-${index}`
    const colorIndex = categoryColorIndex(category)
    const existing = bySlot.get(colorIndex)
    if (existing) collision = [existing, category]
    else bySlot.set(colorIndex, category)
  }

  assert.ok(collision)
  const rows = buildCategoryChartRows([
    { category: collision[0], total: 100, count: 1 },
    { category: collision[1], total: 90, count: 1 },
    { category: 'unique', total: 80, count: 1 },
  ])
  const ordinaryRows = rows.filter((row) => row.category !== OTHER_CATEGORY_ID)

  assert.equal(
    new Set(ordinaryRows.map((row) => row.colorIndex)).size,
    ordinaryRows.length,
  )
  assert.ok(rows.some((row) => row.category === OTHER_CATEGORY_ID))
})

test('overflow uses a reserved identity without colliding with canonical Other', () => {
  const values = Array.from({ length: 9 }, (_, index) => ({
    category: index === 0 ? '其他' : `custom-${index}`,
    total: 100 - index,
    count: 1,
  }))
  const rows = buildCategoryChartRows(values, '', 'Other')

  assert.ok(rows.length <= MAX_CATEGORY_SERIES)
  assert.equal(new Set(rows.map((row) => row.category)).size, rows.length)
  assert.ok(rows.some((row) => row.category === '其他'))
  assert.ok(rows.some((row) => row.category === OTHER_CATEGORY_ID && row.label === 'Other'))
})

test('an explicitly selected category is never folded into Other', () => {
  const values = Array.from({ length: 10 }, (_, index) => ({
    category: `category-${index}`,
    total: 100 - index,
    count: 1,
  }))
  const selected = 'category-9'
  const rows = buildCategoryChartRows(values, selected, 'Other')

  const selectedRow = rows.find((row) => row.category === selected)
  const ordinaryRows = rows.filter((row) => row.category !== OTHER_CATEGORY_ID)

  assert.ok(selectedRow)
  assert.ok(rows.some((row) => row.category === OTHER_CATEGORY_ID))
  assert.ok(rows.length <= MAX_CATEGORY_SERIES)
  assert.equal(
    new Set(ordinaryRows.map((row) => row.colorIndex)).size,
    ordinaryRows.length,
  )
})
