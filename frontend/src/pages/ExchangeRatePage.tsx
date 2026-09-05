import { useEffect, useMemo, useRef, useState, lazy, Suspense } from 'react'
import { useTranslation } from 'react-i18next'
import AnimatedNumber from '../motion/AnimatedNumber'
import Skeleton from '../motion/Skeleton'
import { SUPPORTED_CURRENCIES } from '../constants/currencies'
import { fallbackRatesForBase } from '../utils/exchangeRates'

const ExchangeTrendChart = lazy(() => import('../components/ExchangeTrendChart'))

type RangeKey = '1D' | '1W' | '1M' | '1Y'

const CURRENCIES = SUPPORTED_CURRENCIES

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


function normalizeHistoryRows(rawRates: Record<string, Record<string, number>> | undefined, to: string) {
  return Object.entries(rawRates ?? {})
    .map(([date, row]) => ({ date, rate: Number(row?.[to]) }))
    .filter((point) => Number.isFinite(point.rate) && point.rate > 0)
    .sort((a, b) => a.date.localeCompare(b.date))
}

function fallbackHistory(startDate: string, endDate: string, rate: number) {
  if (!Number.isFinite(rate) || rate <= 0) return []
  return [
    { date: startDate, rate },
    { date: endDate, rate },
  ]
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
    const list = normalizeHistoryRows(raw.rates, to)

    let sampled: Array<{ date: string; rate: number }>
    if (list.length > 0) {
      const step = Math.max(1, Math.floor(list.length / pointsForRange(range)))
      sampled = list.filter((_, i) => i % step === 0)
    } else {
      const latest = await fetchLatest(from)
      sampled = fallbackHistory(s, e, Number(latest.rates[to]))
    }

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
    <div ref={containerRef} className="relative w-full max-w-full box-border">
      <p className="mb-1.5 font-data text-[10px] font-semibold uppercase tracking-[0.12em] text-[hsl(var(--muted-foreground))]">{label}</p>
      <button
        type="button"
        onClick={() => setOpen(v => !v)}
        aria-expanded={open}
        aria-haspopup="listbox"
        className="group flex h-12 w-full max-w-full items-center justify-between overflow-hidden rounded-[3px] border border-[hsl(var(--border))] bg-[hsl(var(--background))] px-3 text-left transition-colors hover:bg-[hsl(var(--card))] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[hsl(var(--ring))] md:px-4" style={{ boxSizing: 'border-box' }}
      >
        <span className="flex items-center gap-3 min-w-0 flex-1">
          <span className="flex h-6 w-6 shrink-0 items-center justify-center border border-[hsl(var(--border))] text-[hsl(var(--mode-accent))] transition-colors group-hover:border-[hsl(var(--mode-accent))]/50">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} className="h-3.5 w-3.5"><circle cx="12" cy="12" r="8" /><path strokeLinecap="round" strokeLinejoin="round" d="M4 12h16M12 4a14 14 0 010 16M12 4a14 14 0 000 16" /></svg>
          </span>
          <span className="font-data shrink-0 font-bold text-[hsl(var(--foreground))]">{selected.code}</span>
          <span className="truncate text-sm text-[hsl(var(--muted-foreground))] transition-colors group-hover:text-[hsl(var(--foreground))]">{selected.en}</span>
        </span>
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} className={`h-4 w-4 shrink-0 text-[hsl(var(--muted-foreground))] transition-transform duration-200 ${open ? 'rotate-180' : ''}`}><path strokeLinecap="round" strokeLinejoin="round" d="m6 9 6 6 6-6" /></svg>
      </button>

      {open && (
        <div role="listbox" className="absolute right-0 z-30 mt-2 w-full max-w-full origin-top border border-[hsl(var(--border))] bg-[hsl(var(--card))] p-2 shadow-lg shadow-black/10" style={{ boxSizing: 'border-box' }}>
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t('exchange.searchCurrency')}
            className="mb-2 h-10 w-full rounded-[3px] border border-[hsl(var(--border))] bg-[hsl(var(--background))] px-3 text-sm text-[hsl(var(--foreground))] outline-none transition-colors placeholder:text-[hsl(var(--muted-foreground))] focus:border-[hsl(var(--mode-accent))] focus:ring-2 focus:ring-[hsl(var(--ring))]/20"
          />
          <div className="max-h-56 space-y-0.5 overflow-auto p-1 overscroll-contain" style={{ WebkitOverflowScrolling: 'touch' }}>
            {options.map(c => (
              <button
                key={c.code}
                type="button"
                role="option"
                aria-selected={c.code === value}
                onClick={() => { onChange(c.code); setOpen(false); setQuery('') }}
                className="group flex min-h-[44px] w-full items-center gap-3 rounded-[2px] px-3 py-2 text-left transition-colors hover:bg-[hsl(var(--mode-accent-wash))]"
              >
                <span className="flex h-6 w-6 shrink-0 items-center justify-center border border-[hsl(var(--border))] text-[hsl(var(--muted-foreground))] transition-colors group-hover:border-[hsl(var(--mode-accent))]/50 group-hover:text-[hsl(var(--mode-accent))]">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} className="h-3.5 w-3.5"><circle cx="12" cy="12" r="8" /><path strokeLinecap="round" strokeLinejoin="round" d="M4 12h16M12 4a14 14 0 010 16M12 4a14 14 0 000 16" /></svg>
                </span>
                <span className="font-data shrink-0 font-bold text-[hsl(var(--foreground))]">{c.code}</span>
                <span className="truncate text-sm text-[hsl(var(--muted-foreground))]">{c.en}</span>
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
    const loadingTimer = window.setTimeout(() => setLatestLoading(true), 0)
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
        const fallback = fallbackRatesForBase(from, CURRENCIES.map((currency) => currency.code))
        setRates(fallback)
        setError(t('exchange.fallback'))
        setAgeSec(0)
        setLatestLoading(false)
      })
    return () => {
      alive = false
      window.clearTimeout(loadingTimer)
    }
  }, [from, t])

  useEffect(() => {
    const requestId = historyReqId.current + 1
    historyReqId.current = requestId
    const loadingTimer = window.setTimeout(() => setHistoryLoading(true), 0)
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
    return () => {
      window.clearTimeout(loadingTimer)
      window.clearTimeout(id)
    }
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
      : { arrow: '→', cls: 'text-[hsl(var(--muted-foreground))]', value: t('exchange.flat') }

  const fromMeta = CURRENCIES.find(c => c.code === from) ?? CURRENCIES[0]
  const toMeta = CURRENCIES.find(c => c.code === to) ?? CURRENCIES[1]

  return (
    <div className="min-h-[calc(100vh-80px)] space-y-6 text-[hsl(var(--foreground))]">
      <div className="ledger-rail border-y border-[hsl(var(--border))] py-5 pl-5 sm:py-6 sm:pl-7">
        <p className="page-kicker mb-2">FINARCH / FX DESK</p>
        <h1 className="font-display text-[28px] font-semibold tracking-[-0.025em] text-[hsl(var(--foreground))]">{t('exchange.title')}</h1>
        <p className="mt-1.5 text-sm text-[hsl(var(--muted-foreground))]">{t('exchange.subtitle')}</p>
      </div>

      <div className="grid grid-cols-1 gap-5 xl:grid-cols-5">
        <section className="relative max-w-full border border-[hsl(var(--border))] border-t-2 border-t-[hsl(var(--mode-accent))] bg-[hsl(var(--card))] p-5 xl:col-span-2 md:p-6">
          {latestLoading ? (
            <div className="space-y-3">
              <Skeleton height="h-4" width="w-20" />
              <Skeleton height="h-[84px]" />
            </div>
          ) : (
            <>
              <label className="font-data text-[10px] font-semibold uppercase tracking-[0.12em] text-[hsl(var(--muted-foreground))]">{t('exchange.amount')}</label>
              <div className="mt-1.5 border border-[hsl(var(--border))] bg-[hsl(var(--background))] px-4 py-3 transition-colors focus-within:border-[hsl(var(--mode-accent))] focus-within:ring-2 focus-within:ring-[hsl(var(--ring))]/20">
                <input
                  type="number"
                  value={amountInput}
                  onChange={e => setAmountInput(e.target.value)}
                  className="font-data h-10 w-full bg-transparent text-[28px] font-semibold text-[hsl(var(--foreground))] outline-none"
                />
                <p className="font-data text-xs text-[hsl(var(--muted-foreground))]">{fromMeta.code}</p>
              </div>
            </>
          )}

          <div className="mt-4 flex flex-col md:flex-row items-center gap-3 md:gap-4 relative">
            <div className="w-full flex-1 min-w-0">
              {latestLoading ? <Skeleton height="h-16" /> : <CurrencySelector label={t('exchange.fromCurrency')} value={from} onChange={setFrom} peerValue={to} t={t} />}
            </div>
            <div className="z-10 -my-1 shrink-0 md:my-0 md:mt-[24px]">
              <button
                type="button"
                aria-label={`${t('exchange.fromCurrency')} / ${t('exchange.toCurrency')}`}
                onClick={() => {
                  setSwapSpin(true)
                  setFrom(to)
                  setTo(from)
                  window.setTimeout(() => setSwapSpin(false), 280)
                }}
                className="group relative flex h-12 w-12 items-center justify-center rounded-[3px] border border-[hsl(var(--border))] bg-[hsl(var(--card))] text-[hsl(var(--mode-accent))] transition-colors hover:bg-[hsl(var(--mode-accent-wash))] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[hsl(var(--ring))]"
              >
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} className={`h-5 w-5 transition-transform duration-300 ease-in-out ${swapSpin ? 'rotate-180 scale-90' : ''}`}><path strokeLinecap="round" strokeLinejoin="round" d="M4 7h12" /><path strokeLinecap="round" strokeLinejoin="round" d="m12 3 4 4-4 4" /><path strokeLinecap="round" strokeLinejoin="round" d="M20 17H8" /><path strokeLinecap="round" strokeLinejoin="round" d="m12 13-4 4 4 4" /></svg>
              </button>
            </div>
            <div className="w-full flex-1 min-w-0">
              {latestLoading ? <Skeleton height="h-16" /> : <CurrencySelector label={t('exchange.toCurrency')} value={to} onChange={setTo} peerValue={from} t={t} />}
            </div>
          </div>

          <button type="button" className="mt-5 h-12 w-full rounded-[3px] bg-[hsl(var(--mode-accent))] text-sm font-bold tracking-wide text-[hsl(var(--primary-foreground))] transition-colors hover:brightness-95 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[hsl(var(--ring))] focus-visible:ring-offset-2">
            {t('exchange.convert')}
          </button>

          <div className="relative mt-6 overflow-hidden border border-[hsl(var(--border))] border-l-2 border-l-[hsl(var(--mode-accent))] bg-[hsl(var(--muted))]/45 p-5">
            {latestLoading ? (
              <div className="space-y-3">
                <Skeleton height="h-4" width="w-28" />
                <Skeleton height="h-10" width="w-48" />
                <Skeleton height="h-4" width="w-64" />
              </div>
            ) : (
              <>
                <p className="font-data text-sm text-[hsl(var(--muted-foreground))]">{debouncedAmount.toLocaleString()} {fromMeta.code}</p>
                <p className="font-data mt-1 text-[32px] font-semibold leading-9 text-[hsl(var(--foreground))]">
                  <AnimatedNumber value={converted} formatter={(n) => `${n.toLocaleString(undefined, { maximumFractionDigits: 2 })} ${toMeta.code}`} />
                </p>
                <p className="font-data mt-2 text-base text-[hsl(var(--foreground))]">
                  {t('exchange.exchangeRate')}: 1 {from} = {rate.toFixed(4)} {to}
                  <span className={`ml-2 text-sm font-semibold ${trendMeta.cls}`}>{trendMeta.arrow} {trendMeta.value}</span>
                </p>
                <p className="font-data mt-1 text-[13px] text-[hsl(var(--muted-foreground))]">{t('exchange.lastUpdated', { sec: ageSec })}</p>
                {error && <p className="mt-2 text-xs text-amber-600">{error}</p>}
              </>
            )}
          </div>
        </section>

        <section className="relative max-w-full overflow-hidden border border-[hsl(var(--border))] bg-[hsl(var(--card))] p-4 sm:p-5 xl:col-span-3 md:p-6">
          <div className="mb-6 flex items-center justify-between gap-3 flex-wrap">
            <h2 className="font-display text-[1.1rem] font-semibold tracking-tight text-[hsl(var(--foreground))]">{from} / {to} {t('exchange.trend')}</h2>
            <div role="group" aria-label={t('exchange.trend')} className="flex items-center gap-px border border-[hsl(var(--border))] bg-[hsl(var(--border))] p-px">
              {(['1D', '1W', '1M', '1Y'] as RangeKey[]).map(r => (
                <button key={r} type="button" onClick={() => setRange(r)} aria-pressed={range === r} className={`min-h-9 bg-[hsl(var(--card))] px-3.5 font-data text-[13px] font-semibold transition-colors ${range === r ? 'text-[hsl(var(--mode-accent))] underline decoration-2 underline-offset-4' : 'text-[hsl(var(--muted-foreground))] hover:bg-[hsl(var(--muted))] hover:text-[hsl(var(--foreground))]'}`}>
                  {t(`exchange.ranges.${r}`)}
                </button>
              ))}
            </div>
          </div>
          <Suspense fallback={<div className="h-[320px] space-y-3 p-3"><Skeleton height="h-5" width="w-32" /><Skeleton height="h-[260px]" /></div>}>
            {historyLoading ? (
              <div className="h-[320px] space-y-3 p-3"><Skeleton height="h-5" width="w-32" /><Skeleton height="h-[260px]" /></div>
            ) : history.length > 0 ? (
              <ExchangeTrendChart key={chartRenderKey} data={history} from={from} to={to} locale={i18n.language} range={range} />
            ) : (
              <div className="flex h-[320px] items-center justify-center border border-dashed border-[hsl(var(--border))] text-sm text-[hsl(var(--muted-foreground))]">
                {t('exchange.noHistory')}
              </div>
            )}
          </Suspense>
        </section>
      </div>
    </div>
  )
}
