import { useEffect, useState } from 'react'
import { PieChart, Pie, Cell, Tooltip, ResponsiveContainer } from 'recharts'
import { getStatsMonthly, getStatsByCategory, getStatsByProject } from '../api/client'
import type { MonthlyStat, CategoryStat, ProjectStat } from '../api/client'

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

  const fmt = (n: number) => `¥${n.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}`
  const fmtShort = (n: number) => {
    if (n >= 10000) return `¥${(n / 10000).toFixed(1)}万`
    return `¥${n.toLocaleString('zh-CN', { maximumFractionDigits: 0 })}`
  }
  const monthNames = ['1月','2月','3月','4月','5月','6月','7月','8月','9月','10月','11月','12月']

  const totalIncome = monthly.reduce((s, m) => s + m.income, 0)
  const totalExpense = monthly.reduce((s, m) => s + m.expense, 0)
  const totalNet = totalIncome - totalExpense

  const maxExpense = Math.max(...monthly.map(m => m.expense), 1)
  const maxIncome = Math.max(...monthly.map(m => m.income), 1)

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="w-8 h-8 border-4 border-blue-500 border-t-transparent rounded-full animate-spin" />
      </div>
    )
  }

  return (
    <div className="space-y-5">
      <div>
        <h1 className="text-2xl font-bold text-gray-800">统计分析</h1>
        <p className="text-sm text-gray-400 mt-1">{year} 年度资金概览</p>
      </div>

      {/* Summary cards */}
      <div className="grid grid-cols-3 gap-3">
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-4 flex items-stretch gap-3 overflow-hidden">
          <div className="w-1 rounded-full bg-green-400 shrink-0" />
          <div>
            <p className="text-xs text-gray-400 uppercase tracking-wider mb-1">年度收入</p>
            <p className="text-xl font-bold text-green-600">{fmtShort(totalIncome)}</p>
          </div>
        </div>
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-4 flex items-stretch gap-3 overflow-hidden">
          <div className="w-1 rounded-full bg-red-400 shrink-0" />
          <div>
            <p className="text-xs text-gray-400 uppercase tracking-wider mb-1">年度支出</p>
            <p className="text-xl font-bold text-red-500">{fmtShort(totalExpense)}</p>
          </div>
        </div>
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-4 flex items-stretch gap-3 overflow-hidden">
          <div className={`w-1 rounded-full shrink-0 ${totalNet >= 0 ? 'bg-blue-400' : 'bg-orange-400'}`} />
          <div>
            <p className="text-xs text-gray-400 uppercase tracking-wider mb-1">净结余</p>
            <p className={`text-xl font-bold ${totalNet >= 0 ? 'text-blue-600' : 'text-orange-500'}`}>
              {totalNet >= 0 ? '+' : ''}{fmtShort(totalNet)}
            </p>
          </div>
        </div>
      </div>

      {/* Monthly pie - income vs expense */}
      <div className="bg-white rounded-2xl shadow-sm border border-gray-100 p-5">
        <h2 className="font-semibold text-gray-700 mb-4">{year} 年收支概况</h2>
        {monthly.length === 0 ? (
          <p className="text-sm text-gray-400 text-center py-8">暂无数据</p>
        ) : (
          <div className="flex flex-col md:flex-row gap-6 items-start">
            {/* Pie + legend */}
            <div className="shrink-0 flex flex-col items-center gap-3 w-full md:w-auto">
              <div className="w-48 h-48">
                <ResponsiveContainer width="100%" height="100%">
                  <PieChart>
                    <Pie
                      data={[
                        { name: '收入', value: totalIncome },
                        { name: '支出', value: totalExpense },
                      ]}
                      dataKey="value"
                      nameKey="name"
                      cx="50%" cy="50%"
                      innerRadius={46}
                      outerRadius={72}
                      paddingAngle={3}
                    >
                      <Cell fill="#10b981" />
                      <Cell fill="#ef4444" />
                    </Pie>
                    <Tooltip formatter={(value, name) => [fmt(value as number), name]} />
                  </PieChart>
                </ResponsiveContainer>
              </div>
              {/* color legend */}
              <div className="flex gap-5">
                <div className="flex items-center gap-1.5">
                  <span className="w-2.5 h-2.5 rounded-full bg-emerald-500 shrink-0" />
                  <span className="text-xs text-gray-500">收入</span>
                  <span className="text-xs font-bold text-emerald-600 tabular-nums ml-1">{fmtShort(totalIncome)}</span>
                </div>
                <div className="flex items-center gap-1.5">
                  <span className="w-2.5 h-2.5 rounded-full bg-red-400 shrink-0" />
                  <span className="text-xs text-gray-500">支出</span>
                  <span className="text-xs font-bold text-red-500 tabular-nums ml-1">{fmtShort(totalExpense)}</span>
                </div>
              </div>
            </div>

            {/* Monthly bar chart */}
            <div className="flex-1 min-w-0 w-full">
              {/* legend */}
              <div className="flex items-center gap-4 mb-3 text-xs text-gray-400">
                <span className="flex items-center gap-1.5">
                  <span className="w-2.5 h-2 rounded-sm bg-green-400 inline-block" />收入
                </span>
                <span className="flex items-center gap-1.5">
                  <span className="w-2.5 h-2 rounded-sm bg-red-400 inline-block" />支出
                </span>
              </div>
              <div className="relative">
                <div className="space-y-3 max-h-52 overflow-y-auto pr-1 scrollbar-thin scrollbar-thumb-gray-200 scrollbar-track-transparent">
                  {monthly.map((m) => {
                    const incPct = maxIncome > 0 ? Math.max(Math.round((m.income / maxIncome) * 100), m.income > 0 ? 3 : 0) : 0
                    const expPct = maxExpense > 0 ? Math.max(Math.round((m.expense / maxExpense) * 100), m.expense > 0 ? 3 : 0) : 0
                    return (
                      <div key={`${m.year}-${m.month}`} className="flex items-center gap-2 min-w-0">
                        <span className="text-xs font-medium text-gray-400 w-7 shrink-0 text-right">{monthNames[m.month - 1]}</span>
                        <div className="flex-1 min-w-0 space-y-1.5">
                          <div className="flex items-center gap-1.5">
                            <div className="flex-1 bg-gray-100 rounded-full h-2.5 overflow-hidden">
                              <div className="h-2.5 rounded-full bg-gradient-to-r from-green-300 to-emerald-500 transition-all duration-500" style={{ width: `${incPct}%` }} />
                            </div>
                            <span className="text-xs text-emerald-600 tabular-nums shrink-0 w-16 text-right">{fmtShort(m.income)}</span>
                          </div>
                          <div className="flex items-center gap-1.5">
                            <div className="flex-1 bg-gray-100 rounded-full h-2.5 overflow-hidden">
                              <div className="h-2.5 rounded-full bg-gradient-to-r from-red-300 to-red-500 transition-all duration-500" style={{ width: `${expPct}%` }} />
                            </div>
                            <span className="text-xs text-red-500 tabular-nums shrink-0 w-16 text-right">{fmtShort(m.expense)}</span>
                          </div>
                        </div>
                      </div>
                    )
                  })}
                </div>
                {monthly.length > 6 && (
                  <div className="pointer-events-none absolute bottom-0 left-0 right-0 h-8 bg-gradient-to-t from-white to-transparent rounded-b" />
                )}
              </div>
            </div>
          </div>
        )}
      </div>

      {/* Category pie chart */}
      <div className="bg-white rounded-2xl shadow-sm border border-gray-100 p-5">
        <h2 className="font-semibold text-gray-700 mb-4">分类支出</h2>
        {categories.length === 0 ? (
          <p className="text-sm text-gray-400 text-center py-8">暂无数据</p>
        ) : (
          <div className="flex flex-col md:flex-row gap-6 items-start">
            <div className="w-full md:w-64 h-56 shrink-0">
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
          <h2 className="font-semibold text-gray-700">项目汇总</h2>
          <span className="text-xs text-gray-400">{projects.length} 个项目</span>
        </div>
        {projects.length === 0 ? (
          <p className="text-sm text-gray-400 text-center py-8">暂无数据</p>
        ) : (
          <table className="w-full text-sm">
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
        )}
      </div>
    </div>
  )
}
