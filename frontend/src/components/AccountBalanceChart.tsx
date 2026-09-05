import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ResponsiveContainer, LineChart, CartesianGrid, XAxis, YAxis, Tooltip, Line } from 'recharts'
import type { Account } from '../api/client'
import Select from './Select'
import { useAccountBalanceHistories, useAccountBalanceHistory, type BalanceRange } from '../hooks/useAccountBalanceHistory'
import { formatAmount, formatAmountCompact } from '../utils/format'
import { useExchangeRates } from '../hooks/useExchangeRates'
import { aggregateAccountBalanceHistories } from '../utils/financeAmounts'

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

  const [range, setRange] = useState<BalanceRange>('30d')
  const [accountId, setAccountId] = useState('')

  const activeAccounts = useMemo(() => accounts.filter(a => a.is_active), [accounts])
  const selectedAccountId = accountId && accounts.some(a => a.id === accountId) ? accountId : ''
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

  const chartData = useMemo(() => data.map((p) => ({
    ...p,
    shortDate: range === '1y' || range === 'all' ? p.date.slice(0, 7) : p.date.slice(5),
  })), [data, range])

  return (
    <div className="border border-[hsl(var(--border))] bg-[hsl(var(--card))] p-4 sm:p-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between mb-4">
        <div>
          <p className="page-kicker mb-1">BALANCE LEDGER</p>
          <h2 className="font-display font-semibold text-[hsl(var(--foreground))]">{t('stats.chart.balanceTitle')}</h2>
          <p className="mt-0.5 text-xs text-[hsl(var(--muted-foreground))]">
            {t('stats.chart.balanceSubtitle')} · {selectedAccount ? chartCurrency : t('stats.chart.cnyEquivalent')}
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <div role="group" aria-label={t('stats.chart.rangeLabel')} className="flex items-center gap-px border border-[hsl(var(--border))] bg-[hsl(var(--border))] p-px">
            {RANGE_OPTIONS.map((opt) => (
              <button
                key={opt.value}
                type="button"
                onClick={() => setRange(opt.value)}
                aria-pressed={range === opt.value}
                className={`bg-[hsl(var(--card))] px-2 py-1 font-data text-[11px] font-semibold transition-colors ${
                  range === opt.value
                    ? 'text-[hsl(var(--mode-accent))] underline decoration-2 underline-offset-4'
                    : 'text-[hsl(var(--muted-foreground))] hover:bg-[hsl(var(--muted))] hover:text-[hsl(var(--foreground))]'
                }`}
              >
                {opt.label}
              </button>
            ))}
          </div>

          {activeAccounts.length > 1 && (
            <div className="w-fit min-w-[8rem]">
              <Select
                size="sm"
                value={selectedAccountId}
                onChange={setAccountId}
                placeholder={t('stats.chart.allAccounts')}
                options={[
                  { value: '', label: t('stats.chart.allAccounts') },
                  ...activeAccounts.map((a) => ({ value: a.id, label: `${a.name} · ${a.currency}` })),
                ]}
              />
            </div>
          )}
        </div>
      </div>

      {isLoading ? (
        <div className="h-64 flex items-center justify-center">
          <div className="h-7 w-7 animate-spin rounded-full border-4 border-[hsl(var(--mode-accent))] border-t-transparent" />
        </div>
      ) : isError ? (
        <div className="h-64 flex flex-col items-center justify-center gap-2 text-sm text-rose-500">
          <p>{t('common.error')}</p>
          <button type="button" onClick={retry} className="font-semibold underline underline-offset-2">
            {t('common.retry')}
          </button>
        </div>
      ) : chartData.length === 0 ? (
        <p className="py-12 text-center text-sm text-[hsl(var(--muted-foreground))]">{t('stats.chart.balanceNoData')}</p>
      ) : (
        <div className="h-64 w-full" role="img" aria-label={t('stats.chart.balanceTitle')}>
          <ResponsiveContainer width="100%" height="100%">
            <LineChart data={chartData} margin={{ top: 8, right: 16, bottom: 0, left: 4 }}>
              <CartesianGrid strokeDasharray="2 5" stroke="hsl(var(--border))" />
              <XAxis dataKey="shortDate" tick={{ fontSize: 11, fill: 'hsl(var(--muted-foreground))', fontFamily: 'var(--font-data)' }} minTickGap={18} />
              <YAxis
                tick={{ fontSize: 11, fill: 'hsl(var(--muted-foreground))', fontFamily: 'var(--font-data)' }}
                width={62}
                tickFormatter={(v) => formatAmountCompact(Number(v), chartCurrency)}
              />
              <Tooltip
                formatter={(value) => formatAmount(Number(value), chartCurrency)}
                labelFormatter={(_, payload) => payload?.[0]?.payload?.date ?? ''}
                contentStyle={{ borderRadius: '3px', border: '1px solid var(--tooltip-border)', background: 'var(--tooltip-bg)', color: 'var(--tooltip-text)', fontSize: '12px', fontFamily: 'var(--font-data)' }}
              />
              <Line
                type="monotone"
                dataKey="balance"
                stroke="hsl(var(--mode-accent))"
                strokeWidth={2.25}
                dot={chartData.length === 1}
                activeDot={{ r: 4 }}
                animationDuration={320}
              />
            </LineChart>
          </ResponsiveContainer>
        </div>
      )}
    </div>
  )
}
