export const OTHER_CATEGORY_ID = '__finarch_category_overflow__'
export const MAX_CATEGORY_SERIES = 8

export interface CategoryChartValue {
  total: number
  count: number
}

export interface CategoryChartRow extends CategoryChartValue {
  category: string
  colorIndex: number
  label?: string
}

interface CategoryTotalsInput extends CategoryChartValue {
  category: string
}

function hashCategory(value: string) {
  let hash = 2166136261
  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index)
    hash = Math.imul(hash, 16777619)
  }
  return hash >>> 0
}

export function categoryColorIndex(category: string) {
  return hashCategory(category) % (MAX_CATEGORY_SERIES - 1)
}

export function buildCategoryChartRows(
  values: CategoryTotalsInput[],
  selectedCategory = '',
  otherLabel = 'Other',
): CategoryChartRow[] {
  const sorted = values
    .filter((value) => value.count > 0)
    .slice()
    .sort((a, b) => b.total - a.total || a.category.localeCompare(b.category))

  const selected = selectedCategory
    ? sorted.find((value) => value.category === selectedCategory)
    : undefined
  const candidates = selected
    ? [selected, ...sorted.filter((value) => value.category !== selectedCategory)]
    : sorted
  const visible: CategoryChartRow[] = []
  const overflow: CategoryTotalsInput[] = []
  const usedColorIndexes = new Set<number>()
  const ordinaryCapacity = MAX_CATEGORY_SERIES - 1

  for (const value of candidates) {
    const colorIndex = categoryColorIndex(value.category)
    if (
      visible.length < ordinaryCapacity &&
      !usedColorIndexes.has(colorIndex)
    ) {
      visible.push({ ...value, colorIndex })
      usedColorIndexes.add(colorIndex)
    } else {
      overflow.push(value)
    }
  }

  const rows: CategoryChartRow[] = overflow.length > 0
    ? [
        ...visible,
        {
          category: OTHER_CATEGORY_ID,
          label: otherLabel,
          total: overflow.reduce((sum, value) => sum + value.total, 0),
          count: overflow.reduce((sum, value) => sum + value.count, 0),
          colorIndex: MAX_CATEGORY_SERIES - 1,
        },
      ]
    : visible

  return rows.sort(
    (a, b) => b.total - a.total || a.category.localeCompare(b.category),
  )
}
