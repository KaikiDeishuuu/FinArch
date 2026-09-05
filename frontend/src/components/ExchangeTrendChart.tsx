import { useMemo } from 'react'
import { Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'

import type { TrendPoint, ExchangeRange } from '../utils/exchangeChart'
import { buildChartPoints, xAxisInterval } from '../utils/exchangeChart'

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
  range: ExchangeRange
}) {
  const isMobile = typeof window !== 'undefined' ? window.matchMedia('(max-width: 768px)').matches : false
  const palette = {
    primary: 'hsl(var(--mode-accent))',
    muted: 'hsl(var(--muted-foreground))',
    grid: 'hsl(var(--border))',
  }
  const tooltipPosition = useMemo(() => ({ x: isMobile ? 10 : 20, y: 16 }), [isMobile])

  const chartData = useMemo(() => buildChartPoints(data, range, locale), [data, locale, range])
  const tickInterval = useMemo(() => xAxisInterval(range, isMobile), [isMobile, range])

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
      role="img"
      aria-label={`${from}/${to}`}
    >
      <div className="h-[220px] w-full px-1 sm:h-[240px] md:h-[320px] md:min-w-0 md:px-0">
        <ResponsiveContainer width="100%" height="100%">
          <AreaChart data={chartData} margin={{ top: 16, right: isMobile ? 8 : 16, left: isMobile ? 8 : 24, bottom: 16 }}>
            <CartesianGrid stroke={palette.grid} strokeDasharray="2 5" vertical={false} />
            <XAxis
              dataKey="tickLabel"
              tick={{ fontSize: isMobile ? 10 : 11, fill: palette.muted, fontFamily: 'var(--font-data)' }}
              tickMargin={8}
              minTickGap={isMobile ? 40 : 24}
              interval={tickInterval}
              tickLine={false}
              axisLine={false}
              padding={{ left: 14, right: 14 }}

            />
            <YAxis
              tick={{ fontSize: isMobile ? 10 : 11, fill: palette.muted, fontFamily: 'var(--font-data)' }}
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
                background: 'var(--tooltip-bg)',
                borderRadius: 3,
                border: '1px solid var(--tooltip-border)',
                color: 'var(--tooltip-text)',
                boxShadow: '0 8px 20px rgba(15,23,42,0.10)',
                fontFamily: 'var(--font-data)',
              }}
              wrapperStyle={{ zIndex: 20 }}
              labelFormatter={(_, payload) => String(payload?.[0]?.payload?.tooltipLabel ?? '')}
              formatter={(value: number | string | undefined) => [formatRate(Number(value ?? 0), from, to), `${from}/${to}`]}
            />
            <Area
              type="monotone"
              dataKey="rate"
              stroke={palette.primary}
              strokeWidth={isMobile ? 2 : 2.25}
              fill="hsl(var(--mode-accent) / 0.08)"
              dot={false}
              activeDot={{ r: 4, fill: 'hsl(var(--card))', stroke: palette.primary, strokeWidth: 2 }}
              isAnimationActive
              animationDuration={420}
            />
          </AreaChart>
        </ResponsiveContainer>
      </div>
    </div>
  )
}
