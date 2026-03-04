import { useTranslation } from 'react-i18next'
import {
    PieChart, Pie, Cell, Tooltip, ResponsiveContainer,
} from 'recharts'
import { categoryLabel } from '../utils/categoryLabel'

const PIE_COLORS = [
    '#8b5cf6', '#f59e0b', '#10b981', '#ef4444', '#06b6d4',
    '#f97316', '#84cc16', '#ec4899', '#22c55e', '#14b8a6',
    '#a855f7', '#eab308', '#0ea5e9',
]

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
    const palette = colors ?? PIE_COLORS

    return (
        <div className="bg-white dark:bg-[hsl(260,15%,11%)] rounded-2xl border border-gray-100/80 dark:border-gray-800/50 p-5 shadow-sm">
            <h2 className="font-semibold text-gray-800 dark:text-gray-200 mb-4">{title}</h2>
            {rows.length === 0 ? (
                <p className="text-sm text-gray-400 dark:text-gray-500 text-center py-8">{t('stats.noData')}</p>
            ) : (
                <div className="flex flex-col md:flex-row gap-6 items-center md:items-start">
                    {/* Pie chart — centered on mobile, left-aligned on desktop */}
                    <div className="w-full max-w-[200px] mx-auto md:mx-0 md:w-64 h-44 md:h-56 shrink-0">
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
                                />
                            </PieChart>
                        </ResponsiveContainer>
                    </div>

                    {/* Legend — wrapping on mobile, vertical scrollable list on desktop */}
                    <div className="w-full flex-1 min-w-0">
                        {/* Mobile: compact wrapping legend */}
                        <div className="flex flex-wrap gap-x-4 gap-y-2 md:hidden">
                            {rows.map((c, idx) => (
                                <div key={c.category} className="flex items-center gap-1.5 min-w-0">
                                    <span
                                        className="w-2.5 h-2.5 rounded-full shrink-0"
                                        style={{ background: palette[idx % palette.length] }}
                                    />
                                    <span className="text-xs text-gray-600 dark:text-gray-400 truncate max-w-[5rem]">
                                        {categoryLabel(c.category)}
                                    </span>
                                    <span className="text-xs font-semibold text-gray-800 dark:text-gray-200 tabular-nums whitespace-nowrap">
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
                                        <span className="text-sm font-medium text-gray-700 dark:text-gray-300 truncate">
                                            {categoryLabel(c.category)}
                                        </span>
                                    </div>
                                    <div className="text-right shrink-0">
                                        <span className="text-sm font-bold text-gray-800 dark:text-gray-200 tabular-nums">
                                            {formatFn(c.total)}
                                        </span>
                                        <span className="text-xs text-gray-400 dark:text-gray-500 ml-1.5">
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
