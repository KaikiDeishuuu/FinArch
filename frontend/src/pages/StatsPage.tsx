import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  PieChart, Pie, Cell, Tooltip, ResponsiveContainer,
} from 'recharts'
import { formatAmountCompact, formatAmount, formatAmountExact, toCNY } from '../utils/format'
import CompactAmount from '../components/CompactAmount'
import { useExchangeRates } from '../contexts/ExchangeRateContext'
import { useTransactions } from '../hooks/useTransactions'
import { useAccounts } from '../hooks/useAccounts'
import Select from '../components/Select'
import type { Account } from '../api/client'
import { StaggerContainer, StaggerItem } from '../motion'
import { categoryLabel } from '../utils/categoryLabel'

const PIE_COLORS = [
  '#8b5cf6','#f59e0b','#10b981','#ef4444','#06b6d4',
  '#f97316','#84cc16','#ec4899','#22c55e','#14b8a6',
  '#a855f7','#eab308','#0ea5e9',
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

  const { t } = useTranslation()
  const MONTH_LABELS = t('stats.monthLabels', { returnObjects: true }) as string[]
  const PAD_L = 72   // room for Y-axis labels
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
    <div ref={containerRef} className="relative w-full" style={{ height: SVG_H }}>
      <svg width={svgWidth} height={SVG_H}>
        {/* Grid lines + Y labels (drawn inside SVG bounds) */}
        {yTicks.map(v => {
          const y = yPx(v)
          return (
            <g key={v}>
              <line
                x1={PAD_L} y1={y} x2={PAD_L + chartW} y2={y}
                className={v === 0 ? 'stroke-gray-300 dark:stroke-gray-600' : 'stroke-gray-100 dark:stroke-gray-800/60'}
                strokeWidth={1}
              />
              <text x={PAD_L - 6} y={y} dominantBaseline="middle" textAnchor="end"
                fontSize={10} className="fill-gray-400 dark:fill-gray-400" fontFamily="inherit">
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
                  fill="#22c55e"
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
                fontSize={10} className="fill-gray-400 dark:fill-gray-400" fontFamily="inherit">
                {MONTH_LABELS[d.month - 1]}
              </text>
            </g>
          )
        })}
      </svg>

      {/* Floating tooltip */}
      {tooltip && (
        <div
          className="pointer-events-none absolute z-10 bg-white dark:bg-[hsl(260,15%,11%)] border border-gray-100 dark:border-gray-800/50 rounded-xl shadow-lg px-3 py-2.5 text-xs"
          style={{
            left: tooltip.x,
            top: Math.max(4, tooltip.y - 72),
            transform: 'translateX(-50%)',
            minWidth: 130,
          }}
        >
          <p className="font-semibold text-gray-700 dark:text-gray-300 mb-1.5 border-b border-gray-50 dark:border-gray-800 pb-1">{tooltip.label}</p>
          <div className="flex items-center justify-between gap-3">
            <span className="flex items-center gap-1.5 text-gray-500 dark:text-gray-400">
              <span className="w-2 h-2 rounded-sm inline-block flex-shrink-0" style={{ background: '#22c55e' }} />{t('stats.pie.incomeLabel')}
            </span>
            <span className="font-bold tabular-nums" style={{ color: '#22c55e' }}>{fmt(tooltip.income)}</span>
          </div>
          <div className="flex items-center justify-between gap-3 mt-1">
            <span className="flex items-center gap-1.5 text-gray-500 dark:text-gray-400">
              <span className="w-2 h-2 rounded-sm inline-block flex-shrink-0" style={{ background: '#f43f5e' }} />{t('stats.pie.expenseLabel')}
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
  const { data: accounts = [] } = useAccounts()
  const [sourceFilter, setSourceFilter] = useState<'all' | 'personal' | 'company'>('all')
  const [filterCategory, setFilterCategory] = useState('')
  const [filterProject, setFilterProject] = useState('')
  const [filterAccount, setFilterAccount] = useState('')
  const { t } = useTranslation()

  const activeAccounts = useMemo(() =>
    accounts.filter((a: Account) => a.is_active),
    [accounts]
  )

  // Filter accounts by selected source tab
  const filteredAccounts = useMemo(() => {
    if (sourceFilter === 'all') return activeAccounts
    const acctType = sourceFilter === 'company' ? 'public' : 'personal'
    return activeAccounts.filter((a: Account) => a.type === acctType)
  }, [activeAccounts, sourceFilter])

  const allCategories = useMemo(
    () => Array.from(new Set(transactions.map(t => t.category).filter(Boolean))).sort() as string[],
    [transactions]
  )

  const allProjects = useMemo(
    () => Array.from(new Set(transactions.map(t => t.project_id).filter(Boolean))).sort() as string[],
    [transactions]
  )

  const filteredBySource = useMemo(() =>
    (sourceFilter === 'all' ? transactions : transactions.filter(t => t.source === sourceFilter))
      .filter(t => !filterCategory || t.category === filterCategory)
      .filter(t => !filterProject || (t.project_id ?? '') === filterProject)
      .filter(t => !filterAccount || t.account_id === filterAccount),
    [transactions, sourceFilter, filterCategory, filterProject, filterAccount]
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
    const otherLabel = t('stats.other')
    const map = new Map<string, { total: number; count: number }>()
    for (const t of filteredBySource) {
      if (t.direction !== 'expense') continue
      const cat = t.category || otherLabel
      if (!map.has(cat)) map.set(cat, { total: 0, count: 0 })
      const entry = map.get(cat)!
      entry.total += toCNY(t.amount_yuan, t.currency || 'CNY', rates)
      entry.count++
    }
    return Array.from(map.entries())
      .map(([category, v]) => ({ category: categoryLabel(category), total: v.total, count: v.count }))
      .sort((a, b) => b.total - a.total)
  }, [filteredBySource, rates, t])

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
        <div className="w-8 h-8 border-4 border-violet-500 border-t-transparent rounded-full animate-spin" />
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 dark:text-gray-100 tracking-tight">{t('stats.title')}</h1>
          <p className="text-sm text-gray-400 dark:text-gray-500 mt-1">{t('stats.subtitle', { year })}</p>
        </div>
        {!ratesLoading && (
          rateDate
            ? <span className="shrink-0 text-[10px] px-2 py-1 rounded-full bg-emerald-50 dark:bg-emerald-900/30 text-emerald-600 dark:text-emerald-400 font-medium mt-1">{t('stats.rateLabel.live')} · $ {rates.USD?.toFixed(2)} · € {rates.EUR?.toFixed(2)} · {rateDate}</span>
            : <span className="shrink-0 text-[10px] px-2 py-1 rounded-full bg-amber-50 dark:bg-amber-900/30 text-amber-600 dark:text-amber-400 font-medium mt-1">{t('stats.rateLabel.fallback')} · $ {rates.USD?.toFixed(2)} · € {rates.EUR?.toFixed(2)}</span>
        )}
      </div>

      {/* Filter bar */}
      <div className="flex flex-wrap items-center gap-2">
        {/* Source filter tabs */}
        <div className="flex gap-1 bg-gray-100 dark:bg-gray-800 rounded-xl p-1 w-fit">
          {(['all', 'personal', 'company'] as const).map((key) => (
            <button
              key={key}
              onClick={() => { setSourceFilter(key); setFilterAccount('') }}
              className={`px-3 py-1 rounded-lg text-xs font-medium transition-colors ${
                sourceFilter === key
                  ? 'bg-white dark:bg-[hsl(260,15%,11%)] text-violet-700 shadow-sm'
                  : 'text-gray-500 hover:text-gray-700 dark:hover:text-gray-300'
              }`}
            >
              {t(`stats.sourceTabs.${key}`)}
            </button>
          ))}
        </div>

        {/* Account filter */}
        {filteredAccounts.length > 1 && (
          <div className="w-fit min-w-[6.5rem]">
            <Select
              value={filterAccount}
              onChange={setFilterAccount}
              placeholder={t('stats.filter.allAccounts')}
              size="sm"
              activeHighlight
              options={[
                { value: '', label: t('stats.filter.allAccounts') },
                ...filteredAccounts.map((a: Account) => ({ value: a.id, label: a.name })),
              ]}
            />
          </div>
        )}

        {/* Category filter */}
        {allCategories.length > 0 && (
          <div className="w-fit min-w-[6.5rem]">
            <Select
              value={filterCategory}
              onChange={setFilterCategory}
              placeholder={t('stats.filter.allCategories')}
              size="sm"
              activeHighlight
              options={[
                { value: '', label: t('stats.filter.allCategories') },
                ...allCategories.map(c => ({ value: c, label: categoryLabel(c) })),
              ]}
            />
          </div>
        )}

        {/* Project filter */}
        {allProjects.length > 0 && (
          <div className="w-fit min-w-[6.5rem]">
            <Select
              value={filterProject}
              onChange={setFilterProject}
              placeholder={t('stats.filter.allProjects')}
              size="sm"
              activeHighlight
              options={[
                { value: '', label: t('stats.filter.allProjects') },
                ...allProjects.map(p => ({ value: p, label: p })),
              ]}
            />
          </div>
        )}

        {/* Clear filters */}
        {(filterCategory || filterProject || filterAccount) && (
          <button
            onClick={() => { setFilterCategory(''); setFilterProject(''); setFilterAccount('') }}
            className="h-8 px-2.5 rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-800/50 text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-800 text-xs transition-all"
          >
            {t('stats.filter.clear')}
          </button>
        )}
      </div>

      {/* Summary cards — Premium: flat, clean */}
      <StaggerContainer className="grid grid-cols-3 gap-3">
        <StaggerItem>
        <div className="bg-white dark:bg-[hsl(260,15%,11%)] rounded-2xl border border-gray-100 dark:border-gray-800/50 p-3 md:p-5 overflow-hidden">
          <p className="text-[10px] md:text-[11px] text-gray-400 dark:text-gray-500 tracking-wide font-semibold truncate">{t('stats.yearlyIncome')}</p>
          <p className="text-base md:text-2xl font-bold text-indigo-600 dark:text-indigo-400 truncate tabular-nums mt-1.5">
            <CompactAmount compact={fmtShort(totalIncome)} exact={fmtExact(totalIncome)} />
          </p>
        </div>
        </StaggerItem>
        <StaggerItem>
        <div className="bg-white dark:bg-[hsl(260,15%,11%)] rounded-2xl border border-gray-100 dark:border-gray-800/50 p-3 md:p-5 overflow-hidden">
          <p className="text-[10px] md:text-[11px] text-gray-400 dark:text-gray-500 tracking-wide font-semibold truncate">{t('stats.yearlyExpense')}</p>
          <p className="text-base md:text-2xl font-bold text-rose-500 dark:text-rose-400 truncate tabular-nums mt-1.5">
            <CompactAmount compact={fmtShort(totalExpense)} exact={fmtExact(totalExpense)} />
          </p>
        </div>
        </StaggerItem>
        <StaggerItem>
        <div className="bg-white dark:bg-[hsl(260,15%,11%)] rounded-2xl border border-gray-100 dark:border-gray-800/50 p-3 md:p-5 overflow-hidden">
          <p className="text-[10px] md:text-[11px] text-gray-400 dark:text-gray-500 tracking-wide font-semibold truncate">{t('stats.yearlyNet')}</p>
          <p className={`text-base md:text-2xl font-bold truncate tabular-nums mt-1.5 ${totalNet >= 0 ? 'text-violet-600 dark:text-violet-400' : 'text-orange-500 dark:text-orange-400'}`}>
            <CompactAmount compact={fmtShort(totalNet)} exact={fmtExact(totalNet)} prefix={totalNet >= 0 ? '+' : ''} />
          </p>
        </div>
        </StaggerItem>
      </StaggerContainer>

      {/* Monthly bar chart — Premium */}
      <div className="bg-white dark:bg-[hsl(260,15%,11%)] rounded-2xl border border-gray-100/80 dark:border-gray-800/50 p-5 shadow-sm">
        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2 mb-5">
          <div>
            <h2 className="font-semibold text-gray-800 dark:text-gray-200">{t('stats.chart.monthlyTitle', { year })}</h2>
            <p className="text-xs text-gray-400 dark:text-gray-500 mt-0.5">{t('stats.chart.monthlySubtitle')}</p>
          </div>
          <div className="flex items-center gap-4 text-xs text-gray-400 dark:text-gray-500">
            <span className="flex items-center gap-1.5">
              <span className="w-3 h-2.5 rounded-sm inline-block" style={{ background: '#22c55e' }} />{t('stats.pie.incomeLabel')}
            </span>
            <span className="flex items-center gap-1.5">
              <span className="w-3 h-2.5 rounded-sm inline-block" style={{ background: '#f43f5e' }} />{t('stats.pie.expenseLabel')}
            </span>
          </div>
        </div>
        {monthly.length === 0 ? (
          <p className="text-sm text-gray-400 dark:text-gray-500 text-center py-10">{t('stats.noData')}</p>
        ) : (
          <MonthlyBarChart data={monthly} fmt={fmt} fmtShort={fmtShort} />
        )}
      </div>

      {/* Income vs expense overview pie */}
      {monthly.length > 0 && totalIncome + totalExpense > 0 && (
        <div className="bg-white dark:bg-[hsl(260,15%,11%)] rounded-2xl border border-gray-100/80 dark:border-gray-800/50 p-5 shadow-sm">
          <h2 className="font-semibold text-gray-800 dark:text-gray-200 mb-4">{t('stats.chart.pieTitle')}</h2>
          <div className="flex items-center gap-6">
            <div className="w-32 h-32 shrink-0">
              <ResponsiveContainer width="100%" height="100%">
                <PieChart>
                  <Pie
                    data={[
                      { name: t('stats.pie.incomeLabel'), value: totalIncome },
                      { name: t('stats.pie.expenseLabel'), value: totalExpense },
                    ]}
                    dataKey="value"
                    cx="50%" cy="50%"
                    innerRadius={38}
                    outerRadius={58}
                    paddingAngle={3}
                    strokeWidth={0}
                  >
                    <Cell fill="#22c55e" />
                    <Cell fill="#f43f5e" />
                  </Pie>
                  <Tooltip formatter={(value, name) => [fmt(value as number), name]} cursor={false}
                    contentStyle={{ borderRadius: '12px', border: '1px solid var(--tooltip-border, #e5e7eb)', boxShadow: '0 4px 12px rgba(0,0,0,0.08)', fontSize: '12px', background: 'var(--tooltip-bg, #fff)', color: 'var(--tooltip-text, #374151)' }}
                    itemStyle={{ color: 'var(--tooltip-text, #374151)' }}
                    labelStyle={{ color: 'var(--tooltip-text, #374151)' }}
                  />
                </PieChart>
              </ResponsiveContainer>
            </div>
            <div className="space-y-3 flex-1">
              <div>
                <div className="flex items-center justify-between text-sm mb-1">
                  <span className="flex items-center gap-2 text-gray-500 dark:text-gray-400 font-medium">
                    <span className="w-2.5 h-2.5 rounded-full shrink-0" style={{ background: '#22c55e' }} />{t('stats.pie.incomeLabel')}
                  </span>
                  <span className="font-bold tabular-nums" style={{ color: '#22c55e' }}>{fmt(totalIncome)}</span>
                </div>
                <div className="h-1.5 bg-gray-100 dark:bg-gray-700 rounded-full overflow-hidden">
                  <div className="h-full rounded-full" style={{ background: '#22c55e', width: `${totalIncome + totalExpense > 0 ? Math.round(totalIncome / (totalIncome + totalExpense) * 100) : 0}%` }} />
                </div>
              </div>
              <div>
                <div className="flex items-center justify-between text-sm mb-1">
                  <span className="flex items-center gap-2 text-gray-500 dark:text-gray-400 font-medium">
                    <span className="w-2.5 h-2.5 rounded-full shrink-0" style={{ background: '#f43f5e' }} />{t('stats.pie.expenseLabel')}
                  </span>
                  <span className="font-bold tabular-nums" style={{ color: '#f43f5e' }}>{fmt(totalExpense)}</span>
                </div>
                <div className="h-1.5 bg-gray-100 dark:bg-gray-700 rounded-full overflow-hidden">
                  <div className="h-full rounded-full" style={{ background: '#f43f5e', width: `${totalIncome + totalExpense > 0 ? Math.round(totalExpense / (totalIncome + totalExpense) * 100) : 0}%` }} />
                </div>
              </div>
              {totalReimbursed > 0 && (
                <div className="pt-1 border-t border-gray-100 dark:border-gray-800">
                  <div className="flex items-center justify-between text-xs text-gray-400 dark:text-gray-500">
                    <span>{t('stats.reimbursed')}</span>
                    <span className="font-semibold text-violet-500 tabular-nums whitespace-nowrap">+{fmt(totalReimbursed)}</span>
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>
      )}

      {/* Category pie chart — Premium */}
      <div className="bg-white dark:bg-[hsl(260,15%,11%)] rounded-2xl border border-gray-100/80 dark:border-gray-800/50 p-5 shadow-sm">
        <h2 className="font-semibold text-gray-800 dark:text-gray-200 mb-4">{t('stats.chart.categoryTitle')}</h2>
        {categories.length === 0 ? (
          <p className="text-sm text-gray-400 dark:text-gray-500 text-center py-8">{t('stats.noData')}</p>
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
                    strokeWidth={0}
                  >
                    {categories.map((_, i) => (
                      <Cell key={i} fill={PIE_COLORS[i % PIE_COLORS.length]} />
                    ))}
                  </Pie>
                  <Tooltip formatter={(value, name) => [fmt(value as number), name]} cursor={false}
                    contentStyle={{ borderRadius: '12px', border: '1px solid var(--tooltip-border, #e5e7eb)', boxShadow: '0 4px 12px rgba(0,0,0,0.08)', fontSize: '12px', background: 'var(--tooltip-bg, #fff)', color: 'var(--tooltip-text, #374151)' }}
                    itemStyle={{ color: 'var(--tooltip-text, #374151)' }}
                    labelStyle={{ color: 'var(--tooltip-text, #374151)' }}
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
                        <span className="text-sm font-medium text-gray-700 dark:text-gray-300 truncate">{categoryLabel(c.category)}</span>
                      </div>
                      <div className="text-right shrink-0">
                        <span className="text-sm font-bold text-gray-800 dark:text-gray-200 tabular-nums">{fmt(c.total)}</span>
                        <span className="text-xs text-gray-400 dark:text-gray-500 ml-1.5">{t('stats.transactionUnit', { count: c.count })}</span>
                      </div>
                    </div>
                  ))}
                </div>
                {categories.length > 6 && (
                  <div className="pointer-events-none absolute bottom-0 left-0 right-0 h-8 bg-gradient-to-t from-white dark:from-[hsl(260,15%,11%)] to-transparent rounded-b" />
                )}
              </div>
            </div>
          </div>
        )}
      </div>

      {/* Project breakdown — Premium */}
      <div className="bg-white dark:bg-[hsl(260,15%,11%)] rounded-2xl border border-gray-100/80 dark:border-gray-800/50 shadow-sm overflow-hidden">
        <div className="px-5 py-4 border-b border-gray-100 dark:border-gray-800/50 flex items-center justify-between">
          <h2 className="font-semibold text-gray-800 dark:text-gray-200">{t('stats.chart.projectTitle')}</h2>
          <span className="text-xs text-gray-400 dark:text-gray-500">{t('stats.projectCount', { count: projects.length })}</span>
        </div>
        {projects.length === 0 ? (
          <p className="text-sm text-gray-400 dark:text-gray-500 text-center py-8">{t('stats.noData')}</p>
        ) : (
          <div className="overflow-x-auto">
          <table className="w-full text-sm min-w-[320px]">
            <thead>
              <tr className="bg-gray-50/80 dark:bg-gray-800/50 text-gray-400 dark:text-gray-500 text-xs uppercase tracking-wide border-b border-gray-100 dark:border-gray-800/50">
                <th className="px-5 py-3 text-left font-semibold">{t('stats.project.name')}</th>
                <th className="px-5 py-3 text-right font-semibold">{t('stats.project.income')}</th>
                <th className="px-5 py-3 text-right font-semibold">{t('stats.project.expense')}</th>
                <th className="px-5 py-3 text-right font-semibold">{t('stats.project.net')}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-50 dark:divide-gray-800/50">
              {projects.map((p) => (
                <tr key={p.project_id} className="hover:bg-gray-50/60 dark:hover:bg-gray-800/30 transition-colors">
                  <td className="px-5 py-3.5">
                    <p className="font-medium text-gray-700 dark:text-gray-300">{p.project_id}</p>
                    {p.project_name && <p className="text-xs text-gray-400 dark:text-gray-500 mt-0.5">{p.project_name}</p>}
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
                      p.net >= 0 ? 'bg-indigo-50 dark:bg-indigo-900/30 text-indigo-700 dark:text-indigo-400' : 'bg-rose-50 dark:bg-rose-900/30 text-rose-600 dark:text-rose-400'
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
