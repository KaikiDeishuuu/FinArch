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
  return (
    <div className="h-[320px] w-full">
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={data} margin={{ top: 8, right: 10, left: 2, bottom: 2 }}>
          <defs>
            <linearGradient id="rateGradient" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="rgba(59,130,246,0.24)" />
              <stop offset="100%" stopColor="rgba(59,130,246,0.03)" />
            </linearGradient>
          </defs>
          <CartesianGrid stroke="#E5E7EB" strokeDasharray="3 3" vertical={false} />
          <XAxis dataKey="date" tick={{ fontSize: 11, fill: '#6B7280' }} tickLine={false} axisLine={false} tickFormatter={(v) => formatDate(String(v), locale)} />
          <YAxis tick={{ fontSize: 11, fill: '#6B7280' }} tickLine={false} axisLine={false} width={56} domain={['dataMin - 0.02', 'dataMax + 0.02']} />
          <Tooltip
            cursor={{ stroke: '#93C5FD', strokeWidth: 1 }}
            contentStyle={{ borderRadius: 12, border: '1px solid #E5E7EB', boxShadow: '0 8px 20px rgba(15,23,42,0.08)' }}
            labelFormatter={(label) => formatDate(String(label), locale)}
            formatter={(value: number | string | undefined) => [Number(value ?? 0).toFixed(4), `${from}/${to}`]}
          />
          <Area type="monotone" dataKey="rate" stroke="#3B82F6" strokeWidth={2} fill="url(#rateGradient)" dot={false} activeDot={{ r: 4, stroke: '#3B82F6', strokeWidth: 2 }} />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  )
}
