import { useState } from 'react'
import type { FormEvent } from 'react'
import { matchSubsetSum } from '../api/client'
import type { MatchResult } from '../api/client'

export default function MatchPage() {
  const [target, setTarget] = useState('')
  const [tolerance, setTolerance] = useState('0.01')
  const [maxItems, setMaxItems] = useState('10')
  const [results, setResults] = useState<MatchResult[]>([])
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [searched, setSearched] = useState(false)
  const [expandedIdx, setExpandedIdx] = useState<number | null>(null)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError('')
    const t = parseFloat(target)
    if (isNaN(t) || t <= 0) { setError('请输入有效目标金额'); return }
    setLoading(true)
    setResults([])
    setSearched(false)
    setExpandedIdx(null)
    try {
      const res = await matchSubsetSum(t, parseFloat(tolerance), parseInt(maxItems))
      setResults(res || [])
      setSearched(true)
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      setError(msg || '匹配失败，请重试')
    } finally {
      setLoading(false)
    }
  }

  const fmt = (n: number) => `¥${n.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}`
  const inputClass = 'w-full border border-gray-200 rounded-xl px-3.5 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent bg-gray-50 transition-all hover:bg-white tabular-nums'
  const labelClass = 'block text-xs font-semibold text-gray-500 mb-2 uppercase tracking-wider'

  return (
    <div className="space-y-6 max-w-3xl">
      {/* Header */}
      <div>
        <h1 className="text-2xl font-bold text-gray-800">子集匹配</h1>
        <p className="text-sm text-gray-400 mt-1">自动搜索与报销总额精确匹配的交易组合</p>
      </div>

      {/* Form card */}
      <div className="bg-white rounded-2xl border border-gray-100 shadow-sm overflow-hidden">
        {/* Info bar */}
        <div className="bg-blue-50 border-b border-blue-100 px-5 py-3 flex items-start gap-2.5">
          <svg className="w-4 h-4 text-blue-500 shrink-0 mt-0.5" fill="currentColor" viewBox="0 0 20 20"><path fillRule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clipRule="evenodd" /></svg>
          <p className="text-sm text-blue-700">在<strong>已上传、未报销</strong>的个人垫付记录中，找出金额之和与目标最接近的组合</p>
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
          {/* Result header */}
          <div className="flex items-center justify-between px-1">
            {results.length === 0 ? (
              <div className="flex items-center gap-2 text-gray-500">
                <span className="text-xl">🔎</span>
                <span className="text-sm">未找到匹配方案，尝试调大误差范围</span>
              </div>
            ) : (
              <div className="flex items-center gap-2">
                <span className="text-xl">✅</span>
                <span className="text-sm text-gray-600">
                  共找到 <span className="text-blue-600 font-bold text-base">{results.length}</span> 个匹配方案
                </span>
              </div>
            )}
          </div>

          {results.map((r, i) => (
            <div key={i} className="bg-white rounded-2xl border border-gray-100 shadow-sm overflow-hidden">
              {/* Card header (clickable) */}
              <button
                className="w-full px-5 py-4 flex items-center justify-between hover:bg-gray-50/60 transition-colors"
                onClick={() => setExpandedIdx(expandedIdx === i ? null : i)}
              >
                <div className="flex items-center gap-4 text-left">
                  <span className="flex items-center justify-center w-8 h-8 rounded-xl bg-blue-600 text-white text-xs font-bold shrink-0 shadow-sm">
                    {i + 1}
                  </span>
                  <div>
                    <div className="flex items-center gap-3">
                      <p className="font-bold text-gray-800 text-base">{fmt(r.total)}</p>
                      {r.error <= 0.01 && (
                        <span className="text-xs bg-green-100 text-green-700 px-2 py-0.5 rounded-full font-medium">精确匹配</span>
                      )}
                    </div>
                    <p className="text-xs text-gray-400 mt-0.5 flex items-center gap-2">
                      <span>误差 {fmt(r.error)}</span>
                      <span className="w-1 h-1 rounded-full bg-gray-300" />
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
                        {r.items.map((item) => (
                          <div key={item.id} className="px-4 py-3 space-y-1.5">
                            <div className="flex items-center justify-between gap-2">
                              <span className="font-semibold text-gray-700 text-sm">{item.category}</span>
                              <span className="font-bold text-red-500 tabular-nums text-sm">−{fmt(item.amount_yuan)}</span>
                            </div>
                            <div className="flex items-center gap-2 flex-wrap text-xs text-gray-400">
                              <span className="tabular-nums">{item.occurred_at}</span>
                              {item.project_id && (
                                <span className="font-mono bg-gray-100 text-gray-600 px-1.5 py-0.5 rounded">{item.project_id}</span>
                              )}
                              {item.note && <span className="truncate max-w-[180px]">{item.note}</span>}
                            </div>
                          </div>
                        ))}
                        <div className="px-4 py-3 bg-gray-50 flex items-center justify-between">
                          <span className="text-xs font-semibold text-gray-500">合计</span>
                          <span className="font-bold text-red-600 tabular-nums text-sm">−{fmt(r.total)}</span>
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
                          </tr>
                        </thead>
                        <tbody>
                          {r.items.map((item, idx) => (
                            <tr key={item.id} className={`border-b border-gray-50 last:border-0 hover:bg-gray-50/60 transition-colors ${idx % 2 === 0 ? '' : 'bg-gray-50/30'}`}>
                              <td className="px-4 py-2.5 font-mono text-gray-400 bg-gray-50/50">{item.id.slice(0, 8)}…</td>
                              <td className="px-4 py-2.5 text-gray-500 tabular-nums whitespace-nowrap">{item.occurred_at}</td>
                              <td className="px-4 py-2.5 font-medium text-gray-600">{item.category}</td>
                              <td className="px-4 py-2.5">
                                {item.project_id
                                  ? <span className="font-mono bg-gray-100 text-gray-600 px-1.5 py-0.5 rounded">{item.project_id}</span>
                                  : <span className="text-gray-300">—</span>
                                }
                              </td>
                              <td className="px-4 py-2.5 text-gray-400 max-w-[140px] truncate" title={item.note ?? undefined}>{item.note || '—'}</td>
                              <td className="px-4 py-2.5 text-right font-bold text-red-500 tabular-nums">−{fmt(item.amount_yuan)}</td>
                            </tr>
                          ))}
                        </tbody>
                        <tfoot>
                          <tr className="bg-gray-50 border-t border-gray-200">
                            <td colSpan={5} className="px-4 py-2.5 text-xs font-semibold text-gray-500">合计</td>
                            <td className="px-4 py-2.5 text-right font-bold text-red-600 tabular-nums">−{fmt(r.total)}</td>
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
          ))}
        </div>
      )}
    </div>
  )
}
