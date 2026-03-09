import { useEffect, useMemo, useState, lazy, Suspense } from 'react'

const ExchangeTrendChart = lazy(() => import('../components/ExchangeTrendChart'))

type RangeKey = '1D' | '1W' | '1M' | '1Y'
const CURRENCIES = ['USD', 'EUR', 'JPY', 'CNY', 'GBP', 'HKD', 'CAD', 'AUD', 'SGD', 'KRW'] as const

async function fetchLatest(base: string): Promise<{ rates: Record<string, number>; updatedAt: number }> {
  const res = await fetch(`https://open.er-api.com/v6/latest/${base}`)
  if (!res.ok) throw new Error('rate fetch failed')
  const data = await res.json() as { rates?: Record<string, number>; time_last_update_unix?: number }
  if (!data.rates) throw new Error('invalid rate payload')
  return { rates: data.rates, updatedAt: (data.time_last_update_unix ?? Math.floor(Date.now() / 1000)) * 1000 }
}

function pointsForRange(range: RangeKey) {
  if (range === '1D') return 24
  if (range === '1W') return 7
  if (range === '1M') return 30
  return 52
}

async function fetchHistory(from: string, to: string, range: RangeKey) {
  const end = new Date()
  const start = new Date()
  if (range === '1D') start.setDate(end.getDate() - 1)
  if (range === '1W') start.setDate(end.getDate() - 7)
  if (range === '1M') start.setMonth(end.getMonth() - 1)
  if (range === '1Y') start.setFullYear(end.getFullYear() - 1)

  const s = start.toISOString().slice(0, 10)
  const e = end.toISOString().slice(0, 10)
  const res = await fetch(`https://api.frankfurter.app/${s}..${e}?from=${from}&to=${to}`)
  if (!res.ok) throw new Error('history fetch failed')
  const raw = await res.json() as { rates?: Record<string, Record<string, number>> }
  const list = Object.entries(raw.rates ?? {}).map(([date, v]) => ({ date: date.slice(5), rate: v[to] ?? 0 })).filter(p => p.rate > 0)
  const step = Math.max(1, Math.floor(list.length / pointsForRange(range)))
  return list.filter((_, i) => i % step === 0)
}

export default function ExchangeRatePage() {
  const [from, setFrom] = useState<string>('USD')
  const [to, setTo] = useState<string>('JPY')
  const [amount, setAmount] = useState<number>(100)
  const [range, setRange] = useState<RangeKey>('1W')
  const [rates, setRates] = useState<Record<string, number>>({})
  const [history, setHistory] = useState<Array<{ date: string; rate: number }>>([])
  const [error, setError] = useState('')
  const [ageSec, setAgeSec] = useState(0)

  useEffect(() => {
    let alive = true
    fetchLatest(from)
      .then((r) => {
        if (!alive) return
        setRates(r.rates)
        void r.updatedAt
        setAgeSec(0)
        setError('')
      })
      .catch(() => {
        if (!alive) return
        const fallback: Record<string, number> = {}
        for (const c of CURRENCIES) fallback[c] = 1
        fallback.CNY = 1
        fallback.USD = 1
        fallback.EUR = 0.92
        fallback.JPY = 156
        fallback.GBP = 0.79
        fallback.HKD = 7.8
        fallback.CAD = 1.36
        fallback.AUD = 1.53
        fallback.SGD = 1.34
        fallback.KRW = 1370
        setRates(fallback)
        setError('Live exchange rate unavailable, fallback data in use.')
        setAgeSec(0)
      })
    return () => { alive = false }
  }, [from])

  useEffect(() => {
    let alive = true
    fetchHistory(from, to, range)
      .then((rows) => { if (alive) setHistory(rows) })
      .catch(() => { if (alive) setHistory([]) })
    return () => { alive = false }
  }, [from, to, range])

  useEffect(() => {
    const id = window.setInterval(() => setAgeSec((v) => v + 1), 1000)
    return () => window.clearInterval(id)
  }, [])

  const rate = useMemo(() => {
    if (!rates[to]) return 1
    return rates[to]
  }, [rates, to])

  const converted = (Number.isFinite(amount) ? amount : 0) * rate

  return (
    <div className="space-y-5">
      <div>
        <h1 className="text-[26px] font-bold tracking-tight text-gray-900 dark:text-gray-100">Exchange Rate Calculator</h1>
        <p className="text-[14px] text-gray-500 dark:text-gray-400 mt-1">Real-time conversion with trend insight.</p>
      </div>

      <section className="rounded-2xl border border-[#E6E8EB] bg-white dark:bg-[hsl(260,15%,11%)] p-4 md:p-5 shadow-[0_6px_18px_rgba(0,0,0,0.04)]">
        <div className="grid grid-cols-1 md:grid-cols-4 gap-3">
          <label className="text-sm text-gray-500">From
            <select className="mt-1 w-full rounded-xl border border-gray-200 bg-white dark:bg-gray-900/50 p-2.5" value={from} onChange={e => setFrom(e.target.value)}>
              {CURRENCIES.map(c => <option key={c} value={c}>{c}</option>)}
            </select>
          </label>
          <label className="text-sm text-gray-500">To
            <select className="mt-1 w-full rounded-xl border border-gray-200 bg-white dark:bg-gray-900/50 p-2.5" value={to} onChange={e => setTo(e.target.value)}>
              {CURRENCIES.map(c => <option key={c} value={c}>{c}</option>)}
            </select>
          </label>
          <label className="text-sm text-gray-500 md:col-span-2">Amount
            <input type="number" className="mt-1 w-full rounded-xl border border-gray-200 bg-white dark:bg-gray-900/50 p-2.5" value={amount} onChange={e => setAmount(Number(e.target.value))} />
          </label>
        </div>

        <div className="mt-4 rounded-xl border border-gray-100 dark:border-gray-800 bg-[#F7F8FA] dark:bg-gray-900/40 p-4">
          <p className="text-sm text-gray-500">Converted amount</p>
          <p className="text-[30px] leading-9 font-semibold text-gray-900 dark:text-gray-100 mt-1">{converted.toLocaleString(undefined, { maximumFractionDigits: 2 })} {to}</p>
          <p className="text-sm text-gray-500 mt-2">Rate: 1 {from} = {rate.toFixed(4)} {to}</p>
          <p className="text-xs text-gray-400 mt-1">Last updated: {ageSec}s ago</p>
          {error && <p className="text-xs text-amber-600 mt-2">{error}</p>}
        </div>
      </section>

      <section className="rounded-2xl border border-[#E6E8EB] bg-white dark:bg-[hsl(260,15%,11%)] p-4 md:p-5 shadow-[0_6px_18px_rgba(0,0,0,0.04)]">
        <div className="flex items-center justify-between gap-3 mb-4">
          <h2 className="text-[18px] font-semibold text-gray-900 dark:text-gray-100">{from} → {to} Trend</h2>
          <div className="flex items-center gap-1 rounded-lg bg-gray-100 dark:bg-gray-800 p-1">
            {(['1D', '1W', '1M', '1Y'] as RangeKey[]).map(r => (
              <button key={r} onClick={() => setRange(r)} className={`px-2.5 py-1 text-xs rounded-md transition ${range === r ? 'bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 shadow-sm' : 'text-gray-500 dark:text-gray-400'}`}>
                {r}
              </button>
            ))}
          </div>
        </div>
        <Suspense fallback={<div className="h-[280px] flex items-center justify-center text-sm text-gray-400">Loading chart…</div>}>
          <ExchangeTrendChart data={history} from={from} to={to} />
        </Suspense>
      </section>
    </div>
  )
}
