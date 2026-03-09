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
  const trendDelta = data.length > 1 ? data[data.length - 1].rate - data[0].rate : 0
  const trendColors = trendDelta > 0
    ? { stroke: '#EF4444', gradientStart: 'rgba(239,68,68,0.12)' }
    : trendDelta < 0
      ? { stroke: '#22C55E', gradientStart: 'rgba(34,197,94,0.12)' }
      : { stroke: '#6B7280', gradientStart: 'rgba(107,114,128,0.12)' }

  return (
    <div className="h-[320px] w-full overflow-hidden">
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={data} margin={{ top: 16, right: 16, left: 36, bottom: 20 }}>
          <defs>
            <linearGradient id="rateGradient" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={trendColors.gradientStart} />
              <stop offset="100%" stopColor="rgba(255,255,255,0)" />
            </linearGradient>
          </defs>
          <CartesianGrid stroke="#E5E7EB" strokeDasharray="3 3" vertical={false} />
          <XAxis dataKey="date" tick={{ fontSize: 11, fill: '#6B7280' }} tickMargin={8} minTickGap={24} tickLine={false} axisLine={false} tickFormatter={(v) => formatDate(String(v), locale)} />
          <YAxis tick={{ fontSize: 11, fill: '#6B7280' }} tickMargin={8} tickLine={false} axisLine={false} width={56} domain={['dataMin - 0.02', 'dataMax + 0.02']} />
          <Tooltip
            cursor={{ stroke: '#93C5FD', strokeWidth: 1 }}
            contentStyle={{ background: '#FFFFFF', borderRadius: 12, border: '1px solid #E5E7EB', boxShadow: '0 8px 20px rgba(15,23,42,0.08)' }}
            labelFormatter={(label) => formatDate(String(label), locale)}
            formatter={(value: number | string | undefined) => [Number(value ?? 0).toFixed(4), `${from}/${to}`]}
          />
          <Area type="monotone" dataKey="rate" stroke={trendColors.stroke} strokeWidth={2} fill="url(#rateGradient)" dot={{ r: 0 }} activeDot={{ r: 5, fill: '#FFF', stroke: trendColors.stroke, strokeWidth: 2 }} />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  )
}
