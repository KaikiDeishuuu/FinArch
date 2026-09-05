import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  PieChart, Pie, Cell, Tooltip, ResponsiveContainer,
} from 'recharts'
import { formatAmountCompact, formatAmount, formatAmountExact } from '../utils/format'
import { transactionAmountToCNY } from '../utils/financeAmounts'
import CompactAmount from '../components/CompactAmount'
import { useExchangeRates } from '../hooks/useExchangeRates'
import { useTransactions } from '../hooks/useTransactions'
import { useAccounts } from '../hooks/useAccounts'
import Select from '../components/Select'
import type { Account } from '../api/client'
import { StaggerContainer, StaggerItem } from '../motion'
import { categoryLabel } from '../utils/categoryLabel'
import { useMode } from '../hooks/useMode'
import ResponsivePieCard from '../components/ResponsivePieCard'
import { calculateWorkModeAdjustments } from '../utils/workModeStats'
import { getModeChartPalette } from '../utils/chartPalette'
import { accountModeForTransactionSource } from '../utils/accountScope'
import AccountBalanceChart from '../components/AccountBalanceChart'


// ─── Custom SVG bar chart (avoids recharts BarChart cursor/overflow bugs) ─────

interface MonthData { month: number; income: number; expense: number }

function MonthlyBarChart({
  data,
  fmt,
  fmtShort,
  incomeColor,
  expenseColor,
}: {
  data: MonthData[]
  fmt: (n: number) => string
  fmtShort: (n: number) => string
  incomeColor: string
  expenseColor: string
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
                  fill={incomeColor}
                />
              )}
              {/* Expense bar */}
              {d.expense > 0 && (
                <path
                  d={`M${expX + CORNER},${expY} h${barW - CORNER * 2} a${CORNER},${CORNER} 0 0 1 ${CORNER},${CORNER} v${expH - CORNER} h${-barW} v${-(expH - CORNER)} a${CORNER},${CORNER} 0 0 1 ${CORNER},${-CORNER}z`}
                  fill={expenseColor}
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
          className="pointer-events-none absolute z-10 rounded-[3px] border border-[var(--tooltip-border)] bg-[var(--tooltip-bg)] px-3 py-2.5 font-data text-xs text-[var(--tooltip-text)] shadow-[0_8px_20px_rgba(15,31,23,0.10)]"
          style={{
            left: tooltip.x,
            top: Math.max(4, tooltip.y - 72),
            transform: 'translateX(-50%)',
            minWidth: 130,
          }}
        >
          <p className="mb-1.5 border-b border-[var(--tooltip-border)] pb-1 font-semibold text-[var(--tooltip-text)]">{tooltip.label}</p>
          <div className="flex items-center justify-between gap-3">
            <span className="flex items-center gap-1.5 text-[hsl(var(--muted-foreground))]">
              <span className="w-2 h-2 rounded-sm inline-block flex-shrink-0" style={{ background: incomeColor }} />{t('stats.pie.incomeLabel')}
            </span>
            <span className="font-data font-semibold tabular-nums" style={{ color: incomeColor }}>{fmt(tooltip.income)}</span>
          </div>
          <div className="flex items-center justify-between gap-3 mt-1">
            <span className="flex items-center gap-1.5 text-[hsl(var(--muted-foreground))]">
              <span className="w-2 h-2 rounded-sm inline-block flex-shrink-0" style={{ background: expenseColor }} />{t('stats.pie.expenseLabel')}
            </span>
            <span className="font-data font-semibold tabular-nums" style={{ color: expenseColor }}>{fmt(tooltip.expense)}</span>
          </div>
        </div>
      )}
    </div>
  )
}


// CategoryPieCard replaced by ResponsivePieCard component

export default function StatsPage() {
  const year = new Date().getFullYear()
  const { data: transactions = [], isLoading: loading, isError, refetch, isFetching } = useTransactions()
  const { rates, rateDate, loading: ratesLoading } = useExchangeRates()
  const { t } = useTranslation()
  const { isWorkMode, mode } = useMode()
  const palette = getModeChartPalette(mode)
  const [workSourceFilter, setWorkSourceFilter] = useState<'personal' | 'company'>('company')
  const sourceFilter: 'personal' | 'company' = isWorkMode ? workSourceFilter : 'personal'
  const accountLookupMode = accountModeForTransactionSource(sourceFilter)
  const { data: accounts = [] } = useAccounts(accountLookupMode)
  const [filterCategory, setFilterCategory] = useState('')
  const [filterProject, setFilterProject] = useState('')
  const [filterAccount, setFilterAccount] = useState('')

  function selectSource(source: 'personal' | 'company') {
    if (!isWorkMode || source === sourceFilter) return
    setWorkSourceFilter(source)
    setFilterCategory('')
    setFilterProject('')
    setFilterAccount('')
  }

  const activeAccounts = useMemo(() =>
    accounts.filter((a: Account) => a.is_active),
    [accounts]
  )

  // Filter accounts by selected source tab
  const filteredAccounts = useMemo(() => {
    const acctType = sourceFilter === 'company' ? 'public' : 'personal'
    return activeAccounts.filter((a: Account) => a.type === acctType)
  }, [activeAccounts, sourceFilter])

  const allCategories = useMemo(
    () => Array.from(new Set(transactions.filter(t => t.source === sourceFilter).map(t => t.category).filter(Boolean))).sort() as string[],
    [transactions, sourceFilter]
  )

  const allProjects = useMemo(
    () => Array.from(new Set(transactions.filter(t => t.source === sourceFilter).map(t => t.project_id).filter(Boolean))).sort() as string[],
    [transactions, sourceFilter]
  )

  // A global mode change can replace the visible source without going through
  // selectSource. Ignore stale selections that do not belong to that source so
  // hidden filters cannot make the new view appear empty.
  const effectiveFilterCategory = allCategories.includes(filterCategory) ? filterCategory : ''
  const effectiveFilterProject = allProjects.includes(filterProject) ? filterProject : ''
  const effectiveFilterAccount = filteredAccounts.some(account => account.id === filterAccount) ? filterAccount : ''

  const filteredBySource = useMemo(() =>
    transactions.filter(t => t.source === sourceFilter)
      .filter(t => !effectiveFilterCategory || t.category === effectiveFilterCategory)
      .filter(t => !effectiveFilterProject || (t.project_id ?? '') === effectiveFilterProject)
      .filter(t => !effectiveFilterAccount || t.account_id === effectiveFilterAccount),
    [transactions, sourceFilter, effectiveFilterCategory, effectiveFilterProject, effectiveFilterAccount]
  )

  const fmt = (n: number) => formatAmount(n, 'CNY')
  const fmtExact = (n: number) => formatAmountExact(n, 'CNY')
  const fmtShort = (n: number) => formatAmountCompact(n, 'CNY')

  const statsData = useMemo(() => {
    const otherLabel = t('stats.other')
    const monthlyMap = new Map<number, { month: number; income: number; expense: number; reimbursed: number }>()
    const expenseCategoryMap = new Map<string, { total: number; count: number }>()
    const incomeCategoryMap = new Map<string, { total: number; count: number }>()
    const projectMap = new Map<string, { project_name: string; income: number; expense: number }>()

    for (const tx of filteredBySource) {
      const cny = transactionAmountToCNY(tx, rates)
      if (tx.occurred_at.startsWith(String(year))) {
        const month = parseInt(tx.occurred_at.substring(5, 7))
        if (!monthlyMap.has(month)) monthlyMap.set(month, { month, income: 0, expense: 0, reimbursed: 0 })
        const entry = monthlyMap.get(month)!
        if (tx.direction === 'income') {
          entry.income += cny
        } else {
          entry.expense += cny
          if (tx.reimbursed) entry.reimbursed += cny
        }
      }

      const category = tx.category || otherLabel
      const categoryMap = tx.direction === 'income' ? incomeCategoryMap : expenseCategoryMap
      if (!categoryMap.has(category)) categoryMap.set(category, { total: 0, count: 0 })
      const categoryEntry = categoryMap.get(category)!
      categoryEntry.total += cny
      categoryEntry.count++

      if (tx.project_id) {
        if (!projectMap.has(tx.project_id)) projectMap.set(tx.project_id, { project_name: tx.project_id, income: 0, expense: 0 })
        const projectEntry = projectMap.get(tx.project_id)!
        if (tx.direction === 'income') projectEntry.income += cny
        else projectEntry.expense += cny
      }
    }

    const mapCategories = (map: Map<string, { total: number; count: number }>) => Array.from(map.entries())
      .map(([category, value]) => ({ category: categoryLabel(category), total: value.total, count: value.count }))
      .sort((a, b) => b.total - a.total)

    return {
      monthly: Array.from(monthlyMap.values()).sort((a, b) => a.month - b.month),
      categories: mapCategories(expenseCategoryMap),
      incomeCategories: mapCategories(incomeCategoryMap),
      projects: Array.from(projectMap.entries())
        .map(([project_id, value]) => ({ project_id, project_name: value.project_name, income: value.income, expense: value.expense, net: value.income - value.expense }))
        .sort((a, b) => a.project_id.localeCompare(b.project_id)),
    }
  }, [filteredBySource, rates, t, year])

  const { monthly, categories, incomeCategories, projects } = statsData

  const totalIncome = monthly.reduce((s, m) => s + m.income, 0)
  const totalExpense = monthly.reduce((s, m) => s + m.expense, 0)

  // Reimbursement adjustments — WORK mode only
  const { totalReimbursed, adjustedNet: totalNet } = isWorkMode && sourceFilter === 'personal'
    ? calculateWorkModeAdjustments(monthly, totalIncome, totalExpense)
    : { totalReimbursed: 0, adjustedNet: totalIncome - totalExpense }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="h-8 w-8 animate-spin rounded-full border-2 border-[hsl(var(--border))] border-t-[hsl(var(--primary))]" />
      </div>
    )
  }

  if (isError) {
    return (
      <div className="ledger-panel flex h-64 flex-col items-center justify-center gap-3" role="alert">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5} className="w-12 h-12 text-rose-300 dark:text-rose-500/70"><path strokeLinecap="round" strokeLinejoin="round" d="M12 9v4m0 4h.01M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0Z" /></svg>
        <div className="text-center">
          <p className="text-sm font-semibold text-gray-700 dark:text-gray-200">{t('stats.error.title')}</p>
          <p className="text-xs text-gray-400 dark:text-gray-500 mt-1">{t('stats.error.desc')}</p>
        </div>
        <button
          type="button"
          onClick={() => refetch()}
          disabled={isFetching}
          className="rounded-[7px] bg-[hsl(var(--primary))] px-4 py-2 text-sm font-semibold text-[hsl(var(--primary-foreground))] transition-colors hover:brightness-95 disabled:opacity-50"
        >
          {isFetching ? t('common.loading') : t('common.retry')}
        </button>
      </div>
    )
  }

  return (
    <div className="space-y-5 md:space-y-6">
      <div className="ledger-rail flex flex-col gap-3 pl-5 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="font-display text-2xl font-semibold tracking-[-0.025em] text-[hsl(var(--foreground))] md:text-[1.75rem]">{t('stats.title')}</h1>
          <p className="mt-1 text-sm text-[hsl(var(--muted-foreground))]">{t('stats.subtitle', { year })}</p>
        </div>
        {!ratesLoading && (
          rateDate
            ? <span className="w-fit max-w-full text-[10px] px-2 py-1 rounded-full bg-emerald-50 dark:bg-emerald-900/30 text-emerald-600 dark:text-emerald-400 font-medium sm:mt-1 truncate">{t('stats.rateLabel.live')} · $ {rates.USD?.toFixed(2)} · € {rates.EUR?.toFixed(2)} · {rateDate}</span>
            : <span className="w-fit max-w-full text-[10px] px-2 py-1 rounded-full bg-amber-50 dark:bg-amber-900/30 text-amber-600 dark:text-amber-400 font-medium sm:mt-1 truncate">{t('stats.rateLabel.fallback')} · $ {rates.USD?.toFixed(2)} · € {rates.EUR?.toFixed(2)}</span>
        )}
      </div>

      {/* Filter bar */}
      <div className="ledger-panel flex flex-wrap items-center gap-2 p-3 md:w-fit">
        {/* Source filter */}
        {isWorkMode ? (
          <div role="group" aria-label={t('transactions.sourceFilterLabel')} className="inline-flex h-8 items-center gap-0.5 rounded-[7px] border border-[hsl(var(--border))] bg-[hsl(var(--muted))] p-0.5">
            {(['company', 'personal'] as const).map((source) => (
              <button
                key={source}
                type="button"
                onClick={() => selectSource(source)}
                aria-pressed={sourceFilter === source}
                className={`h-7 rounded-[5px] px-2.5 text-xs font-semibold transition-colors ${sourceFilter === source
                  ? source === 'company'
                    ? 'bg-[hsl(var(--card))] text-[#2d6687] shadow-sm dark:text-[#72a9c8]'
                    : 'bg-[hsl(var(--card))] text-amber-700 shadow-sm dark:text-amber-300'
                  : 'text-[hsl(var(--muted-foreground))] hover:text-[hsl(var(--foreground))]'
                }`}
              >
                {t(`transactions.sourceTabs.${source}`)}
              </button>
            ))}
          </div>
        ) : (
          <div className="h-8 px-2.5 inline-flex items-center rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-800 text-xs font-medium text-gray-500 dark:text-gray-400">
            {t('common.personal')}
          </div>
        )}

        {/* Account filter */}
        {filteredAccounts.length > 1 && (
          <div className="w-full min-[420px]:w-fit min-w-[6.5rem]">
            <Select
              value={effectiveFilterAccount}
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
          <div className="w-full min-[420px]:w-fit min-w-[6.5rem]">
            <Select
              value={effectiveFilterCategory}
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
          <div className="w-full min-[420px]:w-fit min-w-[6.5rem]">
            <Select
              value={effectiveFilterProject}
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
        {(effectiveFilterCategory || effectiveFilterProject || effectiveFilterAccount) && (
          <button
            onClick={() => { setFilterCategory(''); setFilterProject(''); setFilterAccount('') }}
            className="h-8 px-2.5 rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-800/50 text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-800 text-xs transition-all"
          >
            {t('stats.filter.clear')}
          </button>
        )}
      </div>

      {/* Summary cards — Premium: flat, clean */}
      <StaggerContainer className="grid grid-cols-1 min-[420px]:grid-cols-3 gap-2.5 md:gap-3">
        <StaggerItem>
          <div className="ledger-panel overflow-hidden border-t-2 border-t-[hsl(var(--income))] p-3 md:p-5">
            <p className="page-kicker truncate">{t('stats.yearlyIncome')}</p>
            <p className="mt-1.5 truncate font-data text-base font-semibold tabular-nums text-[hsl(var(--income))] md:text-2xl">
              <CompactAmount compact={fmtShort(totalIncome)} exact={fmtExact(totalIncome)} />
            </p>
          </div>
        </StaggerItem>
        <StaggerItem>
          <div className="ledger-panel overflow-hidden border-t-2 border-t-[hsl(var(--expense))] p-3 md:p-5">
            <p className="page-kicker truncate">{t('stats.yearlyExpense')}</p>
            <p className="mt-1.5 truncate font-data text-base font-semibold tabular-nums text-[hsl(var(--expense))] md:text-2xl">
              <CompactAmount compact={fmtShort(totalExpense)} exact={fmtExact(totalExpense)} />
            </p>
          </div>
        </StaggerItem>
        <StaggerItem>
          <div className="ledger-panel overflow-hidden border-t-2 border-t-[hsl(var(--mode-accent))] p-3 md:p-5">
            <p className="page-kicker truncate">{t('stats.yearlyNet')}</p>
            <p className={`mt-1.5 truncate font-data text-base font-semibold tabular-nums md:text-2xl ${totalNet >= 0 ? 'text-[hsl(var(--mode-accent-strong))]' : 'text-[hsl(var(--expense))]'}`}>
              <CompactAmount compact={fmtShort(totalNet)} exact={fmtExact(totalNet)} prefix={totalNet >= 0 ? '+' : ''} />
            </p>
          </div>
        </StaggerItem>
      </StaggerContainer>

      <AccountBalanceChart accounts={filteredAccounts} />

      {/* Monthly bar chart — Premium */}
      <div className="ledger-panel p-4 md:p-5">
        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 mb-4 md:mb-5">
          <div>
            <h2 className="font-display font-semibold text-[hsl(var(--foreground))]">{t('stats.chart.monthlyTitle', { year })}</h2>
            <p className="mt-0.5 text-xs text-[hsl(var(--muted-foreground))]">{t('stats.chart.monthlySubtitle')}</p>
          </div>
          <div className="flex items-center gap-4 text-xs text-gray-400 dark:text-gray-500">
            <span className="flex items-center gap-1.5">
              <span className="w-3 h-2.5 rounded-sm inline-block" style={{ background: palette.income }} />{t('stats.pie.incomeLabel')}
            </span>
            <span className="flex items-center gap-1.5">
              <span className="w-3 h-2.5 rounded-sm inline-block" style={{ background: palette.expense }} />{t('stats.pie.expenseLabel')}
            </span>
          </div>
        </div>
        {monthly.length === 0 ? (
          <p className="text-sm text-gray-400 dark:text-gray-500 text-center py-10">{t('stats.noData')}</p>
        ) : (
          <MonthlyBarChart data={monthly} fmt={fmt} fmtShort={fmtShort} incomeColor={palette.income} expenseColor={palette.expense} />
        )}
      </div>

      {/* Income vs expense overview pie */}
      {monthly.length > 0 && totalIncome + totalExpense > 0 && (
        <div className="ledger-panel p-4 md:p-5">
          <h2 className="font-display mb-4 font-semibold text-[hsl(var(--foreground))]">{t('stats.chart.pieTitle')}</h2>
          <div className="flex flex-col sm:flex-row items-center sm:items-start gap-5">
            <div className="w-28 h-28 sm:w-32 sm:h-32 shrink-0">
              <ResponsiveContainer width="100%" height="100%">
                <PieChart>
                  <Pie
                    data={[
                      { name: t('stats.pie.incomeLabel'), value: totalIncome },
                      { name: t('stats.pie.expenseLabel'), value: totalExpense },
                    ]}
                    dataKey="value"
                    cx="50%" cy="50%"
                    innerRadius={34}
                    outerRadius={52}
                    paddingAngle={3}
                    strokeWidth={0}
                  >
                    <Cell fill={palette.income} />
                    <Cell fill={palette.expense} />
                  </Pie>
                  <Tooltip formatter={(value, name) => [fmt(value as number), name]} cursor={false}
                    contentStyle={{ borderRadius: '3px', border: '1px solid var(--tooltip-border)', boxShadow: '0 8px 20px rgba(15,31,23,0.10)', fontSize: '12px', fontFamily: 'var(--font-data)', background: 'var(--tooltip-bg)', color: 'var(--tooltip-text)' }}
                    itemStyle={{ color: 'var(--tooltip-text)' }}
                    labelStyle={{ color: 'var(--tooltip-text)' }}
                  />
                </PieChart>
              </ResponsiveContainer>
            </div>
            <div className="space-y-3 flex-1 w-full min-w-0">
              <div>
                <div className="flex items-center justify-between text-sm mb-1 gap-2">
                  <span className="inline-flex items-center gap-1.5 text-gray-500 dark:text-gray-400 font-medium min-w-0 shrink-0">
                    <span className="w-2.5 h-2.5 rounded-full shrink-0" style={{ background: palette.income }} />{t('stats.pie.incomeLabel')}
                  </span>
                  <span className="font-bold tabular-nums truncate text-right" style={{ color: palette.income }}>{fmt(totalIncome)}</span>
                </div>
                <div className="h-1.5 bg-gray-100 dark:bg-gray-700 rounded-full overflow-hidden">
                  <div className="h-full rounded-full" style={{ background: palette.income, width: `${totalIncome + totalExpense > 0 ? Math.round(totalIncome / (totalIncome + totalExpense) * 100) : 0}%` }} />
                </div>
              </div>
              <div>
                <div className="flex items-center justify-between text-sm mb-1 gap-2">
                  <span className="inline-flex items-center gap-1.5 text-gray-500 dark:text-gray-400 font-medium min-w-0 shrink-0">
                    <span className="w-2.5 h-2.5 rounded-full shrink-0" style={{ background: palette.expense }} />{t('stats.pie.expenseLabel')}
                  </span>
                  <span className="font-bold tabular-nums truncate text-right" style={{ color: palette.expense }}>{fmt(totalExpense)}</span>
                </div>
                <div className="h-1.5 bg-gray-100 dark:bg-gray-700 rounded-full overflow-hidden">
                  <div className="h-full rounded-full" style={{ background: palette.expense, width: `${totalIncome + totalExpense > 0 ? Math.round(totalExpense / (totalIncome + totalExpense) * 100) : 0}%` }} />
                </div>
              </div>
              {isWorkMode && sourceFilter === 'personal' && totalReimbursed > 0 && (
                <div className="pt-1 border-t border-gray-100 dark:border-gray-800">
                  <div className="flex items-center justify-between text-xs text-gray-400 dark:text-gray-500 gap-2">
                    <span className="truncate">{t('stats.reimbursed')}</span>
                    <span className="whitespace-nowrap font-data font-semibold tabular-nums text-[hsl(var(--mode-accent-strong))]">+{fmt(totalReimbursed)}</span>
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>
      )}

      <div className="grid grid-cols-1 xl:grid-cols-2 gap-4 md:gap-5">
        <ResponsivePieCard title={t('stats.chart.categoryTitle')} rows={categories} formatFn={fmt} colors={palette.categories} />
        <ResponsivePieCard title={t('stats.chart.incomeCategoryTitle')} rows={incomeCategories} formatFn={fmt} colors={palette.categories} />
      </div>

      {/* Project breakdown — Premium */}
      <div className="ledger-panel overflow-hidden">
        <div className="px-4 md:px-5 py-4 border-b border-gray-100 dark:border-gray-800/50 flex items-center justify-between gap-3">
          <h2 className="font-display font-semibold text-[hsl(var(--foreground))]">{t('stats.chart.projectTitle')}</h2>
          <span className="text-xs text-gray-400 dark:text-gray-500">{t('stats.projectCount', { count: projects.length })}</span>
        </div>
        {projects.length === 0 ? (
          <p className="text-sm text-gray-400 dark:text-gray-500 text-center py-8">{t('stats.noData')}</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm min-w-[320px]">
              <thead>
                <tr className="bg-gray-50/80 dark:bg-gray-800/50 text-gray-400 dark:text-gray-500 text-xs uppercase tracking-wide border-b border-gray-100 dark:border-gray-800/50">
                  <th className="px-4 md:px-5 py-3 text-left font-semibold">{t('stats.project.name')}</th>
                  <th className="px-4 md:px-5 py-3 text-right font-semibold">{t('stats.project.income')}</th>
                  <th className="px-4 md:px-5 py-3 text-right font-semibold">{t('stats.project.expense')}</th>
                  <th className="px-4 md:px-5 py-3 text-right font-semibold">{t('stats.project.net')}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-50 dark:divide-gray-800/50">
                {projects.map((p) => (
                  <tr key={p.project_id} className="hover:bg-gray-50/60 dark:hover:bg-gray-800/30 transition-colors">
                    <td className="px-4 md:px-5 py-3.5">
                      <p className="font-medium text-gray-700 dark:text-gray-300">{p.project_id}</p>
                      {p.project_name && <p className="text-xs text-gray-400 dark:text-gray-500 mt-0.5">{p.project_name}</p>}
                    </td>
                    <td className="px-4 md:px-5 py-3.5 text-right">
                      <span className="whitespace-nowrap font-data font-medium tabular-nums text-[hsl(var(--income))]">
                        <CompactAmount compact={fmtShort(p.income)} exact={fmtExact(p.income)} />
                      </span>
                    </td>
                    <td className="px-4 md:px-5 py-3.5 text-right">
                      <span className="whitespace-nowrap font-data font-medium tabular-nums text-[hsl(var(--expense))]">
                        <CompactAmount compact={fmtShort(p.expense)} exact={fmtExact(p.expense)} />
                      </span>
                    </td>
                    <td className="px-4 md:px-5 py-3.5 text-right">
                      <span className={`rounded px-2 py-0.5 font-data text-xs font-semibold tabular-nums whitespace-nowrap ${p.net >= 0 ? 'bg-[hsl(var(--income))]/10 text-[hsl(var(--income))]' : 'bg-[hsl(var(--expense))]/10 text-[hsl(var(--expense))]'
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
