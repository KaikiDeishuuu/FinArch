import { useEffect, useMemo, useRef, useState } from 'react'
import {
  PieChart, Pie, Cell, Tooltip, ResponsiveContainer,
} from 'recharts'
import { formatAmountCompact, formatAmount, formatAmountExact, toCNY } from '../utils/format'
import CompactAmount from '../components/CompactAmount'
import { useExchangeRates } from '../contexts/ExchangeRateContext'
import { useTransactions } from '../hooks/useTransactions'

const PIE_COLORS = [
  '#0d9488','#f59e0b','#10b981','#ef4444','#8b5cf6',
  '#06b6d4','#f97316','#84cc16','#ec4899','#6366f1',
  '#14b8a6','#a855f7','#eab308',
]

// ─── Custom SVG bar chart (avoids recharts BarChart cursor/overflow bugs) ─────

interface MonthData { month: number; income: number; expense: number }

function MonthlyBarChart({
  data,
  fmt,
  fmtShort,
}: {
  data: MonthData[]
  fmt: (n: number) => string
  fmtShort: (n: number) => string
}) {
  const containerRef = useRef<HTMLDivElement>(null)
  const [svgWidth, setSvgWidth] = useState(560)
  const [tooltip, setTooltip] = useState<{ x: number; y: number; income: number; expense: number; label: string } | null>(null)

  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    setSvgWidth(el.clientWidth)
    const ro = new ResizeObserver(entries => setSvgWidth(entries[0].contentRect.width))
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  const MONTH_LABELS = ['1月','2月','3月','4月','5月','6月','7月','8月','9月','10月','11月','12月']
  const PAD_L = 58   // room for Y-axis labels
  const PAD_R = 12
  const PAD_T = 12
  const PAD_B = 28   // room for X-axis labels
  const SVG_H = 212

  const chartW = svgWidth - PAD_L - PAD_R
  const chartH = SVG_H - PAD_T - PAD_B

  const maxVal = Math.max(...data.flatMap(d => [d.income, d.expense]), 1)
  // nice Y ticks
  const roughStep = maxVal / 4
  const mag = Math.pow(10, Math.floor(Math.log10(roughStep || 1)))
  const niceStep = Math.ceil(roughStep / mag) * mag || 1
  const yMax = niceStep * 4
  const yTicks = [0, niceStep, niceStep * 2, niceStep * 3, niceStep * 4]

  const yPx = (v: number) => PAD_T + chartH - (v / yMax) * chartH

  const groupW = chartW / data.length
  const barGutter = Math.max(groupW * 0.18, 3)
  const pairW = groupW - barGutter * 2
  const barW = Math.max((pairW / 2) - 2, 4)
  const CORNER = Math.min(barW / 2, 4)

  return (
    <div ref={containerRef} className="relative w-full" style={{ height: SVG_H, overflow: 'hidden' }}>
      <svg width={svgWidth} height={SVG_H}>
        {/* Grid lines + Y labels (drawn inside SVG bounds) */}
        {yTicks.map(v => {
          const y = yPx(v)
          return (
            <g key={v}>
              <line
                x1={PAD_L} y1={y} x2={PAD_L + chartW} y2={y}
                stroke={v === 0 ? '#d1d5db' : '#f3f4f6'} strokeWidth={1}
              />
              <text x={PAD_L - 6} y={y} dominantBaseline="middle" textAnchor="end"
                fontSize={10} fill="#9ca3af" fontFamily="inherit">
                {fmtShort(v)}
              </text>
            </g>
          )
        })}

        {/* Bars + X labels */}
        {data.map((d, i) => {
          const gx = PAD_L + i * groupW + barGutter
          const incX = gx
          const expX = gx + barW + 2
          const incH = (d.income / yMax) * chartH
          const expH = (d.expense / yMax) * chartH
          const incY = yPx(d.income)
          const expY = yPx(d.expense)
          const labelX = PAD_L + (i + 0.5) * groupW
          const tooltipX = Math.max(PAD_L + 60, Math.min(labelX, svgWidth - 60))

          return (
            <g key={d.month}
              onMouseEnter={() => setTooltip({ x: tooltipX, y: Math.min(incY, expY), income: d.income, expense: d.expense, label: MONTH_LABELS[d.month - 1] })}
              onMouseLeave={() => setTooltip(null)}
              style={{ cursor: 'default' }}
            >
              {/* Hover highlight */}
              <rect
                x={PAD_L + i * groupW} y={PAD_T} width={groupW} height={chartH}
                fill="transparent"
                onMouseEnter={() => setTooltip({ x: tooltipX, y: Math.min(incY, expY), income: d.income, expense: d.expense, label: MONTH_LABELS[d.month - 1] })}
              />
              {/* Income bar */}
              {d.income > 0 && (
                <path
                  d={`M${incX + CORNER},${incY} h${barW - CORNER * 2} a${CORNER},${CORNER} 0 0 1 ${CORNER},${CORNER} v${incH - CORNER} h${-barW} v${-(incH - CORNER)} a${CORNER},${CORNER} 0 0 1 ${CORNER},${-CORNER}z`}
                  fill="#6366f1"
                />
              )}
              {/* Expense bar */}
              {d.expense > 0 && (
                <path
                  d={`M${expX + CORNER},${expY} h${barW - CORNER * 2} a${CORNER},${CORNER} 0 0 1 ${CORNER},${CORNER} v${expH - CORNER} h${-barW} v${-(expH - CORNER)} a${CORNER},${CORNER} 0 0 1 ${CORNER},${-CORNER}z`}
                  fill="#f43f5e"
                />
              )}
              {/* X label */}
              <text x={labelX} y={PAD_T + chartH + 18} textAnchor="middle"
                fontSize={10} fill="#9ca3af" fontFamily="inherit">
                {MONTH_LABELS[d.month - 1]}
              </text>
            </g>
          )
        })}
      </svg>

      {/* Floating tooltip */}
      {tooltip && (
        <div
          className="pointer-events-none absolute z-10 bg-white border border-gray-100 rounded-xl shadow-lg px-3 py-2.5 text-xs"
          style={{
            left: tooltip.x,
            top: Math.max(4, tooltip.y - 72),
            transform: 'translateX(-50%)',
            minWidth: 130,
          }}
        >
          <p className="font-semibold text-gray-700 mb-1.5 border-b border-gray-50 pb-1">{tooltip.label}</p>
          <div className="flex items-center justify-between gap-3">
            <span className="flex items-center gap-1.5 text-gray-500">
              <span className="w-2 h-2 rounded-sm inline-block flex-shrink-0" style={{ background: '#6366f1' }} />收入
            </span>
            <span className="font-bold tabular-nums" style={{ color: '#6366f1' }}>{fmt(tooltip.income)}</span>
          </div>
          <div className="flex items-center justify-between gap-3 mt-1">
            <span className="flex items-center gap-1.5 text-gray-500">
              <span className="w-2 h-2 rounded-sm inline-block flex-shrink-0" style={{ background: '#f43f5e' }} />支出
            </span>
            <span className="font-bold tabular-nums" style={{ color: '#f43f5e' }}>{fmt(tooltip.expense)}</span>
          </div>
        </div>
      )}
    </div>
  )
}

export default function StatsPage() {
  const year = new Date().getFullYear()
  const { data: transactions = [], isLoading: loading } = useTransactions()
  const { rates, rateDate, loading: ratesLoading } = useExchangeRates()
  const [sourceFilter, setSourceFilter] = useState<'all' | 'personal' | 'company'>('all')

  const filteredBySource = useMemo(() =>
    sourceFilter === 'all' ? transactions : transactions.filter(t => t.source === sourceFilter),
    [transactions, sourceFilter]
  )

  const fmt = (n: number) => formatAmount(n, 'CNY')
  const fmtExact = (n: number) => formatAmountExact(n, 'CNY')
  const fmtShort = (n: number) => formatAmountCompact(n, 'CNY')

  // Compute monthly stats for current year
  const monthly = useMemo(() => {
    const map = new Map<number, { month: number; income: number; expense: number; reimbursed: number }>()
    for (const t of filteredBySource) {
      if (!t.occurred_at.startsWith(String(year))) continue
      const month = parseInt(t.occurred_at.substring(5, 7))
      if (!map.has(month)) map.set(month, { month, income: 0, expense: 0, reimbursed: 0 })
      const entry = map.get(month)!
      const cny = toCNY(t.amount_yuan, t.currency || 'CNY', rates)
      if (t.direction === 'income') {
        entry.income += cny
      } else {
        entry.expense += cny
        if (t.reimbursed) entry.reimbursed += cny
      }
    }
    return Array.from(map.values()).sort((a, b) => a.month - b.month)
  }, [filteredBySource, rates, year])

  // Compute category stats (all-time expense)
  const categories = useMemo(() => {
    const map = new Map<string, { total: number; count: number }>()
    for (const t of filteredBySource) {
      if (t.direction !== 'expense') continue
      const cat = t.category || '其他'
      if (!map.has(cat)) map.set(cat, { total: 0, count: 0 })
      const entry = map.get(cat)!
      entry.total += toCNY(t.amount_yuan, t.currency || 'CNY', rates)
      entry.count++
    }
    return Array.from(map.entries())
      .map(([category, v]) => ({ category, total: v.total, count: v.count }))
      .sort((a, b) => b.total - a.total)
  }, [filteredBySource, rates])

  // Compute project stats (all-time)
  const projects = useMemo(() => {
    const map = new Map<string, { project_name: string; income: number; expense: number }>()
    for (const t of filteredBySource) {
      if (!t.project_id) continue
      if (!map.has(t.project_id)) map.set(t.project_id, { project_name: t.project_id, income: 0, expense: 0 })
      const entry = map.get(t.project_id)!
      const cny = toCNY(t.amount_yuan, t.currency || 'CNY', rates)
      if (t.direction === 'income') entry.income += cny
      else entry.expense += cny
    }
    return Array.from(map.entries())
      .map(([project_id, v]) => ({ project_id, project_name: v.project_name, income: v.income, expense: v.expense, net: v.income - v.expense }))
      .sort((a, b) => a.project_id.localeCompare(b.project_id))
  }, [filteredBySource, rates])

  const totalIncome = monthly.reduce((s, m) => s + m.income, 0)
  const totalExpense = monthly.reduce((s, m) => s + m.expense, 0)
  const totalReimbursed = monthly.reduce((s, m) => s + (m.reimbursed ?? 0), 0)
  const totalNet = totalIncome - totalExpense + totalReimbursed

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="w-8 h-8 border-4 border-teal-500 border-t-transparent rounded-full animate-spin" />
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 tracking-tight">统计分析</h1>
          <p className="text-sm text-gray-400 mt-1">{year} 年度资金概览</p>
        </div>
        {!ratesLoading && (
          rateDate
            ? <span className="shrink-0 text-[10px] px-2 py-1 rounded-full bg-emerald-50 text-emerald-600 font-medium mt-1">实时汇率 · $ {rates.USD?.toFixed(2)} · € {rates.EUR?.toFixed(2)} · {rateDate}</span>
            : <span className="shrink-0 text-[10px] px-2 py-1 rounded-full bg-amber-50 text-amber-600 font-medium mt-1">备用汇率 · $ {rates.USD?.toFixed(2)} · € {rates.EUR?.toFixed(2)}</span>
        )}
      </div>

      {/* Source filter tabs */}
      <div className="flex gap-1 bg-gray-100 rounded-xl p-1 w-fit">
        {([['all', '全部'], ['personal', '个人账户'], ['company', '公司账户']] as const).map(([key, label]) => (
          <button
            key={key}
            onClick={() => setSourceFilter(key)}
            className={`px-3 py-1.5 rounded-lg text-sm font-medium transition-colors ${
              sourceFilter === key
                ? 'bg-white text-teal-700 shadow-sm'
                : 'text-gray-500 hover:text-gray-700'
            }`}
          >
            {label}
          </button>
        ))}
      </div>

      {/* Summary cards — Wise-style: flat, clean */}
      <div className="grid grid-cols-3 gap-3">
        <div className="bg-white rounded-2xl border border-gray-100 p-3 md:p-5 overflow-hidden">
          <p className="text-[10px] md:text-[11px] text-gray-400 uppercase tracking-wider font-semibold">年度收入</p>
          <p className="text-base md:text-2xl font-bold text-indigo-600 truncate tabular-nums mt-1.5">
            <CompactAmount compact={fmtShort(totalIncome)} exact={fmtExact(totalIncome)} />
          </p>
        </div>
        <div className="bg-white rounded-2xl border border-gray-100 p-3 md:p-5 overflow-hidden">
          <p className="text-[10px] md:text-[11px] text-gray-400 uppercase tracking-wider font-semibold">年度支出</p>
          <p className="text-base md:text-2xl font-bold text-rose-500 truncate tabular-nums mt-1.5">
            <CompactAmount compact={fmtShort(totalExpense)} exact={fmtExact(totalExpense)} />
          </p>
        </div>
        <div className="bg-white rounded-2xl border border-gray-100 p-3 md:p-5 overflow-hidden">
          <p className="text-[10px] md:text-[11px] text-gray-400 uppercase tracking-wider font-semibold">净结余</p>
          <p className={`text-base md:text-2xl font-bold truncate tabular-nums mt-1.5 ${totalNet >= 0 ? 'text-teal-600' : 'text-orange-500'}`}>
            <CompactAmount compact={fmtShort(totalNet)} exact={fmtExact(totalNet)} prefix={totalNet >= 0 ? '+' : ''} />
          </p>
        </div>
      </div>

      {/* Monthly bar chart — Wise-style */}
      <div className="bg-white rounded-2xl border border-gray-100 p-5">
        <div className="flex items-center justify-between mb-5">
          <div>
            <h2 className="font-semibold text-gray-800">{year} 年月度收支</h2>
            <p className="text-xs text-gray-400 mt-0.5">按月统计收入与支出</p>
          </div>
          <div className="flex items-center gap-4 text-xs text-gray-400">
            <span className="flex items-center gap-1.5">
              <span className="w-3 h-2.5 rounded-sm inline-block" style={{ background: '#6366f1' }} />收入
            </span>
            <span className="flex items-center gap-1.5">
              <span className="w-3 h-2.5 rounded-sm inline-block" style={{ background: '#f43f5e' }} />支出
            </span>
          </div>
        </div>
        {monthly.length === 0 ? (
          <p className="text-sm text-gray-400 text-center py-10">暂无数据</p>
        ) : (
          <MonthlyBarChart data={monthly} fmt={fmt} fmtShort={fmtShort} />
        )}
      </div>

      {/* Income vs expense overview pie */}
      {monthly.length > 0 && totalIncome + totalExpense > 0 && (
        <div className="bg-white rounded-2xl border border-gray-100 p-5">
          <h2 className="font-semibold text-gray-800 mb-4">收支比例</h2>
          <div className="flex items-center gap-6">
            <div className="w-32 h-32 shrink-0">
              <ResponsiveContainer width="100%" height="100%">
                <PieChart>
                  <Pie
                    data={[
                      { name: '收入', value: totalIncome },
                      { name: '支出', value: totalExpense },
                    ]}
                    dataKey="value"
                    cx="50%" cy="50%"
                    innerRadius={38}
                    outerRadius={58}
                    paddingAngle={3}
                    strokeWidth={0}
                  >
                    <Cell fill="#6366f1" />
                    <Cell fill="#f43f5e" />
                  </Pie>
                  <Tooltip formatter={(value, name) => [fmt(value as number), name]} cursor={false}
                    contentStyle={{ borderRadius: '12px', border: '1px solid #e5e7eb', boxShadow: '0 4px 12px rgba(0,0,0,0.08)', fontSize: '12px' }}
                  />
                </PieChart>
              </ResponsiveContainer>
            </div>
            <div className="space-y-3 flex-1">
              <div>
                <div className="flex items-center justify-between text-sm mb-1">
                  <span className="flex items-center gap-2 text-gray-500 font-medium">
                    <span className="w-2.5 h-2.5 rounded-full shrink-0" style={{ background: '#6366f1' }} />收入
                  </span>
                  <span className="font-bold tabular-nums" style={{ color: '#6366f1' }}>{fmt(totalIncome)}</span>
                </div>
                <div className="h-1.5 bg-gray-100 rounded-full overflow-hidden">
                  <div className="h-full rounded-full" style={{ background: '#6366f1', width: `${totalIncome + totalExpense > 0 ? Math.round(totalIncome / (totalIncome + totalExpense) * 100) : 0}%` }} />
                </div>
              </div>
              <div>
                <div className="flex items-center justify-between text-sm mb-1">
                  <span className="flex items-center gap-2 text-gray-500 font-medium">
                    <span className="w-2.5 h-2.5 rounded-full shrink-0" style={{ background: '#f43f5e' }} />支出
                  </span>
                  <span className="font-bold tabular-nums" style={{ color: '#f43f5e' }}>{fmt(totalExpense)}</span>
                </div>
                <div className="h-1.5 bg-gray-100 rounded-full overflow-hidden">
                  <div className="h-full rounded-full" style={{ background: '#f43f5e', width: `${totalIncome + totalExpense > 0 ? Math.round(totalExpense / (totalIncome + totalExpense) * 100) : 0}%` }} />
                </div>
              </div>
              {totalReimbursed > 0 && (
                <div className="pt-1 border-t border-gray-100">
                  <div className="flex items-center justify-between text-xs text-gray-400">
                    <span>已报销抵扣</span>
                    <span className="font-semibold text-violet-500 tabular-nums whitespace-nowrap">+{fmt(totalReimbursed)}</span>
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>
      )}

      {/* Category pie chart — Wise-style */}
      <div className="bg-white rounded-2xl border border-gray-100 p-5">
        <h2 className="font-semibold text-gray-800 mb-4">分类支出</h2>
        {categories.length === 0 ? (
          <p className="text-sm text-gray-400 text-center py-8">暂无数据</p>
        ) : (
          <div className="flex flex-col md:flex-row gap-6 items-start">
            <div className="w-full md:w-64 h-44 md:h-56 shrink-0">
              <ResponsiveContainer width="100%" height="100%">
                <PieChart>
                  <Pie
                    data={categories}
                    dataKey="total"
                    nameKey="category"
                    cx="50%" cy="50%"
                    innerRadius={52}
                    outerRadius={82}
                    paddingAngle={2}
                  >
                    {categories.map((_, i) => (
                      <Cell key={i} fill={PIE_COLORS[i % PIE_COLORS.length]} />
                    ))}
                  </Pie>
                  <Tooltip formatter={(value, name) => [fmt(value as number), name]} cursor={false}
                    contentStyle={{ borderRadius: '12px', border: '1px solid #e5e7eb', boxShadow: '0 4px 12px rgba(0,0,0,0.08)', fontSize: '12px' }}
                  />
                </PieChart>
              </ResponsiveContainer>
            </div>
            <div className="flex-1 w-full">
              <div className="relative">
                <div className="space-y-3 max-h-56 overflow-y-auto pr-1">
                  {categories.map((c, idx) => (
                    <div key={c.category} className="flex items-center justify-between gap-3">
                      <div className="flex items-center gap-2 min-w-0">
                        <span className="w-3 h-3 rounded-full shrink-0" style={{ background: PIE_COLORS[idx % PIE_COLORS.length] }} />
                        <span className="text-sm font-medium text-gray-700 truncate">{c.category}</span>
                      </div>
                      <div className="text-right shrink-0">
                        <span className="text-sm font-bold text-gray-800 tabular-nums">{fmt(c.total)}</span>
                        <span className="text-xs text-gray-400 ml-1.5">{c.count} 笔</span>
                      </div>
                    </div>
                  ))}
                </div>
                {categories.length > 6 && (
                  <div className="pointer-events-none absolute bottom-0 left-0 right-0 h-8 bg-gradient-to-t from-white to-transparent rounded-b" />
                )}
              </div>
            </div>
          </div>
        )}
      </div>

      {/* Project breakdown — Wise-style */}
      <div className="bg-white rounded-2xl border border-gray-100 overflow-hidden">
        <div className="px-5 py-4 border-b border-gray-100 flex items-center justify-between">
          <h2 className="font-semibold text-gray-800">项目汇总</h2>
          <span className="text-xs text-gray-400">{projects.length} 个项目</span>
        </div>
        {projects.length === 0 ? (
          <p className="text-sm text-gray-400 text-center py-8">暂无数据</p>
        ) : (
          <div className="overflow-x-auto">
          <table className="w-full text-sm min-w-[320px]">
            <thead>
              <tr className="bg-gray-50/80 text-gray-400 text-xs uppercase tracking-wide border-b border-gray-100">
                <th className="px-5 py-3 text-left font-semibold">项目</th>
                <th className="px-5 py-3 text-right font-semibold">收入</th>
                <th className="px-5 py-3 text-right font-semibold">支出</th>
                <th className="px-5 py-3 text-right font-semibold">结余</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-50">
              {projects.map((p) => (
                <tr key={p.project_id} className="hover:bg-gray-50/60 transition-colors">
                  <td className="px-5 py-3.5">
                    <p className="font-medium text-gray-700">{p.project_id}</p>
                    {p.project_name && <p className="text-xs text-gray-400 mt-0.5">{p.project_name}</p>}
                  </td>
                  <td className="px-5 py-3.5 text-right">
                    <span className="text-indigo-600 font-medium tabular-nums whitespace-nowrap">
                      <CompactAmount compact={fmtShort(p.income)} exact={fmtExact(p.income)} />
                    </span>
                  </td>
                  <td className="px-5 py-3.5 text-right">
                    <span className="text-rose-500 font-medium tabular-nums whitespace-nowrap">
                      <CompactAmount compact={fmtShort(p.expense)} exact={fmtExact(p.expense)} />
                    </span>
                  </td>
                  <td className="px-5 py-3.5 text-right">
                    <span className={`font-bold tabular-nums whitespace-nowrap px-2 py-0.5 rounded-lg text-xs ${
                      p.net >= 0 ? 'bg-indigo-50 text-indigo-700' : 'bg-rose-50 text-rose-600'
                    }`}>
                      <CompactAmount compact={fmtShort(p.net)} exact={fmtExact(p.net)} prefix={p.net >= 0 ? '+' : ''} />
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          </div>
        )}
      </div>
    </div>
  )
}
