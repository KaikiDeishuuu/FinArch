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
  '#2563eb', '#ef4444', '#6366f1', '#06b6d4', '#8b5cf6',
  '#f59e0b', '#10b981', '#ec4899', '#84cc16', '#14b8a6',
  '#f97316', '#a855f7', '#0ea5e9',
]

const LIFE_CATEGORIES = [
  '#10b981', '#f59e0b', '#14b8a6', '#f97316', '#84cc16',
  '#06b6d4', '#ec4899', '#8b5cf6', '#eab308', '#22c55e',
  '#0ea5e9', '#ef4444', '#a855f7',
]

/**
 * Returns mode-aware chart colors.
 * WORK: blue/red corporate tones; LIFE: emerald/amber warm tones.
 */
export function getModeChartPalette(mode: AppMode): ChartPalette {
  if (mode === 'life') {
    return {
      income: '#10b981',
      expense: '#f59e0b',
      net: '#14b8a6',
      pending: '#f97316',
      budget: '#8b5cf6',
      recurring: '#06b6d4',
      secondary: '#14b8a6',
      categories: LIFE_CATEGORIES,
    }
  }
  return {
    income: '#3b82f6',
    expense: '#ef4444',
    net: '#6366f1',
    pending: '#f59e0b',
    budget: '#8b5cf6',
    recurring: '#06b6d4',
    secondary: '#6366f1',
    categories: WORK_CATEGORIES,
  }
}
