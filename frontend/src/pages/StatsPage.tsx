import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Cell,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
} from 'recharts'
import { BarChart3, RefreshCw, TriangleAlert } from 'lucide-react'
import { formatAmountCompact, formatAmount, formatAmountExact } from '../utils/format'
import { transactionAmountToCNY } from '../utils/financeAmounts'
import CompactAmount from '../components/CompactAmount'
import { useExchangeRates } from '../hooks/useExchangeRates'
import { useTransactions } from '../hooks/useTransactions'
import { useAccounts } from '../hooks/useAccounts'
import Select from '../components/Select'
import type { Account } from '../api/client'
import { categoryLabel } from '../utils/categoryLabel'
import { buildCategoryChartRows } from '../utils/categoryChart'
import { useMode } from '../hooks/useMode'
import ResponsivePieCard from '../components/ResponsivePieCard'
import { calculateWorkModeAdjustments } from '../utils/workModeStats'
import { useChartPalette } from '../hooks/useChartPalette'
import { accountModeForTransactionSource } from '../utils/accountScope'
import AccountBalanceChart from '../components/AccountBalanceChart'
import { Badge } from '../components/ui/badge'
import { Button } from '../components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../components/ui/card'
import { PageHeader } from '../components/ui/page-header'
import { Segmented, SegmentedButton } from '../components/ui/segmented'
import { Spinner } from '../components/ui/spinner'
import { StatTile } from '../components/ui/stat-tile'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  TableWrapper,
} from '../components/ui/table'

interface MonthData {
  month: number
  income: number
  expense: number
}

interface CategoryRow {
  category: string
  total: number
  count: number
  colorIndex: number
  label?: string
}

function MonthlyBarChart({
  data,
  fmt,
  fmtShort,
  incomeColor,
  expenseColor,
  gridColor,
  surfaceColor,
  foregroundColor,
  mutedForegroundColor,
  borderColor,
}: {
  data: MonthData[]
  fmt: (n: number) => string
  fmtShort: (n: number) => string
  incomeColor: string
  expenseColor: string
  gridColor: string
  surfaceColor: string
  foregroundColor: string
  mutedForegroundColor: string
  borderColor: string
}) {
  const containerRef = useRef<HTMLDivElement>(null)
  const [svgWidth, setSvgWidth] = useState(560)
  const [tooltip, setTooltip] = useState<{
    x: number
    y: number
    income: number
    expense: number
    label: string
  } | null>(null)
  const { t } = useTranslation()
  const monthLabels = t('stats.monthLabels', { returnObjects: true }) as string[]

  useEffect(() => {
    const element = containerRef.current
    if (!element) return

    setSvgWidth(element.clientWidth)
    const observer = new ResizeObserver((entries) => {
      const width = entries[0]?.contentRect.width
      if (width) setSvgWidth(width)
    })
    observer.observe(element)
    return () => observer.disconnect()
  }, [])

  const padLeft = 72
  const padRight = 12
  const padTop = 12
  const padBottom = 28
  const svgHeight = 212
  const chartWidth = Math.max(svgWidth - padLeft - padRight, 1)
  const chartHeight = svgHeight - padTop - padBottom
  const maxValue = Math.max(...data.flatMap((item) => [item.income, item.expense]), 1)
  const roughStep = maxValue / 4
  const magnitude = Math.pow(10, Math.floor(Math.log10(roughStep || 1)))
  const niceStep = Math.ceil(roughStep / magnitude) * magnitude || 1
  const yMax = niceStep * 4
  const yTicks = [0, niceStep, niceStep * 2, niceStep * 3, niceStep * 4]
  const yPosition = (value: number) => padTop + chartHeight - (value / yMax) * chartHeight
  const groupWidth = chartWidth / data.length
  const barGutter = Math.max(groupWidth * 0.18, 3)
  const pairWidth = groupWidth - barGutter * 2
  const barWidth = Math.max(pairWidth / 2 - 2, 4)
  const cornerRadius = Math.min(barWidth / 2, 4)

  function showTooltip(item: MonthData, labelX: number) {
    const incomeY = yPosition(item.income)
    const expenseY = yPosition(item.expense)
    setTooltip({
      x: Math.max(padLeft + 60, Math.min(labelX, svgWidth - 60)),
      y: Math.min(incomeY, expenseY),
      income: item.income,
      expense: item.expense,
      label: monthLabels[item.month - 1] ?? String(item.month),
    })
  }

  return (
    <div ref={containerRef} className="relative w-full" style={{ height: svgHeight }}>
      <svg width={svgWidth} height={svgHeight} role="img" aria-label={t('stats.chart.monthlyTitle', { year: new Date().getFullYear() })}>
        {yTicks.map((value) => {
          const y = yPosition(value)
          return (
            <g key={value}>
              <line
                x1={padLeft}
                y1={y}
                x2={padLeft + chartWidth}
                y2={y}
                stroke={gridColor}
                strokeWidth={value === 0 ? 1.25 : 1}
              />
              <text
                x={padLeft - 6}
                y={y}
                dominantBaseline="middle"
                textAnchor="end"
                fontSize={10}
                fill={mutedForegroundColor}
                fontFamily="inherit"
              >
                {fmtShort(value)}
              </text>
            </g>
          )
        })}

        {data.map((item, index) => {
          const groupX = padLeft + index * groupWidth + barGutter
          const incomeX = groupX
          const expenseX = groupX + barWidth + 2
          const incomeHeight = (item.income / yMax) * chartHeight
          const expenseHeight = (item.expense / yMax) * chartHeight
          const incomeY = yPosition(item.income)
          const expenseY = yPosition(item.expense)
          const labelX = padLeft + (index + 0.5) * groupWidth
          const monthLabel = monthLabels[item.month - 1] ?? String(item.month)

          return (
            <g
              key={item.month}
              tabIndex={0}
              role="img"
              aria-label={`${monthLabel}: ${t('stats.pie.incomeLabel')} ${fmt(item.income)}, ${t('stats.pie.expenseLabel')} ${fmt(item.expense)}`}
              onMouseEnter={() => showTooltip(item, labelX)}
              onMouseLeave={() => setTooltip(null)}
              onFocus={() => showTooltip(item, labelX)}
              onBlur={() => setTooltip(null)}
              className="outline-none focus-visible:[&>rect]:fill-muted"
            >
              <rect
                x={padLeft + index * groupWidth}
                y={padTop}
                width={groupWidth}
                height={chartHeight}
                fill="transparent"
              />
              {item.income > 0 ? (
                <path
                  d={`M${incomeX + cornerRadius},${incomeY} h${barWidth - cornerRadius * 2} a${cornerRadius},${cornerRadius} 0 0 1 ${cornerRadius},${cornerRadius} v${incomeHeight - cornerRadius} h${-barWidth} v${-(incomeHeight - cornerRadius)} a${cornerRadius},${cornerRadius} 0 0 1 ${cornerRadius},${-cornerRadius}z`}
                  fill={incomeColor}
                />
              ) : null}
              {item.expense > 0 ? (
                <path
                  d={`M${expenseX + cornerRadius},${expenseY} h${barWidth - cornerRadius * 2} a${cornerRadius},${cornerRadius} 0 0 1 ${cornerRadius},${cornerRadius} v${expenseHeight - cornerRadius} h${-barWidth} v${-(expenseHeight - cornerRadius)} a${cornerRadius},${cornerRadius} 0 0 1 ${cornerRadius},${-cornerRadius}z`}
                  fill={expenseColor}
                />
              ) : null}
              <text
                x={labelX}
                y={padTop + chartHeight + 18}
                textAnchor="middle"
                fontSize={10}
                fill={mutedForegroundColor}
                fontFamily="inherit"
              >
                {monthLabel}
              </text>
            </g>
          )
        })}
      </svg>

      {tooltip ? (
        <div
          className="pointer-events-none absolute z-10 min-w-32 rounded-lg border px-3 py-2.5 text-xs shadow-[var(--shadow-sm)]"
          style={{
            left: tooltip.x,
            top: Math.max(4, tooltip.y - 72),
            transform: 'translateX(-50%)',
            background: surfaceColor,
            borderColor,
            color: foregroundColor,
          }}
        >
          <p className="mb-1.5 border-b border-border pb-1 font-semibold">{tooltip.label}</p>
          <div className="flex items-center justify-between gap-3">
            <span className="flex items-center gap-1.5 text-muted-foreground">
              <span className="inline-block size-2 shrink-0 rounded-sm" style={{ background: incomeColor }} />
              {t('stats.pie.incomeLabel')}
            </span>
            <span className="font-semibold tabular-nums">{fmt(tooltip.income)}</span>
          </div>
          <div className="mt-1 flex items-center justify-between gap-3">
            <span className="flex items-center gap-1.5 text-muted-foreground">
              <span className="inline-block size-2 shrink-0 rounded-sm" style={{ background: expenseColor }} />
              {t('stats.pie.expenseLabel')}
            </span>
            <span className="font-semibold tabular-nums">{fmt(tooltip.expense)}</span>
          </div>
        </div>
      ) : null}
    </div>
  )
}

export default function StatsPage() {
  const year = new Date().getFullYear()
  const { data: transactions = [], isLoading: loading, isError, refetch, isFetching } = useTransactions()
  const { rates, rateDate, loading: ratesLoading } = useExchangeRates()
  const { t } = useTranslation()
  const { isWorkMode } = useMode()
  const palette = useChartPalette()
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

  const activeAccounts = useMemo(
    () => accounts.filter((account: Account) => account.is_active),
    [accounts],
  )

  const filteredAccounts = useMemo(() => {
    const accountType = sourceFilter === 'company' ? 'public' : 'personal'
    return activeAccounts.filter((account: Account) => account.type === accountType)
  }, [activeAccounts, sourceFilter])

  const allCategories = useMemo(
    () => Array.from(new Set(
      transactions
        .filter((transaction) => transaction.source === sourceFilter)
        .map((transaction) => transaction.category)
        .filter(Boolean),
    )).sort() as string[],
    [transactions, sourceFilter],
  )

  const allProjects = useMemo(
    () => Array.from(new Set(
      transactions
        .filter((transaction) => transaction.source === sourceFilter)
        .map((transaction) => transaction.project_id)
        .filter(Boolean),
    )).sort() as string[],
    [transactions, sourceFilter],
  )

  const effectiveFilterCategory = allCategories.includes(filterCategory) ? filterCategory : ''
  const effectiveFilterProject = allProjects.includes(filterProject) ? filterProject : ''
  const effectiveFilterAccount = filteredAccounts.some((account) => account.id === filterAccount) ? filterAccount : ''

  const filteredBySource = useMemo(
    () => transactions
      .filter((transaction) => transaction.source === sourceFilter)
      .filter((transaction) => !effectiveFilterCategory || transaction.category === effectiveFilterCategory)
      .filter((transaction) => !effectiveFilterProject || (transaction.project_id ?? '') === effectiveFilterProject)
      .filter((transaction) => !effectiveFilterAccount || transaction.account_id === effectiveFilterAccount),
    [
      transactions,
      sourceFilter,
      effectiveFilterCategory,
      effectiveFilterProject,
      effectiveFilterAccount,
    ],
  )

  const fmt = (value: number) => formatAmount(value, 'CNY')
  const fmtExact = (value: number) => formatAmountExact(value, 'CNY')
  const fmtShort = (value: number) => formatAmountCompact(value, 'CNY')
  const otherLabel = t('stats.other')

  const statsData = useMemo(() => {
    const monthlyMap = new Map<number, { month: number; income: number; expense: number; reimbursed: number }>()
    const expenseCategoryMap = new Map<string, { total: number; count: number }>()
    const incomeCategoryMap = new Map<string, { total: number; count: number }>()
    const projectMap = new Map<string, { project_name: string; income: number; expense: number }>()

    for (const transaction of filteredBySource) {
      const cny = transactionAmountToCNY(transaction, rates)
      if (transaction.occurred_at.startsWith(String(year))) {
        const month = parseInt(transaction.occurred_at.substring(5, 7))
        if (!monthlyMap.has(month)) {
          monthlyMap.set(month, { month, income: 0, expense: 0, reimbursed: 0 })
        }
        const entry = monthlyMap.get(month)!
        if (transaction.direction === 'income') {
          entry.income += cny
        } else {
          entry.expense += cny
          if (transaction.reimbursed) entry.reimbursed += cny
        }
      }

      const category = transaction.category || '其他'
      const categoryMap = transaction.direction === 'income' ? incomeCategoryMap : expenseCategoryMap
      if (!categoryMap.has(category)) categoryMap.set(category, { total: 0, count: 0 })
      const categoryEntry = categoryMap.get(category)!
      categoryEntry.total += cny
      categoryEntry.count += 1

      if (transaction.project_id) {
        if (!projectMap.has(transaction.project_id)) {
          projectMap.set(transaction.project_id, {
            project_name: transaction.project_id,
            income: 0,
            expense: 0,
          })
        }
        const projectEntry = projectMap.get(transaction.project_id)!
        if (transaction.direction === 'income') projectEntry.income += cny
        else projectEntry.expense += cny
      }
    }

    const mapCategories = (
      map: Map<string, { total: number; count: number }>,
      selectedCategory = '',
    ): CategoryRow[] => buildCategoryChartRows(
      Array.from(map, ([category, value]) => ({ category, ...value })),
      selectedCategory,
      otherLabel,
    )

    return {
      monthly: Array.from(monthlyMap.values()).sort((a, b) => a.month - b.month),
      categories: mapCategories(expenseCategoryMap, effectiveFilterCategory),
      incomeCategories: mapCategories(incomeCategoryMap, effectiveFilterCategory),
      projects: Array.from(projectMap.entries())
        .map(([project_id, value]) => ({
          project_id,
          project_name: value.project_name,
          income: value.income,
          expense: value.expense,
          net: value.income - value.expense,
        }))
        .sort((a, b) => a.project_id.localeCompare(b.project_id)),
    }
  }, [effectiveFilterCategory, filteredBySource, rates, otherLabel, year])

  const { monthly, categories, incomeCategories, projects } = statsData
  const totalIncome = monthly.reduce((sum, month) => sum + month.income, 0)
  const totalExpense = monthly.reduce((sum, month) => sum + month.expense, 0)

  const { totalReimbursed, adjustedNet: totalNet } = isWorkMode && sourceFilter === 'personal'
    ? calculateWorkModeAdjustments(monthly, totalIncome, totalExpense)
    : { totalReimbursed: 0, adjustedNet: totalIncome - totalExpense }

  if (loading) {
    return (
      <div className="flex h-64 items-center justify-center" role="status" aria-label={t('common.loading')}>
        <Spinner size="lg" className="text-accent" />
      </div>
    )
  }

  if (isError) {
    return (
      <Card className="grid min-h-64 place-items-center">
        <div className="grid justify-items-center gap-3 text-center">
          <span className="grid size-10 place-items-center rounded-lg bg-negative-soft text-negative">
            <TriangleAlert className="size-5" aria-hidden="true" />
          </span>
          <div>
            <h1 className="text-sm font-semibold text-foreground">{t('stats.error.title')}</h1>
            <p className="mt-1 text-xs text-muted-foreground">{t('stats.error.desc')}</p>
          </div>
          <Button onClick={() => void refetch()} disabled={isFetching} loading={isFetching}>
            <RefreshCw className="size-4" aria-hidden="true" />
            {isFetching ? t('common.loading') : t('common.retry')}
          </Button>
        </div>
      </Card>
    )
  }

  const rateBadge = !ratesLoading ? (
    rateDate ? (
      <Badge variant="positive" dot className="max-w-full truncate">
        {t('stats.rateLabel.live')} · $ {rates.USD?.toFixed(2)} · € {rates.EUR?.toFixed(2)} · {rateDate}
      </Badge>
    ) : (
      <Badge variant="warning" dot className="max-w-full truncate">
        {t('stats.rateLabel.fallback')} · $ {rates.USD?.toFixed(2)} · € {rates.EUR?.toFixed(2)}
      </Badge>
    )
  ) : null

  const incomeExpenseTotal = totalIncome + totalExpense
  const incomeShare = incomeExpenseTotal > 0 ? Math.round((totalIncome / incomeExpenseTotal) * 100) : 0
  const expenseShare = incomeExpenseTotal > 0 ? Math.round((totalExpense / incomeExpenseTotal) * 100) : 0

  return (
    <div className="space-y-5 md:space-y-6">
      <PageHeader
        title={t('stats.title')}
        description={t('stats.subtitle', { year })}
        meta={rateBadge}
      />

      <Card className="flex flex-wrap items-center gap-2 p-3" aria-label={t('stats.title')}>
        {isWorkMode ? (
          <Segmented aria-label={t('transactions.sourceFilterLabel')}>
            {(['company', 'personal'] as const).map((source) => (
              <SegmentedButton
                key={source}
                onClick={() => selectSource(source)}
                aria-pressed={sourceFilter === source}
                className="aria-pressed:text-mode"
              >
                {t(`transactions.sourceTabs.${source}`)}
              </SegmentedButton>
            ))}
          </Segmented>
        ) : (
          <Badge variant="mode">{t('common.personal')}</Badge>
        )}

        {filteredAccounts.length > 1 ? (
          <div className="w-full min-w-[6.5rem] min-[420px]:w-fit">
            <Select
              value={effectiveFilterAccount}
              onChange={setFilterAccount}
              placeholder={t('stats.filter.allAccounts')}
              size="sm"
              activeHighlight
              options={[
                { value: '', label: t('stats.filter.allAccounts') },
                ...filteredAccounts.map((account: Account) => ({ value: account.id, label: account.name })),
              ]}
            />
          </div>
        ) : null}

        {allCategories.length > 0 ? (
          <div className="w-full min-w-[6.5rem] min-[420px]:w-fit">
            <Select
              value={effectiveFilterCategory}
              onChange={setFilterCategory}
              placeholder={t('stats.filter.allCategories')}
              size="sm"
              activeHighlight
              options={[
                { value: '', label: t('stats.filter.allCategories') },
                ...allCategories.map((category) => ({ value: category, label: categoryLabel(category) })),
              ]}
            />
          </div>
        ) : null}

        {allProjects.length > 0 ? (
          <div className="w-full min-w-[6.5rem] min-[420px]:w-fit">
            <Select
              value={effectiveFilterProject}
              onChange={setFilterProject}
              placeholder={t('stats.filter.allProjects')}
              size="sm"
              activeHighlight
              options={[
                { value: '', label: t('stats.filter.allProjects') },
                ...allProjects.map((project) => ({ value: project, label: project })),
              ]}
            />
          </div>
        ) : null}

        {effectiveFilterCategory || effectiveFilterProject || effectiveFilterAccount ? (
          <Button
            variant="ghost"
            size="sm"
            onClick={() => {
              setFilterCategory('')
              setFilterProject('')
              setFilterAccount('')
            }}
          >
            {t('stats.filter.clear')}
          </Button>
        ) : null}
      </Card>

      <div className="grid grid-cols-1 gap-2.5 min-[420px]:grid-cols-3 md:gap-3">
        <StatTile
          label={t('stats.yearlyIncome')}
          value={<CompactAmount compact={fmtShort(totalIncome)} exact={fmtExact(totalIncome)} />}
          tone="positive"
        />
        <StatTile
          label={t('stats.yearlyExpense')}
          value={<CompactAmount compact={fmtShort(totalExpense)} exact={fmtExact(totalExpense)} />}
          tone="negative"
        />
        <StatTile
          label={t('stats.yearlyNet')}
          value={(
            <CompactAmount
              compact={fmtShort(totalNet)}
              exact={fmtExact(totalNet)}
              prefix={totalNet >= 0 ? '+' : ''}
            />
          )}
          tone={totalNet >= 0 ? 'positive' : 'negative'}
        />
      </div>

      <AccountBalanceChart accounts={filteredAccounts} />

      <Card>
        <CardHeader className="flex-wrap">
          <div>
            <CardTitle>{t('stats.chart.monthlyTitle', { year })}</CardTitle>
            <CardDescription>{t('stats.chart.monthlySubtitle')}</CardDescription>
          </div>
          <div className="flex items-center gap-4 text-xs text-muted-foreground" aria-label={t('stats.chart.monthlyTitle', { year })}>
            <span className="flex items-center gap-1.5">
              <span className="inline-block h-2.5 w-3 rounded-sm" style={{ background: palette.income }} />
              {t('stats.pie.incomeLabel')}
            </span>
            <span className="flex items-center gap-1.5">
              <span className="inline-block h-2.5 w-3 rounded-sm" style={{ background: palette.expense }} />
              {t('stats.pie.expenseLabel')}
            </span>
          </div>
        </CardHeader>
        <CardContent>
          {monthly.length === 0 ? (
            <p className="py-10 text-center text-sm text-muted-foreground">{t('stats.noData')}</p>
          ) : (
            <>
              <MonthlyBarChart
                data={monthly}
                fmt={fmt}
                fmtShort={fmtShort}
                incomeColor={palette.income}
                expenseColor={palette.expense}
                gridColor={palette.grid}
                surfaceColor={palette.surface}
                foregroundColor={palette.foreground}
                mutedForegroundColor={palette.mutedForeground}
                borderColor={palette.border}
              />
              <Table className="sr-only">
                <caption>{t('stats.chart.monthlyTitle', { year })}</caption>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('stats.chart.monthlyTitle', { year })}</TableHead>
                    <TableHead>{t('stats.pie.incomeLabel')}</TableHead>
                    <TableHead>{t('stats.pie.expenseLabel')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {monthly.map((month) => (
                    <TableRow key={month.month}>
                      <TableCell>{month.month}</TableCell>
                      <TableCell>{fmt(month.income)}</TableCell>
                      <TableCell>{fmt(month.expense)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </>
          )}
        </CardContent>
      </Card>

      {monthly.length > 0 && incomeExpenseTotal > 0 ? (
        <Card>
          <CardHeader>
            <CardTitle id="income-expense-title">{t('stats.chart.pieTitle')}</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col items-center gap-5 sm:flex-row sm:items-start">
            <figure className="size-32 shrink-0" aria-labelledby="income-expense-title">
              <ResponsiveContainer width="100%" height="100%">
                <PieChart>
                  <Pie
                    data={[
                      { name: t('stats.pie.incomeLabel'), value: totalIncome },
                      { name: t('stats.pie.expenseLabel'), value: totalExpense },
                    ]}
                    dataKey="value"
                    cx="50%"
                    cy="50%"
                    innerRadius={34}
                    outerRadius={52}
                    paddingAngle={3}
                    strokeWidth={0}
                  >
                    <Cell fill={palette.income} />
                    <Cell fill={palette.expense} />
                  </Pie>
                  <Tooltip
                    formatter={(value, name) => [fmt(value as number), name]}
                    cursor={false}
                    contentStyle={{
                      borderRadius: '8px',
                      border: `1px solid ${palette.border}`,
                      boxShadow: 'var(--shadow-sm)',
                      fontSize: '12px',
                      background: palette.surface,
                      color: palette.foreground,
                    }}
                    itemStyle={{ color: palette.foreground }}
                    labelStyle={{ color: palette.foreground }}
                  />
                </PieChart>
              </ResponsiveContainer>
            </figure>

            <div className="w-full min-w-0 flex-1 space-y-4">
              <div>
                <div className="mb-1.5 flex items-center justify-between gap-2 text-sm">
                  <span className="inline-flex shrink-0 items-center gap-1.5 font-medium text-muted-foreground">
                    <span className="size-2.5 rounded-full" style={{ background: palette.income }} />
                    {t('stats.pie.incomeLabel')}
                  </span>
                  <span className="truncate text-right font-semibold text-foreground tabular-nums">{fmt(totalIncome)}</span>
                </div>
                <div className="h-1.5 overflow-hidden rounded-full bg-muted">
                  <div className="h-full rounded-full" style={{ background: palette.income, width: `${incomeShare}%` }} />
                </div>
              </div>
              <div>
                <div className="mb-1.5 flex items-center justify-between gap-2 text-sm">
                  <span className="inline-flex shrink-0 items-center gap-1.5 font-medium text-muted-foreground">
                    <span className="size-2.5 rounded-full" style={{ background: palette.expense }} />
                    {t('stats.pie.expenseLabel')}
                  </span>
                  <span className="truncate text-right font-semibold text-foreground tabular-nums">{fmt(totalExpense)}</span>
                </div>
                <div className="h-1.5 overflow-hidden rounded-full bg-muted">
                  <div className="h-full rounded-full" style={{ background: palette.expense, width: `${expenseShare}%` }} />
                </div>
              </div>
              {isWorkMode && sourceFilter === 'personal' && totalReimbursed > 0 ? (
                <div className="flex items-center justify-between gap-2 border-t border-border pt-3 text-xs text-muted-foreground">
                  <span className="truncate">{t('stats.reimbursed')}</span>
                  <span className="whitespace-nowrap font-semibold text-positive tabular-nums">+{fmt(totalReimbursed)}</span>
                </div>
              ) : null}
            </div>
          </CardContent>
        </Card>
      ) : null}

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
        <ResponsivePieCard
          title={t('stats.chart.categoryTitle')}
          rows={categories}
          formatFn={fmt}
        />
        <ResponsivePieCard
          title={t('stats.chart.incomeCategoryTitle')}
          rows={incomeCategories}
          formatFn={fmt}
        />
      </div>

      <Card className="overflow-hidden p-0">
        <CardHeader className="border-b border-border px-4 py-4 md:px-5">
          <div>
            <CardTitle>{t('stats.chart.projectTitle')}</CardTitle>
            <CardDescription>{t('stats.projectCount', { count: projects.length })}</CardDescription>
          </div>
          <BarChart3 className="size-4 text-muted-foreground" aria-hidden="true" />
        </CardHeader>
        {projects.length === 0 ? (
          <p className="py-8 text-center text-sm text-muted-foreground">{t('stats.noData')}</p>
        ) : (
          <TableWrapper className="rounded-none border-0">
            <Table className="min-w-[420px]">
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead className="px-4 text-muted-foreground md:px-5">{t('stats.project.name')}</TableHead>
                  <TableHead className="px-4 text-right text-muted-foreground md:px-5">{t('stats.project.income')}</TableHead>
                  <TableHead className="px-4 text-right text-muted-foreground md:px-5">{t('stats.project.expense')}</TableHead>
                  <TableHead className="px-4 text-right text-muted-foreground md:px-5">{t('stats.project.net')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {projects.map((project) => (
                  <TableRow key={project.project_id}>
                    <TableCell className="px-4 md:px-5">
                      <p className="font-medium text-foreground">{project.project_id}</p>
                      {project.project_name ? (
                        <p className="mt-0.5 text-xs text-muted-foreground">{project.project_name}</p>
                      ) : null}
                    </TableCell>
                    <TableCell className="px-4 text-right md:px-5">
                      <span className="whitespace-nowrap font-medium text-positive tabular-nums">
                        <CompactAmount compact={fmtShort(project.income)} exact={fmtExact(project.income)} />
                      </span>
                    </TableCell>
                    <TableCell className="px-4 text-right md:px-5">
                      <span className="whitespace-nowrap font-medium text-negative tabular-nums">
                        <CompactAmount compact={fmtShort(project.expense)} exact={fmtExact(project.expense)} />
                      </span>
                    </TableCell>
                    <TableCell className="px-4 text-right md:px-5">
                      <Badge variant={project.net >= 0 ? 'positive' : 'negative'}>
                        <CompactAmount
                          compact={fmtShort(project.net)}
                          exact={fmtExact(project.net)}
                          prefix={project.net >= 0 ? '+' : ''}
                        />
                      </Badge>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableWrapper>
        )}
      </Card>
    </div>
  )
}
