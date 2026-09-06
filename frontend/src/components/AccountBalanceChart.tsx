import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import type { Account } from '../api/client'
import { useAccountBalanceHistories, useAccountBalanceHistory, type BalanceRange } from '../hooks/useAccountBalanceHistory'
import { useChartPalette } from '../hooks/useChartPalette'
import { useExchangeRates } from '../hooks/useExchangeRates'
import { aggregateAccountBalanceHistories } from '../utils/financeAmounts'
import { formatAmount, formatAmountCompact } from '../utils/format'
import Select from './Select'
import { Button } from './ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card'
import { EmptyState } from './ui/empty-state'
import { Segmented, SegmentedButton } from './ui/segmented'
import { Spinner } from './ui/spinner'

interface Props {
  accounts: Account[]
}

const RANGE_OPTIONS: Array<{ value: BalanceRange; label: string }> = [
  { value: '7d', label: '7D' },
  { value: '30d', label: '30D' },
  { value: '90d', label: '90D' },
  { value: '1y', label: '1Y' },
  { value: 'all', label: 'ALL' },
]

export default function AccountBalanceChart({ accounts }: Props) {
  const { t } = useTranslation()
  const { rates } = useExchangeRates()
  const palette = useChartPalette()

  const [range, setRange] = useState<BalanceRange>('30d')
  const [accountId, setAccountId] = useState('')

  const activeAccounts = useMemo(() => accounts.filter((account) => account.is_active), [accounts])
  const selectedAccountId = accountId && accounts.some((account) => account.id === accountId) ? accountId : ''
  const selectedAccount = activeAccounts.find((account) => account.id === selectedAccountId)

  const selectedHistory = useAccountBalanceHistory(range, selectedAccountId || undefined, Boolean(selectedAccountId))
  const allHistories = useAccountBalanceHistories(
    range,
    selectedAccountId ? [] : activeAccounts.map((account) => account.id),
  )

  const data = useMemo(() => {
    if (selectedAccount) return selectedHistory.data ?? []
    return aggregateAccountBalanceHistories(
      activeAccounts.map((account, index) => ({
        account,
        points: allHistories[index]?.data ?? [],
      })),
      rates,
    )
  }, [activeAccounts, allHistories, rates, selectedAccount, selectedHistory.data])
  const chartCurrency = selectedAccount?.currency || 'CNY'
  const isLoading = selectedAccount
    ? selectedHistory.isLoading
    : allHistories.some((query) => query.isLoading)
  const isError = selectedAccount
    ? selectedHistory.isError
    : allHistories.some((query) => query.isError)

  const retry = () => {
    if (selectedAccount) {
      void selectedHistory.refetch()
      return
    }
    allHistories.forEach((query) => { void query.refetch() })
  }

  const chartData = useMemo(() => data.map((point) => ({
    ...point,
    shortDate: range === '1y' || range === 'all' ? point.date.slice(0, 7) : point.date.slice(5),
  })), [data, range])

  return (
    <Card>
      <CardHeader className="flex-col gap-3 sm:flex-row sm:items-start">
        <div>
          <CardTitle>{t('stats.chart.balanceTitle')}</CardTitle>
          <CardDescription>
            {t('stats.chart.balanceSubtitle')} · {selectedAccount ? chartCurrency : t('stats.chart.cnyEquivalent')}
          </CardDescription>
        </div>

        <div className="flex w-full flex-wrap items-center gap-2 sm:w-auto sm:justify-end">
          <Segmented
            role="group" aria-label={t('stats.chart.rangeLabel')}
            className="grid flex-1 grid-cols-5 sm:flex-none"
          >
            {RANGE_OPTIONS.map((opt) => (
              <SegmentedButton
                key={opt.value}
                type="button"
                onClick={() => setRange(opt.value)}
                aria-pressed={range === opt.value}
                className="min-w-0 px-2 font-mono text-[10px]"
              >
                {opt.label}
              </SegmentedButton>
            ))}
          </Segmented>

          {activeAccounts.length > 1 ? (
            <div className="min-w-36 flex-1 sm:flex-none">
              <Select
                size="sm"
                value={selectedAccountId}
                onChange={setAccountId}
                placeholder={t('stats.chart.allAccounts')}
                options={[
                  { value: '', label: t('stats.chart.allAccounts') },
                  ...activeAccounts.map((account) => ({
                    value: account.id,
                    label: `${account.name} · ${account.currency}`,
                  })),
                ]}
              />
            </div>
          ) : null}
        </div>
      </CardHeader>

      <CardContent>
        {isLoading ? (
          <div className="flex h-64 items-center justify-center" role="status" aria-label={t('common.loading')}>
            <Spinner className="size-6 text-accent" />
          </div>
        ) : isError ? (
          <div className="flex h-64 flex-col items-center justify-center gap-3 text-center">
            <p className="text-sm font-medium text-negative">{t('common.error')}</p>
            <Button type="button" variant="outline" size="sm" onClick={retry}>
              {t('common.retry')}
            </Button>
          </div>
        ) : chartData.length === 0 ? (
          <EmptyState title={t('stats.chart.balanceNoData')} />
        ) : (
          <figure aria-labelledby="account-balance-title">
            <figcaption id="account-balance-title" className="sr-only">
              {t('stats.chart.balanceTitle')}
            </figcaption>
            <div className="h-64 w-full" aria-hidden="true">
              <ResponsiveContainer width="100%" height="100%">
              <LineChart data={chartData} margin={{ top: 8, right: 16, bottom: 0, left: 4 }}>
                <CartesianGrid strokeDasharray="3 3" stroke={palette.grid} vertical={false} />
                <XAxis
                  dataKey="shortDate"
                  axisLine={{ stroke: palette.border }}
                  tickLine={false}
                  tick={{ fontSize: 11, fill: palette.mutedForeground }}
                  minTickGap={18}
                />
                <YAxis
                  axisLine={false}
                  tickLine={false}
                  tick={{ fontSize: 11, fill: palette.mutedForeground }}
                  width={62}
                  tickFormatter={(value) => formatAmountCompact(Number(value), chartCurrency)}
                />
                <Tooltip
                  formatter={(value) => formatAmount(Number(value), chartCurrency)}
                  labelFormatter={(_, payload) => payload?.[0]?.payload?.date ?? ''}
                  contentStyle={{
                    background: palette.surface,
                    border: `1px solid ${palette.border}`,
                    borderRadius: 8,
                    color: palette.foreground,
                    fontSize: 12,
                    boxShadow: 'var(--shadow-sm)',
                  }}
                  itemStyle={{ color: palette.foreground }}
                  labelStyle={{ color: palette.mutedForeground }}
                  cursor={{ stroke: palette.mutedForeground, strokeDasharray: '3 3' }}
                />
                <Line
                  type="monotone"
                  dataKey="balance"
                  stroke={palette.secondary}
                  strokeWidth={2}
                  dot={chartData.length === 1 ? { r: 4, fill: palette.secondary } : false}
                  activeDot={{ r: 4, fill: palette.secondary, stroke: palette.surface, strokeWidth: 2 }}
                  animationDuration={320}
                />
                </LineChart>
              </ResponsiveContainer>
            </div>
            <table className="sr-only">
              <caption>{t('stats.chart.balanceTitle')}</caption>
              <thead>
                <tr>
                  <th scope="col">{t('common.date')}</th>
                  <th scope="col">{t('stats.chart.balanceTitle')} · {chartCurrency}</th>
                </tr>
              </thead>
              <tbody>
                {chartData.map((point) => (
                  <tr key={point.date}>
                    <th scope="row">{point.date}</th>
                    <td>{formatAmount(point.balance, chartCurrency)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </figure>
        )}
      </CardContent>
    </Card>
  )
}
