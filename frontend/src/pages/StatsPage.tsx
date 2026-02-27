import { useEffect, useState } from 'react'
import { getStatsMonthly, getStatsByCategory, getStatsByProject } from '../api/client'
import type { MonthlyStat, CategoryStat, ProjectStat } from '../api/client'

function Bar({ value, max, color, label }: { value: number; max: number; color: string; label?: string }) {
  const pct = max > 0 ? Math.max(Math.round((value / max) * 100), value > 0 ? 2 : 0) : 0
  return (
    <div className="flex items-center gap-2">
      <div className="flex-1 bg-gray-100 rounded-full h-2.5 overflow-hidden">
        <div
          className={`h-2.5 rounded-full transition-all duration-500 ${color}`}
          style={{ width: `${pct}%` }}
        />
      </div>
      {label && <span className="text-xs tabular-nums text-gray-400 w-8 text-right">{pct}%</span>}
    </div>
  )
}

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
  const maxCat = Math.max(...categories.map(c => c.total), 1)

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

      {/* Monthly trend */}
      <div className="bg-white rounded-2xl shadow-sm border border-gray-100 p-5">
        <div className="flex items-center justify-between mb-4">
          <h2 className="font-semibold text-gray-700">{year} 年月度趋势</h2>
          <div className="flex items-center gap-3 text-xs text-gray-400">
            <span className="flex items-center gap-1.5"><span className="w-2.5 h-2.5 rounded-full bg-green-400 inline-block" />收入</span>
            <span className="flex items-center gap-1.5"><span className="w-2.5 h-2.5 rounded-full bg-red-400 inline-block" />支出</span>
          </div>
        </div>
        {monthly.length === 0 ? (
          <p className="text-sm text-gray-400 text-center py-8">暂无数据</p>
        ) : (
          <div className="space-y-2.5">
            {monthly.map((m) => (
              <div key={`${m.year}-${m.month}`} className="grid grid-cols-[3rem_1fr_6rem] gap-3 items-center">
                <span className="text-xs font-medium text-gray-500 text-right">{monthNames[m.month - 1]}</span>
                <div className="space-y-1.5">
                  <Bar value={m.income} max={maxIncome} color="bg-gradient-to-r from-green-300 to-green-500" />
                  <Bar value={m.expense} max={maxExpense} color="bg-gradient-to-r from-red-300 to-red-500" />
                </div>
                <div className="text-right space-y-1">
                  <p className="text-xs text-green-600 tabular-nums">{fmtShort(m.income)}</p>
                  <p className="text-xs text-red-500 tabular-nums">{fmtShort(m.expense)}</p>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Category breakdown */}
      <div className="bg-white rounded-2xl shadow-sm border border-gray-100 p-5">
        <h2 className="font-semibold text-gray-700 mb-4">分类支出</h2>
        {categories.length === 0 ? (
          <p className="text-sm text-gray-400 text-center py-8">暂无数据</p>
        ) : (
          <div className="space-y-3.5">
            {categories.map((c, idx) => (
              <div key={c.category}>
                <div className="flex justify-between items-baseline mb-1.5">
                  <div className="flex items-center gap-2">
                    <span className="text-xs text-gray-400 w-4 text-right">{idx + 1}</span>
                    <span className="text-sm font-medium text-gray-700">{c.category}</span>
                  </div>
                  <div className="text-right">
                    <span className="text-sm font-semibold text-gray-700">{fmt(c.total)}</span>
                    <span className="text-xs text-gray-400 ml-1.5">{c.count} 笔</span>
                  </div>
                </div>
                <Bar value={c.total} max={maxCat} color="bg-gradient-to-r from-blue-300 to-blue-500" label="pct" />
              </div>
            ))}
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
