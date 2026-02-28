import { useEffect, useState } from 'react'
import {
  PieChart, Pie, Cell, Tooltip, ResponsiveContainer,
  BarChart, Bar, XAxis, YAxis, CartesianGrid,
} from 'recharts'
import { getStatsMonthly, getStatsByCategory, getStatsByProject } from '../api/client'
import type { MonthlyStat, CategoryStat, ProjectStat } from '../api/client'
import { formatAmountCompact, formatAmount } from '../utils/format'

const PIE_COLORS = [
  '#3b82f6','#f59e0b','#10b981','#ef4444','#8b5cf6',
  '#06b6d4','#f97316','#84cc16','#ec4899','#6366f1',
  '#14b8a6','#a855f7','#eab308',
]

export default function StatsPage() {
  const year = new Date().getFullYear()
  const [monthly, setMonthly] = useState<MonthlyStat[]>([])
  const [categories, setCategories] = useState<CategoryStat[]>([])
  const [projects, setProjects] = useState<ProjectStat[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    Promise.all([
      getStatsMonthly(year),
      getStatsByCategory(),
      getStatsByProject(),
    ]).then(([m, c, p]) => {
      setMonthly(m ?? [])
      setCategories(c ?? [])
      setProjects(p ?? [])
    }).finally(() => setLoading(false))
  }, [year])

  const fmt = (n: number) => formatAmount(n, 'CNY')
  const fmtShort = (n: number) => formatAmountCompact(n, 'CNY')
  const monthNames = ['1月','2月','3月','4月','5月','6月','7月','8月','9月','10月','11月','12月']

  const totalIncome = monthly.reduce((s, m) => s + m.income, 0)
  const totalExpense = monthly.reduce((s, m) => s + m.expense, 0)
  const totalReimbursed = monthly.reduce((s, m) => s + (m.reimbursed ?? 0), 0)
  const totalNet = totalIncome - totalExpense + totalReimbursed

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="w-8 h-8 border-4 border-blue-500 border-t-transparent rounded-full animate-spin" />
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-gray-900 tracking-tight">统计分析</h1>
        <p className="text-sm text-gray-400 mt-1">{year} 年度资金概览</p>
      </div>

      {/* Summary cards */}
      <div className="grid grid-cols-3 gap-3">
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-3 md:p-5 overflow-hidden relative">
          <div className="absolute top-0 left-0 right-0 h-1 rounded-t-2xl bg-gradient-to-r from-emerald-400 to-green-500" />
          <p className="text-[10px] md:text-[11px] text-gray-400 uppercase tracking-wider font-semibold mt-2">年度收入</p>
          <p className="text-base md:text-2xl font-bold text-emerald-600 truncate tabular-nums mt-1">{fmtShort(totalIncome)}</p>
        </div>
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-3 md:p-5 overflow-hidden relative">
          <div className="absolute top-0 left-0 right-0 h-1 rounded-t-2xl bg-gradient-to-r from-red-400 to-rose-500" />
          <p className="text-[10px] md:text-[11px] text-gray-400 uppercase tracking-wider font-semibold mt-2">年度支出</p>
          <p className="text-base md:text-2xl font-bold text-red-500 truncate tabular-nums mt-1">{fmtShort(totalExpense)}</p>
        </div>
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-3 md:p-5 overflow-hidden relative">
          <div className={`absolute top-0 left-0 right-0 h-1 rounded-t-2xl ${totalNet >= 0 ? 'bg-gradient-to-r from-blue-400 to-indigo-500' : 'bg-gradient-to-r from-orange-400 to-amber-500'}`} />
          <p className="text-[10px] md:text-[11px] text-gray-400 uppercase tracking-wider font-semibold mt-2">净结余</p>
          <p className={`text-base md:text-2xl font-bold truncate tabular-nums mt-1 ${totalNet >= 0 ? 'text-blue-600' : 'text-orange-500'}`}>
            {totalNet >= 0 ? '+' : ''}{fmtShort(totalNet)}
          </p>
        </div>
      </div>

      {/* Monthly bar chart */}
      <div className="bg-white rounded-2xl shadow-sm border border-gray-100 p-5">
        <div className="flex items-center justify-between mb-5">
          <div>
            <h2 className="font-semibold text-gray-800">{year} 年月度收支</h2>
            <p className="text-xs text-gray-400 mt-0.5">按月统计收入与支出</p>
          </div>
          <div className="flex items-center gap-4 text-xs text-gray-400">
            <span className="flex items-center gap-1.5">
              <span className="w-3 h-2.5 rounded-sm bg-emerald-400 inline-block" />收入
            </span>
            <span className="flex items-center gap-1.5">
              <span className="w-3 h-2.5 rounded-sm bg-red-400 inline-block" />支出
            </span>
          </div>
        </div>
        {monthly.length === 0 ? (
          <p className="text-sm text-gray-400 text-center py-10">暂无数据</p>
        ) : (
          <div className="h-52 md:h-64">
            <ResponsiveContainer width="100%" height="100%">
              <BarChart
                data={monthly.map(m => ({ name: monthNames[m.month - 1], income: m.income, expense: m.expense }))}
                margin={{ top: 4, right: 8, left: -16, bottom: 0 }}
                barCategoryGap="32%"
                barGap={3}
              >
                <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" vertical={false} />
                <XAxis
                  dataKey="name"
                  tick={{ fontSize: 11, fill: '#9ca3af' }}
                  axisLine={false}
                  tickLine={false}
                />
                <YAxis
                  tick={{ fontSize: 11, fill: '#9ca3af' }}
                  axisLine={false}
                  tickLine={false}
                  tickFormatter={fmtShort}
                  width={56}
                />
                <Tooltip
                  formatter={(value, name) => [fmt(value as number), name === 'income' ? '收入' : '支出']}
                  labelStyle={{ fontWeight: 600, color: '#374151' }}
                  contentStyle={{ borderRadius: '12px', border: '1px solid #e5e7eb', boxShadow: '0 4px 12px rgba(0,0,0,0.08)', fontSize: '12px' }}
                  cursor={{ fill: 'rgba(0,0,0,0.03)', radius: 6 }}
                />
                <Bar dataKey="income" name="收入" fill="#34d399" radius={[4, 4, 0, 0]} />
                <Bar dataKey="expense" name="支出" fill="#f87171" radius={[4, 4, 0, 0]} />
              </BarChart>
            </ResponsiveContainer>
          </div>
        )}
      </div>

      {/* Income vs expense overview pie */}
      {monthly.length > 0 && totalIncome + totalExpense > 0 && (
        <div className="bg-white rounded-2xl shadow-sm border border-gray-100 p-5">
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
                    <Cell fill="#34d399" />
                    <Cell fill="#f87171" />
                  </Pie>
                  <Tooltip formatter={(value, name) => [fmt(value as number), name]} />
                </PieChart>
              </ResponsiveContainer>
            </div>
            <div className="space-y-3 flex-1">
              <div>
                <div className="flex items-center justify-between text-sm mb-1">
                  <span className="flex items-center gap-2 text-gray-500 font-medium">
                    <span className="w-2.5 h-2.5 rounded-full bg-emerald-400 shrink-0" />收入
                  </span>
                  <span className="font-bold text-emerald-600 tabular-nums">{fmt(totalIncome)}</span>
                </div>
                <div className="h-1.5 bg-gray-100 rounded-full overflow-hidden">
                  <div className="h-full bg-emerald-400 rounded-full" style={{ width: `${totalIncome + totalExpense > 0 ? Math.round(totalIncome / (totalIncome + totalExpense) * 100) : 0}%` }} />
                </div>
              </div>
              <div>
                <div className="flex items-center justify-between text-sm mb-1">
                  <span className="flex items-center gap-2 text-gray-500 font-medium">
                    <span className="w-2.5 h-2.5 rounded-full bg-red-400 shrink-0" />支出
                  </span>
                  <span className="font-bold text-red-500 tabular-nums">{fmt(totalExpense)}</span>
                </div>
                <div className="h-1.5 bg-gray-100 rounded-full overflow-hidden">
                  <div className="h-full bg-red-400 rounded-full" style={{ width: `${totalIncome + totalExpense > 0 ? Math.round(totalExpense / (totalIncome + totalExpense) * 100) : 0}%` }} />
                </div>
              </div>
              {totalReimbursed > 0 && (
                <div className="pt-1 border-t border-gray-100">
                  <div className="flex items-center justify-between text-xs text-gray-400">
                    <span>已报销抵扣</span>
                    <span className="font-semibold text-blue-500 tabular-nums">+{fmt(totalReimbursed)}</span>
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>
      )}

      {/* Category pie chart */}
      <div className="bg-white rounded-2xl shadow-sm border border-gray-100 p-5">
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
                  <Tooltip formatter={(value, name) => [fmt(value as number), name]} />
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

      {/* Project breakdown */}
      <div className="bg-white rounded-2xl shadow-sm border border-gray-100 overflow-hidden">
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
                    <span className="text-green-600 font-medium tabular-nums">{fmt(p.income)}</span>
                  </td>
                  <td className="px-5 py-3.5 text-right">
                    <span className="text-red-500 font-medium tabular-nums">{fmt(p.expense)}</span>
                  </td>
                  <td className="px-5 py-3.5 text-right">
                    <span className={`font-bold tabular-nums px-2 py-0.5 rounded-lg text-xs ${
                      p.net >= 0 ? 'bg-green-50 text-green-700' : 'bg-red-50 text-red-600'
                    }`}>
                      {p.net >= 0 ? '+' : ''}{fmt(p.net)}
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
