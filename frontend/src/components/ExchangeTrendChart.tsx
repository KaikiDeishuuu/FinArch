import { useMemo } from 'react'
import { Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { useTheme } from '../contexts/ThemeContext'

export interface TrendPoint {
  date: string
  rate: number
}

function formatDate(input: string, locale: string, includeYear = false) {
  const d = new Date(input)
  if (Number.isNaN(d.getTime())) return input
  return d.toLocaleDateString(locale.startsWith('zh') ? 'zh-TW' : 'en-US', includeYear
    ? { month: 'short', day: 'numeric', year: 'numeric' }
    : { month: 'short', day: 'numeric' })
}

function formatRate(value: number, from: string, to: string) {
  const lowPrecisionPairs = new Set(['JPY', 'KRW'])
  const digits = lowPrecisionPairs.has(from) || lowPrecisionPairs.has(to) ? 2 : 4
  const num = Number(value)
  if (num > 0 && num < 0.01) {
    return num.toPrecision(2)
  }
  return num.toFixed(digits)
}

export default function ExchangeTrendChart({
  data,
  from,
  to,
  locale,
  range,
}: {
  data: TrendPoint[]
  from: string
  to: string
  locale: string
  range: '1D' | '1W' | '1M' | '1Y'
}) {
  const isMobile = typeof window !== 'undefined' ? window.matchMedia('(max-width: 768px)').matches : false
  const { resolved } = useTheme()
  const isDark = resolved === 'dark'
  const palette = { primary: '#3B82F6', income: '#22C55E', expense: '#EF4444', secondary: '#6B7280' }
  const tooltipPosition = useMemo(() => ({ x: isMobile ? 10 : 20, y: 16 }), [isMobile])

  const includeYearInTicks = range === '1M' || range === '1Y'
  const xAxisInterval = useMemo(() => {
    if (range === '1Y') return isMobile ? 7 : 3
    if (range === '1M') return isMobile ? 4 : 2
    if (range === '1W') return isMobile ? 1 : 0
    return isMobile ? 3 : 1
  }, [isMobile, range])

  const trendDelta = data.length > 1 ? data[data.length - 1].rate - data[0].rate : 0
  const trendColors = trendDelta > 0
    ? { stroke: palette.expense, gradientStart: isDark ? 'rgba(248,113,113,0.35)' : 'rgba(239,68,68,0.20)' }
    : trendDelta < 0
      ? { stroke: palette.income, gradientStart: isDark ? 'rgba(74,222,128,0.3)' : 'rgba(34,197,94,0.18)' }
      : { stroke: palette.secondary, gradientStart: isDark ? 'rgba(156,163,175,0.28)' : 'rgba(107,114,128,0.14)' }

  const yFormatter = (v: number) => {
    const num = Number(v)
    if (num > 0 && num < 0.01) {
      if (isMobile) return num.toPrecision(1)
      return num.toPrecision(2)
    }
    return num.toFixed(isMobile ? 2 : 4)
  }

  return (
    <div
      className="w-full max-w-full overflow-x-auto overflow-y-visible touch-pan-y md:overflow-visible"
      style={{ WebkitTapHighlightColor: 'transparent' }}
    >
      <div className="h-[220px] w-full px-1 sm:h-[240px] md:h-[320px] md:min-w-0 md:px-0">
        <ResponsiveContainer width="100%" height="100%">
          <AreaChart data={data} margin={{ top: 16, right: isMobile ? 8 : 16, left: isMobile ? 8 : 24, bottom: 16 }}>
            <defs>
              <linearGradient id="rateGradient" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor={trendColors.gradientStart} />
                <stop offset="100%" stopColor="rgba(255,255,255,0)" />
              </linearGradient>
            </defs>
            <CartesianGrid stroke={isDark ? '#374151' : '#E5E7EB'} strokeDasharray="3 3" vertical={false} />
            <XAxis
              dataKey="date"
              tick={{ fontSize: isMobile ? 10 : 11, fill: isDark ? '#9CA3AF' : '#6B7280' }}
              tickMargin={8}
              minTickGap={isMobile ? 40 : 24}
              interval={xAxisInterval}
              tickLine={false}
              axisLine={false}
              padding={{ left: 14, right: 14 }}
              tickFormatter={(v) => formatDate(String(v), locale, includeYearInTicks)}
            />
            <YAxis
              tick={{ fontSize: isMobile ? 10 : 11, fill: isDark ? '#9CA3AF' : '#6B7280' }}
              tickMargin={6}
              tickLine={false}
              axisLine={false}
              width={isMobile ? 48 : 64}
              domain={['auto', 'auto']}
              tickFormatter={yFormatter}
            />
            <Tooltip
              allowEscapeViewBox={{ x: false, y: false }}
              position={tooltipPosition}
              cursor={{ stroke: palette.primary, strokeWidth: 1.2, strokeDasharray: '4 4' }}
              contentStyle={{
                background: isDark ? '#111827' : '#FFFFFF',
                borderRadius: 12,
                border: `1px solid ${isDark ? '#374151' : '#E5E7EB'}`,
                color: isDark ? '#F3F4F6' : '#111827',
                boxShadow: '0 8px 20px rgba(15,23,42,0.14)',
              }}
              wrapperStyle={{ zIndex: 20 }}
              labelFormatter={(label) => formatDate(String(label), locale, true)}
              formatter={(value: number | string | undefined) => [formatRate(Number(value ?? 0), from, to), `${from}/${to}`]}
            />
            <Area
              type="monotone"
              dataKey="rate"
              stroke={trendColors.stroke}
              strokeWidth={isMobile ? 2.5 : 3}
              fill="url(#rateGradient)"
              dot={false}
              activeDot={{ r: 5, fill: isDark ? '#111827' : '#FFF', stroke: trendColors.stroke, strokeWidth: 2 }}
              isAnimationActive
              animationDuration={420}
            />
          </AreaChart>
        </ResponsiveContainer>
      </div>
    </div>
  )
}
