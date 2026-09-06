import { useMemo } from 'react'
import { Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { useChartPalette } from '../hooks/useChartPalette'

import type { TrendPoint, ExchangeRange } from '../utils/exchangeChart'
import { buildChartPoints, xAxisInterval } from '../utils/exchangeChart'

function formatRate(value: number, from: string, to: string) {
  const lowPrecisionPairs = new Set(['JPY', 'KRW'])
  const digits = lowPrecisionPairs.has(from) || lowPrecisionPairs.has(to) ? 2 : 4
  const num = Number(value)
  if (num > 0 && num < 0.01) return num.toPrecision(2)
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
  range: ExchangeRange
}) {
  const isMobile = typeof window !== 'undefined' ? window.matchMedia('(max-width: 768px)').matches : false
  const palette = useChartPalette()
  const tooltipPosition = useMemo(() => ({ x: isMobile ? 10 : 20, y: 16 }), [isMobile])
  const chartData = useMemo(() => buildChartPoints(data, range, locale), [data, locale, range])
  const tickInterval = useMemo(() => xAxisInterval(range, isMobile), [isMobile, range])

  const trendDelta = chartData.length > 1 ? chartData[chartData.length - 1]!.rate - chartData[0]!.rate : 0
  const trendColors = trendDelta > 0
    ? { stroke: palette.expense, gradientStart: palette.fillNegative }
    : trendDelta < 0
      ? { stroke: palette.income, gradientStart: palette.fillPositive }
      : { stroke: palette.secondary, gradientStart: palette.fillNeutral }

  const yFormatter = (value: number) => {
    const num = Number(value)
    if (num > 0 && num < 0.01) return num.toPrecision(isMobile ? 1 : 2)
    return num.toFixed(isMobile ? 2 : 4)
  }

  return (
    <div className="w-full max-w-full touch-pan-y overflow-x-auto overflow-y-visible md:overflow-visible" style={{ WebkitTapHighlightColor: 'transparent' }}>
      <div className="h-[220px] w-full px-1 sm:h-[240px] md:h-[320px] md:min-w-0 md:px-0">
        <ResponsiveContainer width="100%" height="100%">
          <AreaChart data={chartData} margin={{ top: 16, right: isMobile ? 8 : 16, left: isMobile ? 8 : 24, bottom: 16 }}>
            <defs>
              <linearGradient id="rateGradient" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor={trendColors.gradientStart} />
                <stop offset="100%" stopColor={palette.surface} stopOpacity={0} />
              </linearGradient>
            </defs>
            <CartesianGrid stroke={palette.grid} strokeDasharray="3 3" vertical={false} />
            <XAxis
              dataKey="tickLabel"
              tick={{ fontSize: isMobile ? 10 : 11, fill: palette.mutedForeground }}
              tickMargin={8}
              minTickGap={isMobile ? 40 : 24}
              interval={tickInterval}
              tickLine={false}
              axisLine={false}
              padding={{ left: 14, right: 14 }}
            />
            <YAxis
              tick={{ fontSize: isMobile ? 10 : 11, fill: palette.mutedForeground }}
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
              cursor={{ stroke: palette.secondary, strokeWidth: 1.2, strokeDasharray: '4 4' }}
              contentStyle={{
                background: palette.surface,
                borderRadius: 8,
                border: `1px solid ${palette.border}`,
                color: palette.foreground,
                boxShadow: 'var(--shadow-sm)',
              }}
              wrapperStyle={{ zIndex: 20 }}
              labelFormatter={(_, payload) => String(payload?.[0]?.payload?.tooltipLabel ?? '')}
              formatter={(value: number | string | undefined) => [formatRate(Number(value ?? 0), from, to), `${from}/${to}`]}
            />
            <Area
              type="monotone"
              dataKey="rate"
              stroke={trendColors.stroke}
              strokeWidth={2}
              fill="url(#rateGradient)"
              dot={false}
              activeDot={{ r: 5, fill: palette.surface, stroke: trendColors.stroke, strokeWidth: 2 }}
              isAnimationActive
              animationDuration={320}
            />
          </AreaChart>
        </ResponsiveContainer>
      </div>
    </div>
  )
}
