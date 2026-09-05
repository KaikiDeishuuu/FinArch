import { useTranslation } from 'react-i18next'
import {
    PieChart, Pie, Cell, Tooltip, ResponsiveContainer,
} from 'recharts'
import { categoryLabel } from '../utils/categoryLabel'
import { useMode } from '../hooks/useMode'
import { getModeChartPalette } from '../utils/chartPalette'

interface PieRow {
    category: string
    total: number
    count: number
}

interface ResponsivePieCardProps {
    title: string
    rows: PieRow[]
    formatFn: (n: number) => string
    colors?: string[]
}

export default function ResponsivePieCard({ title, rows, formatFn, colors }: ResponsivePieCardProps) {
    const { t } = useTranslation()
    const { mode } = useMode()
    const palette = colors ?? getModeChartPalette(mode).categories

    return (
        <div className="border border-[hsl(var(--border))] bg-[hsl(var(--card))] p-4 md:p-5">
            <p className="page-kicker mb-1">CATEGORY LEDGER</p>
            <h2 className="font-display mb-3 font-semibold text-[hsl(var(--foreground))] md:mb-4">{title}</h2>
            {rows.length === 0 ? (
                <p className="py-8 text-center text-sm text-[hsl(var(--muted-foreground))]">{t('stats.noData')}</p>
            ) : (
                <div className="flex flex-col md:flex-row gap-4 md:gap-6 items-center md:items-start">
                    {/* Pie chart — centered on mobile, left-aligned on desktop */}
                    <div className="mx-auto h-40 w-full max-w-[180px] shrink-0 md:mx-0 md:h-56 md:w-64" role="img" aria-label={title}>
                        <ResponsiveContainer width="100%" height="100%">
                            <PieChart>
                                <Pie
                                    data={rows}
                                    dataKey="total"
                                    nameKey="category"
                                    cx="50%"
                                    cy="50%"
                                    innerRadius={52}
                                    outerRadius={82}
                                    paddingAngle={2}
                                    strokeWidth={0}
                                >
                                    {rows.map((_, i) => (
                                        <Cell key={i} fill={palette[i % palette.length]} />
                                    ))}
                                </Pie>
                                <Tooltip
                                    formatter={(value, name) => [formatFn(value as number), name]}
                                    cursor={false}
                                    contentStyle={{ borderRadius: '3px', border: '1px solid var(--tooltip-border)', background: 'var(--tooltip-bg)', color: 'var(--tooltip-text)', fontFamily: 'var(--font-data)' }}
                                />
                            </PieChart>
                        </ResponsiveContainer>
                    </div>

                    {/* Legend — wrapping on mobile, vertical scrollable list on desktop */}
                    <div className="w-full flex-1 min-w-0">
                        {/* Mobile: compact wrapping legend */}
                        <div className="grid grid-cols-2 gap-x-3 gap-y-2 md:hidden">
                            {rows.map((c, idx) => (
                                <div key={c.category} className="flex items-center gap-1.5 min-w-0">
                                    <span
                                        className="w-2.5 h-2.5 rounded-full shrink-0"
                                        style={{ background: palette[idx % palette.length] }}
                                    />
                                    <span className="max-w-[5rem] truncate text-xs text-[hsl(var(--muted-foreground))]">
                                        {categoryLabel(c.category)}
                                    </span>
                                    <span className="font-data whitespace-nowrap text-xs font-semibold tabular-nums text-[hsl(var(--foreground))]">
                                        {formatFn(c.total)}
                                    </span>
                                </div>
                            ))}
                        </div>

                        {/* Desktop: full legend list */}
                        <div className="hidden md:block space-y-3 max-h-56 overflow-y-auto pr-1">
                            {rows.map((c, idx) => (
                                <div key={c.category} className="flex items-center justify-between gap-3">
                                    <div className="flex items-center gap-2 min-w-0">
                                        <span
                                            className="w-3 h-3 rounded-full shrink-0"
                                            style={{ background: palette[idx % palette.length] }}
                                        />
                                        <span className="truncate text-sm font-medium text-[hsl(var(--foreground))]">
                                            {categoryLabel(c.category)}
                                        </span>
                                    </div>
                                    <div className="text-right shrink-0">
                                        <span className="font-data text-sm font-bold tabular-nums text-[hsl(var(--foreground))]">
                                            {formatFn(c.total)}
                                        </span>
                                        <span className="ml-1.5 text-xs text-[hsl(var(--muted-foreground))]">
                                            {t('stats.transactionUnit', { count: c.count })}
                                        </span>
                                    </div>
                                </div>
                            ))}
                        </div>
                    </div>
                </div>
            )}
        </div>
    )
}
