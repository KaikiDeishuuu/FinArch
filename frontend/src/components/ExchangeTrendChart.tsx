import { useMemo } from 'react'
import { Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'

export interface TrendPoint {
  date: string
  rate: number
}

function formatDate(input: string, locale: string) {
  const d = new Date(input)
  if (Number.isNaN(d.getTime())) return input
  return d.toLocaleDateString(locale.startsWith('zh') ? 'zh-TW' : 'en-US', { month: 'short', day: 'numeric' })
}

function formatRate(value: number, from: string, to: string) {
  const lowPrecisionPairs = new Set(['JPY', 'KRW'])
  const digits = lowPrecisionPairs.has(from) || lowPrecisionPairs.has(to) ? 2 : 4
  return Number(value).toFixed(digits)
}

export default function ExchangeTrendChart({
  data,
  from,
  to,
  locale,
}: {
  data: TrendPoint[]
  from: string
  to: string
  locale: string
}) {
  const isMobile = typeof window !== 'undefined' ? window.matchMedia('(max-width: 768px)').matches : false
  const tickInterval = useMemo(() => {
    if (!isMobile || data.length <= 3) return 0
    return Math.max(1, Math.floor((data.length - 1) / 2))
  }, [data.length, isMobile])

  const tooltipPosition = useMemo(() => ({ x: isMobile ? 10 : 20, y: 16 }), [isMobile])

  const trendDelta = data.length > 1 ? data[data.length - 1].rate - data[0].rate : 0
  const trendColors = trendDelta > 0
    ? { stroke: '#EF4444', gradientStart: 'rgba(239,68,68,0.12)' }
    : trendDelta < 0
      ? { stroke: '#22C55E', gradientStart: 'rgba(34,197,94,0.12)' }
      : { stroke: '#6B7280', gradientStart: 'rgba(107,114,128,0.12)' }

  return (
    <div className="h-[320px] w-full max-w-full overflow-hidden touch-none">
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={data} margin={{ top: 16, right: isMobile ? 12 : 16, left: isMobile ? 48 : 48, bottom: 20 }}>
          <defs>
            <linearGradient id="rateGradient" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={trendColors.gradientStart} />
              <stop offset="100%" stopColor="rgba(255,255,255,0)" />
            </linearGradient>
          </defs>
          <CartesianGrid stroke="#E5E7EB" strokeDasharray="3 3" vertical={false} />
          <XAxis
            dataKey="date"
            tick={{ fontSize: 11, fill: '#6B7280' }}
            tickMargin={8}
            minTickGap={isMobile ? 36 : 24}
            interval={tickInterval}
            tickLine={false}
            axisLine={false}
            padding={{ left: 12, right: 12 }}
            tickFormatter={(v) => formatDate(String(v), locale)}
          />
          <YAxis
            tick={{ fontSize: 11, fill: '#6B7280' }}
            tickMargin={8}
            tickLine={false}
            axisLine={false}
            width={64}
            domain={['dataMin - 0.02', 'dataMax + 0.02']}
            tickFormatter={(v) => Number(v).toFixed(2)}
          />
          <Tooltip
            allowEscapeViewBox={{ x: false, y: false }}
            position={tooltipPosition}
            cursor={{ stroke: '#93C5FD', strokeWidth: 1 }}
            contentStyle={{ background: '#FFFFFF', borderRadius: 12, border: '1px solid #E5E7EB', boxShadow: '0 8px 20px rgba(15,23,42,0.08)' }}
            labelFormatter={(label) => formatDate(String(label), locale)}
            formatter={(value: number | string | undefined) => [formatRate(Number(value ?? 0), from, to), `${from}/${to}`]}
          />
          <Area type="monotone" dataKey="rate" stroke={trendColors.stroke} strokeWidth={2.5} fill="url(#rateGradient)" dot={{ r: 0 }} activeDot={{ r: 5, fill: '#FFF', stroke: trendColors.stroke, strokeWidth: 2 }} isAnimationActive />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  )
}
