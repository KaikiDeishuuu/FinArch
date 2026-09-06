import { lazy, Suspense, useEffect, useId, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowRightLeft, ChevronDown, Globe2, Search } from 'lucide-react'
import AnimatedNumber from '../motion/AnimatedNumber'
import { SUPPORTED_CURRENCIES } from '../constants/currencies'
import { fallbackRatesForBase } from '../utils/exchangeRates'
import { Alert } from '../components/ui/alert'
import { Badge } from '../components/ui/badge'
import { Button } from '../components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '../components/ui/card'
import { EmptyState } from '../components/ui/empty-state'
import { Input, Label } from '../components/ui/input'
import { PageHeader } from '../components/ui/page-header'
import { Segmented, SegmentedButton } from '../components/ui/segmented'
import Skeleton from '../components/ui/skeleton'
import { cn } from '../lib/utils'

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
  const labelId = useId()
  const valueId = useId()
  const listboxId = useId()
  const searchId = useId()
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
    <div ref={containerRef} className="relative w-full">
      <Label id={labelId}>{label}</Label>
      <button
        type="button"
        aria-labelledby={`${labelId} ${valueId}`}
        aria-controls={listboxId}
        aria-expanded={open}
        aria-haspopup="listbox"
        onClick={() => setOpen((value) => !value)}
        className="mt-1.5 flex h-11 w-full items-center justify-between gap-3 rounded-lg border border-input bg-card px-3 text-left text-sm shadow-xs transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        <span className="flex min-w-0 items-center gap-2.5">
          <Globe2 className="size-4 shrink-0 text-muted-foreground" />
          <span id={valueId} className="shrink-0 font-mono font-semibold text-foreground">{selected.code}</span>
          <span className="truncate text-muted-foreground">{selected.en}</span>
        </span>
        <ChevronDown className={cn('size-4 shrink-0 text-muted-foreground transition-transform', open && 'rotate-180')} />
      </button>

      {open ? (
        <div className="absolute inset-x-0 z-30 mt-2 rounded-lg border border-border bg-card p-2 shadow-[var(--shadow-sm)]">
          <div className="relative mb-2">
            <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              id={searchId}
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder={t('exchange.searchCurrency')}
              aria-label={t('exchange.searchCurrency')}
              className="pl-9"
            />
          </div>
          <div id={listboxId} role="listbox" aria-labelledby={labelId} className="max-h-56 space-y-0.5 overflow-auto overscroll-contain">
            {options.map((currency) => (
              <button
                key={currency.code}
                type="button"
                role="option"
                aria-selected={currency.code === value}
                onClick={() => { onChange(currency.code); setOpen(false); setQuery('') }}
                className="flex min-h-10 w-full items-center gap-2.5 rounded-md px-3 py-2 text-left text-sm transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <span className="w-10 shrink-0 font-mono font-semibold text-foreground">{currency.code}</span>
                <span className="truncate text-muted-foreground">{currency.en}</span>
              </button>
            ))}
          </div>
        </div>
      ) : null}
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
    ? { arrow: '↑', variant: 'negative' as const, value: `+${trendPct.toFixed(2)}%` }
    : trendPct < -0.02
      ? { arrow: '↓', variant: 'positive' as const, value: `${trendPct.toFixed(2)}%` }
      : { arrow: '→', variant: 'neutral' as const, value: t('exchange.flat') }

  const fromMeta = CURRENCIES.find(c => c.code === from) ?? CURRENCIES[0]
  const toMeta = CURRENCIES.find(c => c.code === to) ?? CURRENCIES[1]

  return (
    <div className="space-y-6">
      <PageHeader title={t('exchange.title')} description={t('exchange.subtitle')} />

      <div className="grid gap-5 xl:grid-cols-5">
        <Card className="min-w-0 xl:col-span-2">
          <CardHeader>
            <div>
              <CardTitle>{t('exchange.amount')}</CardTitle>
              <p className="mt-1 text-xs text-muted-foreground">{fromMeta.code} → {toMeta.code}</p>
            </div>
            {!latestLoading ? <Badge variant={trendMeta.variant}>{trendMeta.arrow} {trendMeta.value}</Badge> : null}
          </CardHeader>

          <CardContent className="space-y-5">
            {latestLoading ? (
              <div className="space-y-3">
                <Skeleton height="h-11" />
                <Skeleton height="h-16" />
              </div>
            ) : (
              <>
                <div>
                  <Label htmlFor="exchange-amount">{t('exchange.amount')}</Label>
                  <div className="mt-1.5 flex items-center gap-3 rounded-lg border border-input bg-card px-3 focus-within:border-ring focus-within:ring-2 focus-within:ring-ring/20">
                    <input
                      id="exchange-amount"
                      type="number"
                      value={amountInput}
                      onChange={(event) => setAmountInput(event.target.value)}
                      className="h-12 min-w-0 flex-1 bg-transparent text-2xl font-semibold tabular-nums text-foreground outline-none"
                    />
                    <span className="font-mono text-xs font-semibold text-muted-foreground">{fromMeta.code}</span>
                  </div>
                </div>

                <div className="grid items-end gap-3 sm:grid-cols-[1fr_auto_1fr]">
                  <CurrencySelector label={t('exchange.fromCurrency')} value={from} onChange={setFrom} peerValue={to} t={t} />
                  <Button
                    variant="outline"
                    size="icon"
                    aria-label={`${t('exchange.fromCurrency')} / ${t('exchange.toCurrency')}`}
                    onClick={() => {
                      setSwapSpin(true)
                      setFrom(to)
                      setTo(from)
                      window.setTimeout(() => setSwapSpin(false), 280)
                    }}
                    className="mx-auto sm:mb-1.5"
                  >
                    <ArrowRightLeft className={cn('size-4 transition-transform', swapSpin && 'rotate-180')} />
                  </Button>
                  <CurrencySelector label={t('exchange.toCurrency')} value={to} onChange={setTo} peerValue={from} t={t} />
                </div>
              </>
            )}

            <div className="rounded-lg border border-border bg-muted/55 p-4">
              {latestLoading ? (
                <div className="space-y-3">
                  <Skeleton height="h-4" width="w-28" />
                  <Skeleton height="h-8" width="w-48" />
                  <Skeleton height="h-4" width="w-64" />
                </div>
              ) : (
                <>
                  <p className="text-xs text-muted-foreground">{debouncedAmount.toLocaleString()} {fromMeta.code}</p>
                  <p className="mt-1 text-3xl font-semibold leading-tight tabular-nums text-foreground">
                    <AnimatedNumber value={converted} formatter={(value) => `${value.toLocaleString(undefined, { maximumFractionDigits: 2 })} ${toMeta.code}`} />
                  </p>
                  <p className="mt-2 text-sm text-muted-foreground">
                    {t('exchange.exchangeRate')}: <span className="font-medium tabular-nums text-foreground">1 {from} = {rate.toFixed(4)} {to}</span>
                  </p>
                  <p className="mt-1 text-xs text-muted-foreground">{t('exchange.lastUpdated', { sec: ageSec })}</p>
                  {error ? <Alert variant="warning" className="mt-3">{error}</Alert> : null}
                </>
              )}
            </div>
          </CardContent>
        </Card>

        <Card className="min-w-0 overflow-hidden xl:col-span-3">
          <CardHeader className="flex-wrap">
            <CardTitle>{from} / {to} {t('exchange.trend')}</CardTitle>
            <Segmented aria-label={t('exchange.trend')}>
              {(['1D', '1W', '1M', '1Y'] as RangeKey[]).map((option) => (
                <SegmentedButton key={option} onClick={() => setRange(option)} aria-pressed={range === option}>
                  {t(`exchange.ranges.${option}`)}
                </SegmentedButton>
              ))}
            </Segmented>
          </CardHeader>
          <CardContent>
            <Suspense fallback={<div className="h-[320px] space-y-3 p-3"><Skeleton height="h-5" width="w-32" /><Skeleton height="h-[260px]" /></div>}>
              {historyLoading ? (
                <div className="h-[320px] space-y-3 p-3"><Skeleton height="h-5" width="w-32" /><Skeleton height="h-[260px]" /></div>
              ) : history.length > 0 ? (
                <ExchangeTrendChart key={chartRenderKey} data={history} from={from} to={to} locale={i18n.language} range={range} />
              ) : (
                <EmptyState className="h-[320px] content-center" title={t('exchange.noHistory')} icon={<Globe2 />} />
              )}
            </Suspense>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
