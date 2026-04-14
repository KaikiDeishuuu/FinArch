import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ResponsiveContainer, LineChart, CartesianGrid, XAxis, YAxis, Tooltip, Line } from 'recharts'
import type { Account } from '../api/client'
import Select from './Select'
import { useAccountBalanceHistory, type BalanceRange } from '../hooks/useAccountBalanceHistory'
import { formatAmount, formatAmountCompact } from '../utils/format'
import { useMode } from '../contexts/ModeContext'
import { getModeChartPalette } from '../utils/chartPalette'

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
  const { mode } = useMode()
  const palette = getModeChartPalette(mode)

  const [range, setRange] = useState<BalanceRange>('30d')
  const [accountId, setAccountId] = useState('')

  const activeAccounts = useMemo(() => accounts.filter(a => a.is_active), [accounts])

  useEffect(() => {
    if (accountId && !accounts.some(a => a.id === accountId)) {
      setAccountId('')
    }
  }, [accounts, accountId])

  const { data = [], isLoading } = useAccountBalanceHistory(range, accountId || undefined)

  const chartData = useMemo(() => data.map((p) => ({
    ...p,
    shortDate: range === '1y' || range === 'all' ? p.date.slice(0, 7) : p.date.slice(5),
  })), [data, range])

  return (
    <div className="bg-white dark:bg-[hsl(260,15%,11%)] rounded-2xl border border-gray-100/80 dark:border-gray-800/50 p-5 shadow-sm">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between mb-4">
        <div>
          <h2 className="font-semibold text-gray-800 dark:text-gray-200">{t('stats.chart.balanceTitle')}</h2>
          <p className="text-xs text-gray-400 dark:text-gray-500 mt-0.5">{t('stats.chart.balanceSubtitle')}</p>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <div className="flex items-center gap-1 p-1 rounded-lg bg-gray-100/80 dark:bg-gray-800/60">
            {RANGE_OPTIONS.map((opt) => (
              <button
                key={opt.value}
                type="button"
                onClick={() => setRange(opt.value)}
                className={`px-2 py-1 rounded-md text-[11px] font-semibold transition-colors ${
                  range === opt.value
                    ? 'bg-white dark:bg-gray-700 text-violet-600 dark:text-violet-300 shadow-sm'
                    : 'text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200'
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
                value={accountId}
                onChange={setAccountId}
                placeholder={t('stats.chart.allAccounts')}
                options={[
                  { value: '', label: t('stats.chart.allAccounts') },
                  ...activeAccounts.map((a) => ({ value: a.id, label: a.name })),
                ]}
              />
            </div>
          )}
        </div>
      </div>

      {isLoading ? (
        <div className="h-64 flex items-center justify-center">
          <div className="w-7 h-7 border-4 border-violet-500 border-t-transparent rounded-full animate-spin" />
        </div>
      ) : chartData.length === 0 ? (
        <p className="text-sm text-gray-400 dark:text-gray-500 text-center py-12">{t('stats.chart.balanceNoData')}</p>
      ) : (
        <div className="h-64 w-full">
          <ResponsiveContainer width="100%" height="100%">
            <LineChart data={chartData} margin={{ top: 8, right: 16, bottom: 0, left: 4 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="rgba(148, 163, 184, 0.20)" />
              <XAxis dataKey="shortDate" tick={{ fontSize: 11, fill: '#94a3b8' }} minTickGap={18} />
              <YAxis
                tick={{ fontSize: 11, fill: '#94a3b8' }}
                width={62}
                tickFormatter={(v) => formatAmountCompact(Number(v), 'CNY')}
              />
              <Tooltip
                formatter={(value) => formatAmount(Number(value), 'CNY')}
                labelFormatter={(_, payload) => payload?.[0]?.payload?.date ?? ''}
                contentStyle={{ borderRadius: '12px', border: '1px solid #e5e7eb', fontSize: '12px' }}
              />
              <Line
                type="monotone"
                dataKey="balance"
                stroke={palette.secondary}
                strokeWidth={2.5}
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
