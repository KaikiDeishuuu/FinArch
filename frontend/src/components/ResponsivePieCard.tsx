import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Cell,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
} from 'recharts'
import { useChartPalette } from '../hooks/useChartPalette'
import { categoryLabel } from '../utils/categoryLabel'
import type { CategoryChartRow } from '../utils/categoryChart'
import { Card, CardContent, CardHeader, CardTitle } from './ui/card'

interface DisplayPieRow extends CategoryChartRow {
  label: string
}

interface ResponsivePieCardProps {
  title: string
  rows: CategoryChartRow[]
  formatFn: (n: number) => string
}

export default function ResponsivePieCard({ title, rows, formatFn }: ResponsivePieCardProps) {
  const { t } = useTranslation()
  const chartPalette = useChartPalette()
  const displayRows = useMemo<DisplayPieRow[]>(
    () => rows.map((row) => ({
      ...row,
      label: row.label ?? categoryLabel(row.category),
    })),
    [rows],
  )

  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
      </CardHeader>
      {displayRows.length === 0 ? (
        <p className="py-8 text-center text-sm text-muted-foreground">{t('stats.noData')}</p>
      ) : (
        <CardContent className="flex flex-col items-center gap-4 md:flex-row md:items-start md:gap-6">
          <div
            role="img"
            aria-label={title}
            className="mx-auto h-44 w-full max-w-48 shrink-0 md:mx-0 md:h-56 md:w-64 md:max-w-none"
          >
            <ResponsiveContainer width="100%" height="100%">
              <PieChart>
                <Pie
                  data={displayRows}
                  dataKey="total"
                  nameKey="label"
                  cx="50%"
                  cy="50%"
                  innerRadius={52}
                  outerRadius={82}
                  paddingAngle={2}
                  stroke={chartPalette.surface}
                  strokeWidth={2}
                >
                  {displayRows.map((row) => (
                    <Cell key={row.category} fill={chartPalette.categories[row.colorIndex]} />
                  ))}
                </Pie>
                <Tooltip
                  formatter={(value, name) => [formatFn(Number(value)), name]}
                  cursor={false}
                  contentStyle={{
                    background: chartPalette.surface,
                    border: `1px solid ${chartPalette.border}`,
                    borderRadius: 8,
                    color: chartPalette.foreground,
                    fontSize: 12,
                    boxShadow: 'var(--shadow-sm)',
                  }}
                  itemStyle={{ color: chartPalette.foreground }}
                  labelStyle={{ color: chartPalette.mutedForeground }}
                />
              </PieChart>
            </ResponsiveContainer>
          </div>

          <div className="grid w-full min-w-0 flex-1 grid-cols-1 gap-px overflow-hidden rounded-lg border border-border bg-border sm:grid-cols-2 md:max-h-56 md:grid-cols-1 md:overflow-y-auto">
            {displayRows.map((row) => (
              <div
                key={row.category}
                className="flex min-w-0 items-center justify-between gap-3 bg-card px-3 py-2.5"
              >
                <div className="flex min-w-0 items-center gap-2">
                  <span
                    aria-hidden="true"
                    className="size-2.5 shrink-0 rounded-sm"
                    style={{ backgroundColor: chartPalette.categories[row.colorIndex] }}
                  />
                  <span className="truncate text-xs font-medium text-foreground" title={row.label}>
                    {row.label}
                  </span>
                </div>
                <div className="shrink-0 text-right">
                  <p className="text-xs font-semibold text-foreground tabular-nums">{formatFn(row.total)}</p>
                  <p className="mt-0.5 text-[10px] text-muted-foreground">
                    {t('stats.transactionUnit', { count: row.count })}
                  </p>
                </div>
              </div>
            ))}
          </div>
        </CardContent>
      )}
    </Card>
  )
}
