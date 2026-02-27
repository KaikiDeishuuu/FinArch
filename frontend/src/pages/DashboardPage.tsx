import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { getStatsSummary, listTransactions } from '../api/client'
import type { PoolBalance, Transaction } from '../api/client'
import { useAuth } from '../contexts/AuthContext'

const FEATURES = [
  {
    to: '/transactions',
    icon: '📋',
    title: '交易明细',
    desc: '查看、筛选所有收支记录，标记上传与报销状态',
    color: 'bg-blue-50 border-blue-100',
    iconBg: 'bg-blue-100 text-blue-600',
  },
  {
    to: '/add',
    icon: '➕',
    title: '添加交易',
    desc: '新增一笔收入或支出，支持多类别与项目归属',
    color: 'bg-green-50 border-green-100',
    iconBg: 'bg-green-100 text-green-600',
  },
  {
    to: '/match',
    icon: '🔍',
    title: '子集匹配',
    desc: '输入报销总额，自动寻找与之精确匹配的交易组合',
    color: 'bg-purple-50 border-purple-100',
    iconBg: 'bg-purple-100 text-purple-600',
  },
  {
    to: '/stats',
    icon: '📈',
    title: '统计分析',
    desc: '月度趋势、分类支出和项目汇总可视化报告',
    color: 'bg-orange-50 border-orange-100',
    iconBg: 'bg-orange-100 text-orange-600',
  },
]

export default function DashboardPage() {
  const { user } = useAuth()
  const [balance, setBalance] = useState<PoolBalance | null>(null)
  const [pendingTxs, setPendingTxs] = useState<Transaction[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    Promise.all([getStatsSummary(), listTransactions()])
      .then(([bal, txs]) => {
        setBalance(bal)
        // 未上传 or 已上传未报销的个人垫付
        const pending = (txs ?? []).filter(
          (t) => t.source === 'personal' && !t.reimbursed
        )
        setPendingTxs(pending)
      })
      .catch((err) => {
        const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
        setError(msg || '数据加载失败，请刷新页面重试')
      })
      .finally(() => setLoading(false))
  }, [])

  const fmt = (n: number) => `¥${n.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}`

  const notUploaded = pendingTxs.filter((t) => !t.uploaded)
  const uploadedNotReimbursed = pendingTxs.filter((t) => t.uploaded && !t.reimbursed)

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="w-8 h-8 border-4 border-blue-500 border-t-transparent rounded-full animate-spin" />
      </div>
    )
  }

  if (error) {
    return (
      <div className="bg-red-50 border border-red-200 text-red-700 rounded-xl p-6 text-sm">
        <p className="font-semibold mb-1">加载失败</p>
        <p>{error}</p>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h1 className="text-xl md:text-2xl font-bold text-gray-800 truncate">你好，{user?.name || user?.email?.split('@')[0]} 👋</h1>
          <p className="text-sm text-gray-400 mt-1">科研经费管理系统 · {new Date().toLocaleDateString('zh-CN')}</p>
        </div>
        <Link
          to="/add"
          className="shrink-0 bg-blue-600 hover:bg-blue-700 text-white text-sm font-semibold px-4 py-2.5 rounded-xl transition-colors shadow-sm"
        >
          + 添加
        </Link>
      </div>

      {/* Balance cards */}
      <div className="grid grid-cols-2 gap-4">
        <div className="bg-gradient-to-br from-green-500 to-emerald-600 rounded-2xl p-5 text-white shadow-sm">
          <p className="text-xs font-semibold uppercase tracking-wider opacity-75 mb-2">公司账户余额</p>
          <p className="text-3xl font-bold">{balance ? fmt(balance.company_balance) : '—'}</p>
          <p className="text-xs opacity-60 mt-2">当前可用资金</p>
        </div>
        <div className="bg-gradient-to-br from-amber-500 to-orange-500 rounded-2xl p-5 text-white shadow-sm">
          <p className="text-xs font-semibold uppercase tracking-wider opacity-75 mb-2">个人待报销</p>
          <p className="text-3xl font-bold">{balance ? fmt(balance.personal_outstanding) : '—'}</p>
          <p className="text-xs opacity-60 mt-2">个人垫付未报销合计</p>
        </div>
      </div>

      {/* Pending action hints */}
      {(notUploaded.length > 0 || uploadedNotReimbursed.length > 0) && (
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-5">
          <h2 className="text-sm font-semibold text-gray-600 mb-3 uppercase tracking-wider">待处理事项</h2>
          <div className="space-y-2">
            {notUploaded.length > 0 && (
              <Link to="/transactions" className="flex items-center gap-3 p-3 rounded-xl bg-amber-50 border border-amber-100 hover:border-amber-300 transition-colors">
                <span className="w-8 h-8 rounded-lg bg-amber-100 flex items-center justify-center text-base shrink-0">⬆️</span>
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium text-amber-800">有 {notUploaded.length} 笔垫付尚未上传系统</p>
                  <p className="text-xs text-amber-600 mt-0.5">上传后才可标记报销 → 点击前往交易明细</p>
                </div>
                <svg className="w-4 h-4 text-amber-400 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" /></svg>
              </Link>
            )}
            {uploadedNotReimbursed.length > 0 && (
              <Link to="/match" className="flex items-center gap-3 p-3 rounded-xl bg-blue-50 border border-blue-100 hover:border-blue-300 transition-colors">
                <span className="w-8 h-8 rounded-lg bg-blue-100 flex items-center justify-center text-base shrink-0">🔍</span>
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium text-blue-800">有 {uploadedNotReimbursed.length} 笔已上传等待报销</p>
                  <p className="text-xs text-blue-600 mt-0.5">使用子集匹配找到报销组合 → 点击前往匹配</p>
                </div>
                <svg className="w-4 h-4 text-blue-400 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" /></svg>
              </Link>
            )}
          </div>
        </div>
      )}

      {/* Feature guide */}
      <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-5">
        <h2 className="text-sm font-semibold text-gray-600 mb-4 uppercase tracking-wider">功能导航</h2>
        <div className="grid grid-cols-2 gap-3">
          {FEATURES.map((f) => (
            <Link
              key={f.to}
              to={f.to}
              className={`flex items-start gap-3 p-3.5 rounded-xl border transition-all hover:shadow-sm ${f.color}`}
            >
              <span className={`w-9 h-9 rounded-xl flex items-center justify-center text-lg shrink-0 ${f.iconBg}`}>
                {f.icon}
              </span>
              <div className="min-w-0">
                <p className="text-sm font-semibold text-gray-700">{f.title}</p>
                <p className="text-xs text-gray-500 mt-0.5 leading-relaxed">{f.desc}</p>
              </div>
            </Link>
          ))}
        </div>
      </div>

      {/* Workflow guide */}
      <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-5">
        <h2 className="text-sm font-semibold text-gray-600 mb-4 uppercase tracking-wider">使用流程</h2>
        {/* Mobile: 2-column grid */}
        <div className="grid grid-cols-2 gap-3 md:hidden">
          {[
            { step: '1', icon: '✍️', title: '记录垫付', desc: '添加个人垫付的支出，填写类别和项目' },
            { step: '2', icon: '⬆️', title: '标记上传', desc: '在交易明细中标记已上传到报销系统' },
            { step: '3', icon: '🔍', title: '子集匹配', desc: '输入报销单总额，自动找到对应交易组合' },
            { step: '4', icon: '✅', title: '完成报销', desc: '确认报销完成，标记相关交易为已报销' },
          ].map((s) => (
            <div key={s.step} className="bg-gray-50 rounded-xl p-3 flex items-start gap-2.5">
              <div className="w-6 h-6 rounded-full bg-blue-600 text-white text-xs font-bold flex items-center justify-center shrink-0 mt-0.5">{s.step}</div>
              <div>
                <p className="text-base leading-none mb-1">{s.icon}</p>
                <p className="text-xs font-semibold text-gray-700">{s.title}</p>
                <p className="text-[11px] text-gray-400 mt-0.5 leading-relaxed">{s.desc}</p>
              </div>
            </div>
          ))}
        </div>
        {/* Desktop: horizontal with connecting lines */}
        <div className="hidden md:flex items-start gap-0">
          {[
            { step: '1', icon: '✍️', title: '记录垫付', desc: '添加个人垫付的支出，填写类别和项目' },
            { step: '2', icon: '⬆️', title: '标记上传', desc: '在交易明细中标记已上传到报销系统' },
            { step: '3', icon: '🔍', title: '子集匹配', desc: '输入报销单总额，自动找到对应交易组合' },
            { step: '4', icon: '✅', title: '完成报销', desc: '确认报销完成，标记相关交易为已报销' },
          ].map((s, i, arr) => (
            <div key={s.step} className="flex items-start flex-1">
              <div className="flex flex-col items-center">
                <div className="w-8 h-8 rounded-full bg-blue-600 text-white text-xs font-bold flex items-center justify-center shrink-0">{s.step}</div>
                <div className="text-center mt-2 px-1">
                  <p className="text-base">{s.icon}</p>
                  <p className="text-xs font-semibold text-gray-700 mt-1">{s.title}</p>
                  <p className="text-[11px] text-gray-400 mt-0.5 leading-relaxed">{s.desc}</p>
                </div>
              </div>
              {i < arr.length - 1 && (
                <div className="flex-1 h-px bg-gray-200 mt-4 mx-1" />
              )}
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
