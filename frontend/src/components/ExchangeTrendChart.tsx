import { LineChart, Line, XAxis, YAxis, CartesianGrid, ResponsiveContainer, Tooltip } from 'recharts'

export interface TrendPoint {
  date: string
  rate: number
}

export default function ExchangeTrendChart({ data, from, to }: { data: TrendPoint[]; from: string; to: string }) {
  return (
    <div className="h-[280px] w-full">
      <ResponsiveContainer width="100%" height="100%">
        <LineChart data={data} margin={{ top: 8, right: 8, left: 0, bottom: 4 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="rgba(148,163,184,0.3)" vertical={false} />
          <XAxis dataKey="date" tick={{ fontSize: 11, fill: '#6B7280' }} tickLine={false} axisLine={false} />
          <YAxis tick={{ fontSize: 11, fill: '#6B7280' }} tickLine={false} axisLine={false} width={56} domain={['dataMin - 0.02', 'dataMax + 0.02']} />
          <Tooltip
            cursor={{ stroke: '#CBD5E1', strokeWidth: 1 }}
            contentStyle={{ borderRadius: 10, border: '1px solid #E5E7EB', fontSize: 12 }}
            formatter={(value: number | string | undefined) => [Number(value ?? 0).toFixed(4), `${from}/${to}`]}
          />
          <Line type="monotone" dataKey="rate" stroke="#2563EB" strokeWidth={2} dot={false} activeDot={{ r: 4 }} />
        </LineChart>
      </ResponsiveContainer>
    </div>
  )
}
