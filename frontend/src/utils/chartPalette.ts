import type { AppMode } from '../contexts/modeContextCore'

export interface ChartPalette {
  income: string
  expense: string
  net: string
  pending: string
  budget: string
  recurring: string
  secondary: string
  categories: string[]
}

const WORK_CATEGORIES = [
  '#2d6687', '#b95642', '#487a68', '#a87839', '#668596',
  '#805e52', '#74947e', '#a96a58', '#456575', '#92804d',
  '#5f756b', '#8c655e', '#6a7d91',
]

const LIFE_CATEGORIES = [
  '#28745b', '#a87839', '#537d68', '#b95642', '#668596',
  '#7e7250', '#3f6b5a', '#a96a58', '#6f8d79', '#456575',
  '#92804d', '#805e52', '#6a7d91',
]

/**
 * Returns mode-aware chart colors.
 * WORK: blue/red corporate tones; LIFE: emerald/amber warm tones.
 */
export function getModeChartPalette(mode: AppMode): ChartPalette {
  if (mode === 'life') {
    return {
      income: '#28745b',
      expense: '#b95642',
      net: '#28745b',
      pending: '#a87839',
      budget: '#28745b',
      recurring: '#537d68',
      secondary: '#668596',
      categories: LIFE_CATEGORIES,
    }
  }
  return {
    income: '#28745b',
    expense: '#b95642',
    net: '#2d6687',
    pending: '#a87839',
    budget: '#2d6687',
    recurring: '#487a68',
    secondary: '#668596',
    categories: WORK_CATEGORIES,
  }
}
