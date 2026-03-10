export type ExchangeRange = '1D' | '1W' | '1M' | '1Y'

export interface TrendPoint {
  date: string
  rate: number
}

export interface ChartPoint extends TrendPoint {
  tickLabel: string
  tooltipLabel: string
}

const localeForIntl = (locale: string) => (locale.startsWith('zh') ? 'zh-TW' : 'en-US')

const formatDay = (date: Date, locale: string, includeYear = false) =>
  date.toLocaleDateString(localeForIntl(locale), includeYear
    ? { month: 'short', day: 'numeric', year: 'numeric' }
    : { month: 'short', day: 'numeric' })

const formatMonth = (date: Date, locale: string) =>
  date.toLocaleDateString(localeForIntl(locale), { month: 'short' })

const formatMonthTooltip = (date: Date, locale: string) =>
  date.toLocaleDateString(localeForIntl(locale), { month: 'long', year: 'numeric' })

export function buildChartPoints(data: TrendPoint[], range: ExchangeRange, locale: string): ChartPoint[] {
  if (range !== '1Y') {
    return data.map((p) => {
      const d = new Date(p.date)
      if (Number.isNaN(d.getTime())) {
        return { ...p, tickLabel: p.date, tooltipLabel: p.date }
      }
      return {
        ...p,
        tickLabel: formatDay(d, locale, range === '1M'),
        tooltipLabel: formatDay(d, locale, true),
      }
    })
  }

  const monthly = new Map<string, { sum: number; count: number; lastDate: string }>()
  for (const p of data) {
    const d = new Date(p.date)
    if (Number.isNaN(d.getTime())) continue
    const key = `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, '0')}`
    const current = monthly.get(key) ?? { sum: 0, count: 0, lastDate: p.date }
    current.sum += p.rate
    current.count += 1
    current.lastDate = p.date
    monthly.set(key, current)
  }

  return Array.from(monthly.entries()).sort(([a], [b]) => a.localeCompare(b)).map(([_, v]) => {
    const d = new Date(v.lastDate)
    return {
      date: v.lastDate,
      rate: v.count > 0 ? v.sum / v.count : 0,
      tickLabel: formatMonth(d, locale),
      tooltipLabel: formatMonthTooltip(d, locale),
    }
  })
}

export function xAxisInterval(range: ExchangeRange, isMobile: boolean) {
  if (range === '1Y') return 0
  if (range === '1M') return isMobile ? 4 : 2
  if (range === '1W') return isMobile ? 1 : 0
  return isMobile ? 3 : 1
}
