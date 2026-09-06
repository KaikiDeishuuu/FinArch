import type { AppMode } from '../contexts/modeContextCore'
import {
  CHART_COLOR_TOKENS,
  DARK_CHART_COLORS,
  LIGHT_CHART_COLORS,
} from '../constants/theme'

export interface ChartPalette {
  income: string
  expense: string
  net: string
  pending: string
  budget: string
  recurring: string
  secondary: string
  categories: string[]
  grid: string
  fillPositive: string
  fillNegative: string
  fillNeutral: string
  surface: string
  foreground: string
  mutedForeground: string
  border: string
}

function readColor(styles: CSSStyleDeclaration, token: string, fallback: string) {
  return styles.getPropertyValue(token).trim() || fallback
}

function createFallbackPalette(mode: AppMode, resolved: 'light' | 'dark'): ChartPalette {
  const isDark = resolved === 'dark'
  const categories = [...(isDark ? DARK_CHART_COLORS : LIGHT_CHART_COLORS)]
  const modeColor = mode === 'life'
    ? isDark ? '#3FC1AC' : '#0B7A6B'
    : isDark ? '#7F9CF3' : '#2F5FD6'

  return {
    income: isDark ? '#4CC38A' : '#1B7F4C',
    expense: isDark ? '#F0717A' : '#C93A3F',
    net: modeColor,
    pending: isDark ? '#E0A23C' : '#9A5B00',
    budget: categories[6]!,
    recurring: categories[2]!,
    secondary: modeColor,
    categories,
    grid: isDark ? 'rgb(42 47 57 / 0.82)' : 'rgb(226 230 236 / 0.78)',
    fillPositive: isDark ? 'rgb(76 195 138 / 0.28)' : 'rgb(27 127 76 / 0.18)',
    fillNegative: isDark ? 'rgb(240 113 122 / 0.30)' : 'rgb(201 58 63 / 0.18)',
    fillNeutral: isDark ? 'rgb(163 170 182 / 0.20)' : 'rgb(93 101 114 / 0.14)',
    surface: isDark ? '#181B21' : '#FFFFFF',
    foreground: isDark ? '#E8EAEE' : '#161A22',
    mutedForeground: isDark ? '#A3AAB6' : '#5D6572',
    border: isDark ? '#2A2F39' : '#E2E6EC',
  }
}

/**
 * Resolves chart tokens to concrete colors for Recharts SVG attributes.
 * Categorical colors have one fixed order; consumers must aggregate series beyond slot eight.
 */
export function getModeChartPalette(mode: AppMode, resolved: 'light' | 'dark' = 'light'): ChartPalette {
  const fallback = createFallbackPalette(mode, resolved)
  if (typeof window === 'undefined') return fallback

  const root = document.documentElement
  const rootMatchesResolvedTheme = root.classList.contains('dark') === (resolved === 'dark')

  // ThemeContext applies the root class in an effect. During that one render, use the
  // resolved-theme constants rather than reading stale custom properties from the DOM.
  if (!rootMatchesResolvedTheme) return fallback

  const styles = window.getComputedStyle(root)
  const categories = CHART_COLOR_TOKENS.map((token, index) => (
    readColor(styles, token, fallback.categories[index]!)
  ))
  const modeColor = readColor(
    styles,
    mode === 'life' ? '--mode-life' : '--mode-work',
    fallback.secondary,
  )

  return {
    income: readColor(styles, '--positive', fallback.income),
    expense: readColor(styles, '--negative', fallback.expense),
    net: modeColor,
    pending: readColor(styles, '--warning', fallback.pending),
    budget: categories[6]!,
    recurring: categories[2]!,
    secondary: modeColor,
    categories,
    grid: readColor(styles, '--chart-grid', fallback.grid),
    fillPositive: readColor(styles, '--chart-fill-positive', fallback.fillPositive),
    fillNegative: readColor(styles, '--chart-fill-negative', fallback.fillNegative),
    fillNeutral: readColor(styles, '--chart-fill-neutral', fallback.fillNeutral),
    surface: readColor(styles, '--card', fallback.surface),
    foreground: readColor(styles, '--foreground', fallback.foreground),
    mutedForeground: readColor(styles, '--muted-foreground', fallback.mutedForeground),
    border: readColor(styles, '--border', fallback.border),
  }
}
