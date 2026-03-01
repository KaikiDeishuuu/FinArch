import { useState, useRef, useEffect } from 'react'
import type { FormEvent } from 'react'
import { toggleReimbursed, toggleUploaded } from '../api/client'
import type { MatchResult, MatchResultItem } from '../api/client'
import { formatAmount, toCNY } from '../utils/format'
import { useExchangeRates } from '../contexts/ExchangeRateContext'
import { useTransactions, useInvalidateTransactions } from '../hooks/useTransactions'
import type { WorkerTxItem, WorkerResult } from '../workers/match.worker'
import MatchWorkerConstructor from '../workers/match.worker.ts?worker'

export default function MatchPage() {
  const [target, setTarget] = useState('')
  const [tolerance, setTolerance] = useState('0.01')
  const [maxItems, setMaxItems] = useState('10')
  const [results, setResults] = useState<MatchResult[]>([])
  const [timePruned, setTimePruned] = useState(false)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [searched, setSearched] = useState(false)
  const [expandedIdx, setExpandedIdx] = useState<number | null>(null)
  const [confirmId, setConfirmId] = useState<string | null>(null)
  const [loadingId, setLoadingId] = useState<string | null>(null)
  const [reimbursedIds, setReimbursedIds] = useState<Set<string>>(new Set())
  const { rates } = useExchangeRates()
  const { data: txs = [] } = useTransactions()
  const invalidate = useInvalidateTransactions()
  const workerRef = useRef<Worker | null>(null)

  // Lazily create the worker on first use
  function getWorker(): Worker {
    if (!workerRef.current) {
      workerRef.current = new MatchWorkerConstructor()
    }
    return workerRef.current
  }

  useEffect(() => {
    return () => { workerRef.current?.terminate() }
  }, [])

  // Compute CNY total for a result using live rates from its items
  function cnyTotal(r: MatchResult): number {
    if (!r.items?.length) return r.total
    return r.items.reduce((s, item) => s + toCNY(item.amount_yuan, item.currency || 'CNY', rates), 0)
  }
  // True if a result contains non-CNY currencies
  function hasMixedCurrency(r: MatchResult): boolean {
    return !!r.items?.some(item => item.currency && item.currency.toUpperCase() !== 'CNY')
  }

  async function handleReimburse(id: string, alreadyUploaded: boolean) {
    setLoadingId(id)
    try {
      await Promise.all([
        toggleReimbursed(id),
        ...(alreadyUploaded ? [] : [toggleUploaded(id)]),
      ])
      setReimbursedIds(prev => new Set(prev).add(id))
      invalidate()
    } finally {
      setLoadingId(null)
      setConfirmId(null)
    }
  }

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError('')
    const t = parseFloat(target)
    if (isNaN(t) || t <= 0) { setError('请输入有效目标金额'); return }
    const tol = parseFloat(tolerance) || 0
    const maxD = parseInt(maxItems) || 10

    setLoading(true)
    setResults([])
    setTimePruned(false)
    setSearched(false)
    setExpandedIdx(null)
    setConfirmId(null)
    setReimbursedIds(new Set())

    // Build a lookup map from tx id → tx
    const txMap = new Map(txs.map(tx => [tx.id, tx]))

    // Filter: uploaded, unreimbursed, personal-expense
    const candidates = txs.filter(tx =>
      tx.source === 'personal' &&
      tx.direction === 'expense' &&
      tx.uploaded &&
      !tx.reimbursed
    )

    const workerItems: WorkerTxItem[] = candidates.map(tx => ({
      id: tx.id,
      amountCents: Math.round(tx.amount_yuan * 100),
      occurredTs: Math.floor(new Date(tx.occurred_at).getTime() / 1000),
      projectId: tx.project_id ?? undefined,
    }))

    const worker = getWorker()
    worker.onmessage = (ev: MessageEvent<{ ok: boolean; results?: WorkerResult[]; error?: string }>) => {
      setLoading(false)
      setSearched(true)
      if (!ev.data.ok) {
        setError(ev.data.error || '匹配失败，请重试')
        return
      }
      const workerResults = ev.data.results ?? []
      const pruned = workerResults.some(r => r.timePruned)
      setTimePruned(pruned)

      // Map WorkerResult → MatchResult, enriching with cached tx data
      const mapped: MatchResult[] = workerResults.map(wr => {
        const items: MatchResultItem[] = wr.ids.flatMap(id => {
            const tx = txMap.get(id)
            if (!tx) return []
            const item: MatchResultItem = {
              id: tx.id,
              occurred_at: tx.occurred_at,
              direction: tx.direction,
              source: tx.source,
              category: tx.category,
              amount_yuan: tx.amount_yuan,
              currency: tx.currency,
              note: tx.note ?? '',
              project_id: tx.project_id ?? '',
              uploaded: tx.uploaded,
            }
            return [item]
          })
        return {
          ids: wr.ids,
          total: wr.totalCents / 100,
          error: wr.errorCents / 100,
          project_count: wr.projectCount,
          item_count: wr.itemCount,
          items,
          total_cents: wr.totalCents,
          error_cents: wr.errorCents,
          score: wr.score,
          time_pruned: wr.timePruned,
        }
      })
      setResults(mapped)
    }
    worker.onerror = (err) => {
      setLoading(false)
      setSearched(true)
      setError(err.message || '匹配失败，请重试')
    }
    worker.postMessage({
      targetCents: Math.round(t * 100),
      toleranceCents: Math.round(tol * 100),
      maxDepth: maxD,
      limit: 20,
      items: workerItems,
    })
  }

  const fmt = (amount: number, currency: string) => formatAmount(amount, currency)
  const inputClass = 'w-full border border-gray-200 rounded-xl px-3.5 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent bg-gray-50 transition-all hover:bg-white tabular-nums'
  const labelClass = 'block text-xs font-semibold text-gray-500 mb-2 uppercase tracking-wider'

  return (
    <div className="space-y-6 max-w-3xl">
      {/* Header */}
      <div>
        <h1 className="text-2xl font-bold text-gray-900 tracking-tight">子集匹配</h1>
        <p className="text-sm text-gray-400 mt-1">自动搜索与报销总额精确匹配的交易组合</p>
      </div>

      {/* Form card */}
      <div className="bg-white rounded-2xl border border-gray-100 shadow-sm overflow-hidden">
        {/* Info bar */}
        <div className="bg-blue-50 border-b border-blue-100 px-5 py-3 flex items-start gap-2.5">
          <svg className="w-4 h-4 text-blue-500 shrink-0 mt-0.5" fill="currentColor" viewBox="0 0 20 20"><path fillRule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clipRule="evenodd" /></svg>
          <div>
            <p className="text-sm text-blue-700">在<strong>已上传、未报销</strong>的个人庞付记录中，找出金额之和与目标最接近的组合</p>
            <p className="text-xs text-blue-500/80 mt-1">注意：匹配算法基于登记金额（原币对应数字）进行匹配；如项目包含多币种交易，请手动按当前汇率换算后输入目标金额。</p>
          </div>
        </div>

        <form onSubmit={handleSubmit} className="p-5">
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
            <div>
              <label className={labelClass}>目标金额（元）</label>
              <input
                type="number"
                required
                min="0.01"
                step="0.01"
                className={inputClass}
                placeholder="如：1200.00"
                value={target}
                onChange={(e) => setTarget(e.target.value)}
              />
            </div>
            <div>
              <label className={labelClass}>允许误差（元）</label>
              <input
                type="number"
                min="0"
                step="0.01"
                className={inputClass}
                value={tolerance}
                onChange={(e) => setTolerance(e.target.value)}
              />
            </div>
            <div>
              <label className={labelClass}>最多交易笔数</label>
              <input
                type="number"
                min="1"
                max="50"
                className={inputClass}
                value={maxItems}
                onChange={(e) => setMaxItems(e.target.value)}
              />
            </div>
          </div>

          {error && (
            <div className="mt-4 bg-red-50 border border-red-200 text-red-700 rounded-xl px-4 py-3 text-sm flex items-center gap-2">
              <svg className="w-4 h-4 shrink-0" fill="currentColor" viewBox="0 0 20 20"><path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" /></svg>
              {error}
            </div>
          )}

          <button
            type="submit"
            disabled={loading}
            className="mt-5 w-full bg-blue-600 hover:bg-blue-700 disabled:opacity-50 text-white font-semibold rounded-xl py-2.5 text-sm transition-all shadow-sm flex items-center justify-center gap-2"
          >
            {loading ? (
              <>
                <span className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin" />
                搜索中…
              </>
            ) : (
              <>
                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2.5}><path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" /></svg>
                开始匹配
              </>
            )}
          </button>
        </form>
      </div>

      {/* Results */}
      {searched && (
        <div className="space-y-3">
          {/* Time-pruned warning */}
          {timePruned && (
            <div className="flex items-start gap-2.5 bg-amber-50 border border-amber-200 rounded-xl px-4 py-3 text-sm text-amber-700">
              <svg className="w-4 h-4 shrink-0 mt-0.5" fill="currentColor" viewBox="0 0 20 20"><path fillRule="evenodd" d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z" clipRule="evenodd" /></svg>
              <span>候选项较多，已自动限制为最近 90 天数据以加速搜索</span>
            </div>
          )}

          {/* Result header */}
          <div className="flex items-center justify-between px-1">
            {results.length === 0 ? (
              <div className="flex items-center gap-2 text-gray-500">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-5 h-5 shrink-0"><circle cx="11" cy="11" r="8"/><path d="M21 21l-4.35-4.35"/></svg>
                <span className="text-sm">未找到匹配方案，尝试调大误差范围</span>
              </div>
            ) : (
              <div className="flex items-center gap-2">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-5 h-5 text-green-500 shrink-0"><polyline points="20 6 9 17 4 12"/></svg>
                <span className="text-sm text-gray-600">
                  共找到 <span className="text-blue-600 font-bold text-base">{results.length}</span> 个匹配方案
                </span>
              </div>
            )}
          </div>

          {results.map((r, i) => {
            const rankColors = [
              'bg-yellow-500',  // gold
              'bg-gray-400',    // silver
              'bg-amber-700',   // bronze
            ]
            const rankBg = i < 3 ? rankColors[i] : 'bg-blue-600'
            return (
            <div key={i} className="bg-white rounded-2xl border border-gray-100 shadow-sm overflow-hidden">
              {/* Card header (clickable) */}
              <button
                className="w-full px-5 py-4 flex items-center justify-between hover:bg-gray-50/60 transition-colors"
                onClick={() => setExpandedIdx(expandedIdx === i ? null : i)}
              >
                <div className="flex items-center gap-4 text-left">
                  <span className={`flex items-center justify-center w-8 h-8 rounded-xl ${rankBg} text-white text-xs font-bold shrink-0 shadow-sm`}>
                    {i + 1}
                  </span>
                  <div>
                    <div className="flex items-center gap-2 flex-wrap">
                      <p className="font-bold text-gray-800 text-base">{fmt(cnyTotal(r), 'CNY')}</p>
                      {r.error <= 0.01 && (
                        <span className="text-xs bg-green-100 text-green-700 px-2 py-0.5 rounded-full font-medium">精确匹配</span>
                      )}
                      {r.score != null && (
                        <span className="text-xs bg-blue-50 text-blue-600 px-2 py-0.5 rounded-full font-medium tabular-nums">
                          Score {r.score.toFixed(3)}
                        </span>
                      )}
                      {hasMixedCurrency(r) && (
                        <span className="text-xs bg-amber-100 text-amber-700 px-2 py-0.5 rounded-full font-medium">含外币·已汇率换算</span>
                      )}
                    </div>
                    <p className="text-xs text-gray-400 mt-0.5 flex items-center gap-2">
                      <span>{r.item_count} 笔</span>
                      <span className="w-1 h-1 rounded-full bg-gray-300" />
                      <span>{r.project_count} 个项目</span>
                    </p>
                  </div>
                </div>
                <div className={`w-7 h-7 rounded-full flex items-center justify-center transition-transform ${expandedIdx === i ? 'bg-blue-100 text-blue-600 rotate-180' : 'bg-gray-100 text-gray-400'}`}>
                  <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" /></svg>
                </div>
              </button>

              {/* Expanded detail */}
              {expandedIdx === i && (
                <div className="border-t border-gray-100">
                  {r.items && r.items.length > 0 ? (
                    <>
                      {/* Mobile: card list */}
                      <div className="md:hidden divide-y divide-gray-50">
                        {r.items.map((item) => {
                          const done = reimbursedIds.has(item.id)
                          const confirming = confirmId === item.id
                          const busy = loadingId === item.id
                          return (
                            <div key={item.id} className={`px-4 py-3 space-y-1.5 ${done ? 'opacity-60' : ''}`}>
                              <div className="flex items-center justify-between gap-2">
                                <span className={`font-semibold text-sm ${done ? 'text-gray-400 line-through' : 'text-gray-700'}`}>{item.category}</span>
                                <span className={`font-bold tabular-nums whitespace-nowrap text-sm ${done ? 'text-gray-400 line-through' : 'text-red-500'}`}>−{fmt(item.amount_yuan, item.currency)}</span>
                              </div>
                              <div className="flex items-center gap-2 flex-wrap text-xs text-gray-400">
                                <span className="tabular-nums">{item.occurred_at}</span>
                                {item.project_id && (
                                  <span className="font-mono bg-gray-100 text-gray-600 px-1.5 py-0.5 rounded">{item.project_id}</span>
                                )}
                                {item.note && <span className="truncate max-w-[180px]">{item.note}</span>}
                              </div>
                              {done ? (
                                <span className="inline-flex items-center gap-1 text-xs text-green-600 font-medium">
                                  <svg className="w-3.5 h-3.5" fill="currentColor" viewBox="0 0 20 20"><path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" /></svg>
                                  已报销
                                </span>
                              ) : confirming ? (
                                <div className="flex items-center gap-2 pt-0.5">
                                  <span className="text-xs text-gray-500">确认标记已报销？</span>
                                  <button onClick={() => handleReimburse(item.id, item.uploaded)} disabled={busy}
                                    className="text-xs bg-green-600 hover:bg-green-700 disabled:opacity-50 text-white px-2.5 py-1 rounded-lg font-medium">
                                    {busy ? '…' : '确认'}
                                  </button>
                                  <button onClick={() => setConfirmId(null)}
                                    className="text-xs text-gray-400 hover:text-gray-600 px-2 py-1">
                                    取消
                                  </button>
                                </div>
                              ) : (
                                <button onClick={() => setConfirmId(item.id)}
                                  className="text-xs text-blue-500 hover:text-blue-700 font-medium">
                                  标记已报销
                                </button>
                              )}
                            </div>
                          )
                        })}
                        <div className="px-4 py-3 bg-gray-50 flex items-center justify-between">
                          <span className="text-xs font-semibold text-gray-500">合计（按实时汇率）</span>
                          <span className="font-bold text-red-600 tabular-nums whitespace-nowrap text-sm">−{fmt(cnyTotal(r), 'CNY')}</span>
                        </div>
                      </div>
                      {/* Desktop: table */}
                      <table className="hidden md:table w-full text-xs">
                        <thead>
                          <tr className="bg-gray-50 text-gray-400 uppercase tracking-wider border-b border-gray-100">
                            <th className="px-4 py-2.5 text-left font-semibold">ID</th>
                            <th className="px-4 py-2.5 text-left font-semibold">日期</th>
                            <th className="px-4 py-2.5 text-left font-semibold">类别</th>
                            <th className="px-4 py-2.5 text-left font-semibold">项目</th>
                            <th className="px-4 py-2.5 text-left font-semibold">备注</th>
                            <th className="px-4 py-2.5 text-right font-semibold">金额</th>
                            <th className="px-4 py-2.5 text-center font-semibold">报销</th>
                          </tr>
                        </thead>
                        <tbody>
                          {r.items.map((item, idx) => {
                            const done = reimbursedIds.has(item.id)
                            const confirming = confirmId === item.id
                            const busy = loadingId === item.id
                            return (
                              <tr key={item.id} className={`border-b border-gray-50 last:border-0 transition-colors ${done ? 'opacity-50 bg-green-50/30' : idx % 2 === 0 ? 'hover:bg-gray-50/60' : 'bg-gray-50/30 hover:bg-gray-50/60'}`}>
                                <td className="px-4 py-2.5 font-mono text-gray-400 bg-gray-50/50">{item.id.slice(0, 8)}…</td>
                                <td className={`px-4 py-2.5 tabular-nums whitespace-nowrap ${done ? 'text-gray-400 line-through' : 'text-gray-500'}`}>{item.occurred_at}</td>
                                <td className={`px-4 py-2.5 font-medium ${done ? 'text-gray-400 line-through' : 'text-gray-600'}`}>{item.category}</td>
                                <td className="px-4 py-2.5">
                                  {item.project_id
                                    ? <span className="font-mono bg-gray-100 text-gray-600 px-1.5 py-0.5 rounded">{item.project_id}</span>
                                    : <span className="text-gray-300">—</span>
                                  }
                                </td>
                                <td className="px-4 py-2.5 text-gray-400 max-w-[140px] truncate" title={item.note ?? undefined}>{item.note || '—'}</td>
                                <td className={`px-4 py-2.5 text-right font-bold tabular-nums whitespace-nowrap ${done ? 'text-gray-400 line-through' : 'text-red-500'}`}>−{fmt(item.amount_yuan, item.currency)}</td>
                                <td className="px-4 py-2.5 text-center">
                                  {done ? (
                                    <span className="inline-flex items-center gap-1 text-xs text-green-600 font-medium whitespace-nowrap">
                                      <svg className="w-3.5 h-3.5" fill="currentColor" viewBox="0 0 20 20"><path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" /></svg>
                                      已报销
                                    </span>
                                  ) : confirming ? (
                                    <div className="flex items-center justify-center gap-1.5">
                                      <button onClick={() => handleReimburse(item.id, item.uploaded)} disabled={busy}
                                        className="text-xs bg-green-600 hover:bg-green-700 disabled:opacity-50 text-white px-2 py-1 rounded-lg font-medium">
                                        {busy ? '…' : '确认'}
                                      </button>
                                      <button onClick={() => setConfirmId(null)}
                                        className="text-xs text-gray-400 hover:text-gray-600 px-1.5 py-1">
                                        取消
                                      </button>
                                    </div>
                                  ) : (
                                    <button onClick={() => setConfirmId(item.id)}
                                      className="text-xs text-blue-500 hover:text-blue-700 font-medium whitespace-nowrap">
                                      标记报销
                                    </button>
                                  )}
                                </td>
                              </tr>
                            )
                          })}
                        </tbody>
                        <tfoot>
                          <tr className="bg-gray-50 border-t border-gray-200">
                            <td colSpan={5} className="px-4 py-2.5 text-xs font-semibold text-gray-500">合计（按实时汇率）</td>
                            <td className="px-4 py-2.5 text-right font-bold text-red-600 tabular-nums whitespace-nowrap">−{fmt(cnyTotal(r), 'CNY')}</td>
                            <td />
                          </tr>
                        </tfoot>
                      </table>
                    </>
                  ) : (
                    <div className="px-5 py-4">
                      <p className="text-xs text-gray-500 font-semibold mb-2 uppercase tracking-wider">交易 ID 列表</p>
                      <div className="flex flex-wrap gap-1.5">
                        {r.ids.map((id) => (
                          <span key={id} className="font-mono text-xs bg-gray-100 text-gray-600 px-2 py-1 rounded-lg">
                            {id.slice(0, 8)}…
                          </span>
                        ))}
                      </div>
                    </div>
                  )}
                </div>
              )}
            </div>
          )
        })}
        </div>
      )}
    </div>
  )
}
