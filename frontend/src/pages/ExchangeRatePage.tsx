import { useEffect, useMemo, useRef, useState, lazy, Suspense } from 'react'
import { useTranslation } from 'react-i18next'
import AnimatedNumber from '../motion/AnimatedNumber'
import Skeleton from '../motion/Skeleton'

const ExchangeTrendChart = lazy(() => import('../components/ExchangeTrendChart'))

type RangeKey = '1D' | '1W' | '1M' | '1Y'

interface CurrencyMeta {
  code: string
  en: string
  zh: string
}

const CURRENCIES: CurrencyMeta[] = [
  { code: 'USD', en: 'US Dollar', zh: '美元' },
  { code: 'EUR', en: 'Euro', zh: '欧元' },
  { code: 'JPY', en: 'Japanese Yen', zh: '日元' },
  { code: 'CNY', en: 'Chinese Yuan', zh: '人民币' },
  { code: 'GBP', en: 'British Pound', zh: '英镑' },
  { code: 'HKD', en: 'Hong Kong Dollar', zh: '港元' },
  { code: 'CAD', en: 'Canadian Dollar', zh: '加元' },
  { code: 'AUD', en: 'Australian Dollar', zh: '澳元' },
  { code: 'SGD', en: 'Singapore Dollar', zh: '新加坡元' },
  { code: 'KRW', en: 'South Korean Won', zh: '韩元' },
]

const RATE_CACHE_TTL = 60_000
const HISTORY_CACHE_TTL = 5 * 60_000
const rateCache = new Map<string, { updatedAt: number; rates: Record<string, number> }>()
const historyCache = new Map<string, { updatedAt: number; data: Array<{ date: string; rate: number }> }>()
const latestRequestMap = new Map<string, Promise<{ rates: Record<string, number>; updatedAt: number }>>()
const historyRequestMap = new Map<string, Promise<Array<{ date: string; rate: number }>>>()

async function fetchLatest(base: string): Promise<{ rates: Record<string, number>; updatedAt: number }> {
  const cacheHit = rateCache.get(base)
  if (cacheHit && Date.now() - cacheHit.updatedAt < RATE_CACHE_TTL) return cacheHit

  const inflight = latestRequestMap.get(base)
  if (inflight) return inflight

  const request = (async () => {
    const res = await fetch(`https://open.er-api.com/v6/latest/${base}`)
    if (!res.ok) throw new Error('rate fetch failed')
    const data = await res.json() as { rates?: Record<string, number>; time_last_update_unix?: number }
    if (!data.rates) throw new Error('invalid rate payload')
    const payload = {
      rates: data.rates,
      updatedAt: (data.time_last_update_unix ?? Math.floor(Date.now() / 1000)) * 1000,
    }
    rateCache.set(base, payload)
    return payload
  })()

  latestRequestMap.set(base, request)
  return request.finally(() => latestRequestMap.delete(base))
}

function pointsForRange(range: RangeKey) {
  if (range === '1D') return 24
  if (range === '1W') return 7
  if (range === '1M') return 30
  return 52
}

async function fetchHistory(from: string, to: string, range: RangeKey) {
  const key = `${from}:${to}:${range}`
  const hit = historyCache.get(key)
  if (hit && Date.now() - hit.updatedAt < HISTORY_CACHE_TTL) return hit.data

  const inflight = historyRequestMap.get(key)
  if (inflight) return inflight

  const end = new Date()
  const start = new Date()
  if (range === '1D') start.setDate(end.getDate() - 1)
  if (range === '1W') start.setDate(end.getDate() - 7)
  if (range === '1M') start.setMonth(end.getMonth() - 1)
  if (range === '1Y') start.setFullYear(end.getFullYear() - 1)

  const s = start.toISOString().slice(0, 10)
  const e = end.toISOString().slice(0, 10)
  const request = (async () => {
    const res = await fetch(`https://api.frankfurter.app/${s}..${e}?from=${from}&to=${to}`)
    if (!res.ok) throw new Error('history fetch failed')
    const raw = await res.json() as { rates?: Record<string, Record<string, number>> }
    const list = Object.entries(raw.rates ?? {}).map(([date, v]) => ({ date, rate: v[to] ?? 0 })).filter(p => p.rate > 0)
    const step = Math.max(1, Math.floor(list.length / pointsForRange(range)))
    const sampled = list.filter((_, i) => i % step === 0)
    historyCache.set(key, { updatedAt: Date.now(), data: sampled })
    return sampled
  })()

  historyRequestMap.set(key, request)
  return request.finally(() => historyRequestMap.delete(key))
}

function CurrencySelector({
  label,
  value,
  onChange,
  peerValue,
  t,
}: {
  label: string
  value: string
  onChange: (next: string) => void
  peerValue: string
  t: (k: string) => string
}) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const containerRef = useRef<HTMLDivElement>(null)
  const selected = CURRENCIES.find(c => c.code === value) ?? CURRENCIES[0]
  const options = CURRENCIES
    .filter(c => c.code !== peerValue)
    .filter(c => `${c.code} ${c.en} ${c.zh}`.toLowerCase().includes(query.trim().toLowerCase()))

  useEffect(() => {
    if (!open) return

    const handleDocumentClick = (event: MouseEvent) => {
      if (!containerRef.current) return
      if (!containerRef.current.contains(event.target as Node)) {
        setOpen(false)
      }
    }

    document.addEventListener('click', handleDocumentClick)
    return () => document.removeEventListener('click', handleDocumentClick)
  }, [open])

  return (
    <div ref={containerRef} className="relative">
      <p className="mb-1.5 text-xs font-medium text-gray-500 dark:text-gray-400">{label}</p>
      <button
        onClick={() => setOpen(v => !v)}
        className="group flex h-12 w-full items-center justify-between rounded-2xl border border-gray-200/80 bg-white/60 px-4 text-left shadow-sm backdrop-blur-md transition-all duration-300 hover:scale-[1.02] hover:border-blue-300 hover:bg-white hover:shadow-md focus:outline-none focus:ring-4 focus:ring-blue-500/20 dark:border-gray-700/80 dark:bg-black/20 dark:hover:border-blue-500/50 dark:hover:bg-gray-900/60"
      >
        <span className="flex items-center gap-3">
          <span className="flex h-6 w-6 items-center justify-center rounded-lg border border-gray-200 bg-gray-50 text-gray-500 transition-colors group-hover:border-blue-200 group-hover:bg-blue-50 group-hover:text-blue-600 dark:border-gray-700 dark:bg-gray-800 dark:group-hover:border-blue-500/30 dark:group-hover:bg-blue-500/10 dark:group-hover:text-blue-400">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} className="h-3.5 w-3.5"><circle cx="12" cy="12" r="8" /><path strokeLinecap="round" strokeLinejoin="round" d="M4 12h16M12 4a14 14 0 010 16M12 4a14 14 0 000 16" /></svg>
          </span>
          <span className="font-bold text-gray-900 dark:text-gray-100">{selected.code}</span>
          <span className="truncate text-sm text-gray-500 transition-colors group-hover:text-gray-700 dark:text-gray-400 dark:group-hover:text-gray-300">{selected.en}</span>
        </span>
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} className={`h-4 w-4 text-gray-400 transition-transform duration-300 ${open ? 'rotate-180' : ''}`}><path strokeLinecap="round" strokeLinejoin="round" d="m6 9 6 6 6-6" /></svg>
      </button>

      {open && (
        <div className="absolute z-30 mt-2 w-full origin-top transform rounded-[20px] border border-white/60 bg-white/80 p-2.5 shadow-[0_8px_30px_rgb(0,0,0,0.12)] backdrop-blur-2xl transition-all duration-300 dark:border-white/10 dark:bg-gray-900/80">
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t('exchange.searchCurrency')}
            className="mb-2 h-10 w-full rounded-xl border border-gray-200/80 bg-white/50 px-3 text-sm shadow-inner outline-none transition-all focus:border-blue-400 focus:ring-4 focus:ring-blue-500/10 dark:border-gray-700/80 dark:bg-black/20 focus:dark:ring-blue-500/20"
          />
          <div className="max-h-56 overflow-auto space-y-1 rounded-xl p-1 overscroll-contain" style={{ WebkitOverflowScrolling: 'touch' }}>
            {options.map(c => (
              <button
                key={c.code}
                onClick={() => { onChange(c.code); setOpen(false); setQuery('') }}
                className="group flex min-h-[44px] w-full items-center gap-3 rounded-xl px-3 py-2 text-left transition-all duration-200 hover:bg-blue-50 hover:pl-4 dark:hover:bg-blue-500/10"
              >
                <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-lg border border-gray-200 bg-white text-gray-500 transition-colors group-hover:border-blue-200 group-hover:text-blue-600 dark:border-gray-700 dark:bg-gray-800 dark:group-hover:border-blue-500/30 dark:group-hover:text-blue-400">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} className="h-3.5 w-3.5"><circle cx="12" cy="12" r="8" /><path strokeLinecap="round" strokeLinejoin="round" d="M4 12h16M12 4a14 14 0 010 16M12 4a14 14 0 000 16" /></svg>
                </span>
                <span className="font-bold text-gray-900 transition-colors group-hover:text-blue-700 dark:text-gray-100 dark:group-hover:text-blue-300">{c.code}</span>
                <span className="truncate text-sm text-gray-500 transition-colors group-hover:text-blue-600/70 dark:text-gray-400 dark:group-hover:text-blue-400/70">{c.en}</span>
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

export default function ExchangeRatePage() {
  const { t, i18n } = useTranslation()
  const [from, setFrom] = useState<string>('USD')
  const [to, setTo] = useState<string>('JPY')
  const [amountInput, setAmountInput] = useState('100')
  const [debouncedAmount, setDebouncedAmount] = useState(100)
  const [range, setRange] = useState<RangeKey>('1W')
  const [rates, setRates] = useState<Record<string, number>>({})
  const [history, setHistory] = useState<Array<{ date: string; rate: number }>>([])
  const [error, setError] = useState('')
  const [ageSec, setAgeSec] = useState(0)
  const [swapSpin, setSwapSpin] = useState(false)
  const [historyLoading, setHistoryLoading] = useState(false)
  const [latestLoading, setLatestLoading] = useState(true)
  const [chartRenderKey, setChartRenderKey] = useState(0)
  const historyReqId = useRef(0)

  useEffect(() => {
    const id = window.setTimeout(() => {
      const n = Number(amountInput)
      setDebouncedAmount(Number.isFinite(n) ? n : 0)
    }, 360)
    return () => window.clearTimeout(id)
  }, [amountInput])

  useEffect(() => {
    let alive = true
    setLatestLoading(true)
    fetchLatest(from)
      .then((r) => {
        if (!alive) return
        setRates(r.rates)
        setAgeSec(Math.max(0, Math.round((Date.now() - r.updatedAt) / 1000)))
        setError('')
        setLatestLoading(false)
      })
      .catch(() => {
        if (!alive) return
        const fallback: Record<string, number> = {}
        for (const c of CURRENCIES) fallback[c.code] = 1
        fallback.EUR = 0.92
        fallback.JPY = 156
        fallback.GBP = 0.79
        fallback.HKD = 7.8
        fallback.CAD = 1.36
        fallback.AUD = 1.53
        fallback.SGD = 1.34
        fallback.KRW = 1370
        setRates(fallback)
        setError(t('exchange.fallback'))
        setAgeSec(0)
        setLatestLoading(false)
      })
    return () => { alive = false }
  }, [from, t])

  useEffect(() => {
    const requestId = historyReqId.current + 1
    historyReqId.current = requestId
    setHistoryLoading(true)
    const id = window.setTimeout(() => {
      fetchHistory(from, to, range)
        .then((rows) => {
          if (requestId !== historyReqId.current) return
          setHistory(rows)
          setHistoryLoading(false)
        })
        .catch(() => {
          if (requestId !== historyReqId.current) return
          setHistory([])
          setHistoryLoading(false)
        })
    }, 320)
    return () => window.clearTimeout(id)
  }, [from, to, range])

  useEffect(() => {
    const id = window.setInterval(() => setAgeSec((v) => v + 1), 1000)
    return () => window.clearInterval(id)
  }, [])

  useEffect(() => {
    const rerenderChart = () => setChartRenderKey(v => v + 1)
    window.addEventListener('resize', rerenderChart)
    window.addEventListener('orientationchange', rerenderChart)
    return () => {
      window.removeEventListener('resize', rerenderChart)
      window.removeEventListener('orientationchange', rerenderChart)
    }
  }, [])

  const rate = rates[to] ?? 1
  const converted = debouncedAmount * rate
  const trendPct = useMemo(() => {
    if (history.length < 2) return 0
    const start = history[0].rate
    const end = history[history.length - 1].rate
    return start > 0 ? ((end - start) / start) * 100 : 0
  }, [history])

  const trendMeta = trendPct > 0.02
    ? { arrow: '↑', cls: 'text-rose-600', value: `+${trendPct.toFixed(2)}%` }
    : trendPct < -0.02
      ? { arrow: '↓', cls: 'text-emerald-600', value: `${trendPct.toFixed(2)}%` }
      : { arrow: '→', cls: 'text-gray-400', value: t('exchange.flat') }

  const fromMeta = CURRENCIES.find(c => c.code === from) ?? CURRENCIES[0]
  const toMeta = CURRENCIES.find(c => c.code === to) ?? CURRENCIES[1]

  return (
    <div className="relative min-h-[calc(100vh-80px)] space-y-6 font-['Noto_Sans_TC','Noto_Sans_Traditional_Chinese',sans-serif] text-[#111827]">
      {/* Decorative blurred backgrounds */}
      <div className="absolute top-[-10%] left-[-5%] z-[-1] h-[500px] w-[500px] rounded-full bg-blue-500/10 blur-[120px] dark:bg-blue-900/20 pointer-events-none" />
      <div className="absolute bottom-[-10%] right-[-5%] z-[-1] h-[600px] w-[600px] rounded-full bg-purple-500/10 blur-[120px] dark:bg-purple-900/20 pointer-events-none" />

      <div className="relative z-10">
        <h1 className="text-[28px] font-bold tracking-tight text-gray-900 dark:text-gray-100">{t('exchange.title')}</h1>
        <p className="mt-1.5 text-sm text-gray-500 dark:text-gray-400">{t('exchange.subtitle')}</p>
      </div>

      <div className="relative z-10 grid grid-cols-1 gap-6 xl:grid-cols-5">
        <section className="max-w-full xl:col-span-2 relative overflow-hidden rounded-[24px] border border-white/60 bg-white/70 p-5 md:p-6 shadow-[0_8px_30px_rgb(0,0,0,0.04)] backdrop-blur-2xl dark:border-white/10 dark:bg-[#1A1825]/70 transition-all duration-300 hover:shadow-[0_8px_30px_rgb(0,0,0,0.08)]">
          {latestLoading ? (
            <div className="space-y-3">
              <Skeleton height="h-4" width="w-20" />
              <Skeleton height="h-[84px]" />
            </div>
          ) : (
            <>
              <label className="text-sm font-medium text-gray-500 dark:text-gray-400">{t('exchange.amount')}</label>
              <div className="mt-1.5 rounded-2xl border border-gray-200/80 bg-white/50 px-4 py-3 shadow-inner transition-all focus-within:border-blue-400 focus-within:ring-4 focus-within:ring-blue-500/10 dark:border-gray-700/80 dark:bg-black/20 focus-within:dark:ring-blue-500/20">
                <input
                  type="number"
                  value={amountInput}
                  onChange={e => setAmountInput(e.target.value)}
                  className="h-10 w-full bg-transparent text-[28px] font-semibold text-gray-900 outline-none dark:text-gray-100"
                />
                <p className="text-xs text-gray-400">{fromMeta.code}</p>
              </div>
            </>
          )}

          <div className="mt-4 flex flex-col items-center gap-2 md:grid md:grid-cols-[1fr_auto_1fr] md:items-end">
            <div className="w-full">
              {latestLoading ? <Skeleton height="h-16" /> : <CurrencySelector label={t('exchange.fromCurrency')} value={from} onChange={setFrom} peerValue={to} t={t} />}
            </div>
            <button
              onClick={() => {
                setSwapSpin(true)
                setFrom(to)
                setTo(from)
                window.setTimeout(() => setSwapSpin(false), 280)
              }}
              className="group relative z-10 my-2 mx-auto md:my-0 md:mb-0.5 flex h-12 w-12 items-center justify-center rounded-full border border-gray-200/80 bg-white text-gray-500 shadow-sm backdrop-blur-md transition-all duration-300 ease-out hover:scale-110 hover:border-blue-200 hover:text-blue-600 hover:shadow-blue-500/20 focus:outline-none focus:ring-4 focus:ring-blue-500/20 dark:border-gray-700 dark:bg-gray-800 dark:hover:border-blue-500/30 dark:hover:text-blue-400"
            >
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} className={`h-5 w-5 transition-transform duration-300 ease-in-out ${swapSpin ? 'rotate-180 scale-90' : ''}`}><path strokeLinecap="round" strokeLinejoin="round" d="M4 7h12" /><path strokeLinecap="round" strokeLinejoin="round" d="m12 3 4 4-4 4" /><path strokeLinecap="round" strokeLinejoin="round" d="M20 17H8" /><path strokeLinecap="round" strokeLinejoin="round" d="m12 13-4 4 4 4" /></svg>
            </button>
            <div className="w-full">
              {latestLoading ? <Skeleton height="h-16" /> : <CurrencySelector label={t('exchange.toCurrency')} value={to} onChange={setTo} peerValue={from} t={t} />}
            </div>
          </div>

          <button className="mt-5 h-12 w-full rounded-xl bg-gradient-to-r from-blue-600 to-indigo-600 text-sm font-bold tracking-wide text-white shadow-[0_4px_14px_0_rgba(79,70,229,0.39)] transition-all duration-300 hover:-translate-y-0.5 hover:shadow-[0_6px_20px_rgba(79,70,229,0.39)] hover:from-blue-500 hover:to-indigo-500 focus:outline-none focus:ring-4 focus:ring-indigo-500/20 active:scale-[0.98]">
            {t('exchange.convert')}
          </button>

          <div className="mt-6 overflow-hidden relative rounded-3xl border border-white/60 bg-gradient-to-br from-blue-50/50 to-purple-50/50 p-5 backdrop-blur-md shadow-inner dark:border-white/5 dark:from-blue-900/10 dark:to-purple-900/10">
            <div className="absolute -right-10 -top-10 h-32 w-32 rounded-full bg-blue-400/20 blur-[40px] pointer-events-none" />
            {latestLoading ? (
              <div className="space-y-3">
                <Skeleton height="h-4" width="w-28" />
                <Skeleton height="h-10" width="w-48" />
                <Skeleton height="h-4" width="w-64" />
              </div>
            ) : (
              <>
                <p className="text-sm text-gray-500">{debouncedAmount.toLocaleString()} {fromMeta.code}</p>
                <p className="mt-1 text-[32px] font-semibold leading-9 text-gray-900 transition-all duration-300 dark:text-gray-100">
                  <AnimatedNumber value={converted} formatter={(n) => `${n.toLocaleString(undefined, { maximumFractionDigits: 2 })} ${toMeta.code}`} />
                </p>
                <p className="mt-2 text-base text-gray-600 dark:text-gray-300">
                  {t('exchange.exchangeRate')}: 1 {from} = {rate.toFixed(4)} {to}
                  <span className={`ml-2 text-sm font-semibold ${trendMeta.cls}`}>{trendMeta.arrow} {trendMeta.value}</span>
                </p>
                <p className="mt-1 text-[13px] text-gray-400">{t('exchange.lastUpdated', { sec: ageSec })}</p>
                {error && <p className="mt-2 text-xs text-amber-600">{error}</p>}
              </>
            )}
          </div>
        </section>

        <section className="max-w-full overflow-hidden xl:col-span-3 relative rounded-[24px] border border-white/60 bg-white/70 p-5 md:p-6 shadow-[0_8px_30px_rgb(0,0,0,0.04)] backdrop-blur-2xl dark:border-white/10 dark:bg-[#1A1825]/70 transition-all duration-300 hover:shadow-[0_8px_30px_rgb(0,0,0,0.08)]">
          <div className="mb-6 flex items-center justify-between gap-3 flex-wrap">
            <h2 className="text-[1.1rem] font-bold text-gray-900 dark:text-gray-100 tracking-tight">{from} / {to} {t('exchange.trend')}</h2>
            <div className="flex items-center gap-1 rounded-xl bg-black/5 p-1 backdrop-blur-md dark:bg-white/5">
              {(['1D', '1W', '1M', '1Y'] as RangeKey[]).map(r => (
                <button key={r} onClick={() => setRange(r)} className={`min-h-9 rounded-lg px-3.5 text-[13px] font-semibold transition-all duration-300 ${range === r ? 'bg-white text-blue-600 shadow-sm dark:bg-gray-800 dark:text-blue-400' : 'text-gray-500 hover:text-gray-800 hover:bg-black/5 dark:text-gray-400 dark:hover:text-gray-200 dark:hover:bg-white/5'}`}>
                  {t(`exchange.ranges.${r}`)}
                </button>
              ))}
            </div>
          </div>
          <Suspense fallback={<div className="h-[320px] space-y-3 p-3"><Skeleton height="h-5" width="w-32" /><Skeleton height="h-[260px]" /></div>}>
            {historyLoading ? (
              <div className="h-[320px] space-y-3 p-3"><Skeleton height="h-5" width="w-32" /><Skeleton height="h-[260px]" /></div>
            ) : (
              <ExchangeTrendChart key={chartRenderKey} data={history} from={from} to={to} locale={i18n.language} />
            )}
          </Suspense>
        </section>
      </div>
    </div>
  )
}
