import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { getStatsSummary, listTransactions } from '../api/client'
import type { PoolBalance, Transaction } from '../api/client'
import { useAuth } from '../contexts/AuthContext'

const IconList = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-5 h-5">
    <path d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01" />
  </svg>
)
const IconPlus = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" className="w-5 h-5">
    <path d="M12 5v14M5 12h14" />
  </svg>
)
const IconSearch = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-5 h-5">
    <circle cx="11" cy="11" r="8" /><path d="M21 21l-4.35-4.35" />
  </svg>
)
const IconChart = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-5 h-5">
    <path d="M18 20V10M12 20V4M6 20v-6" />
  </svg>
)
const IconUpload = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-5 h-5">
    <path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4" />
    <polyline points="17 8 12 3 7 8" />
    <line x1="12" y1="3" x2="12" y2="15" />
  </svg>
)
const IconPen = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-5 h-5">
    <path d="M11 4H4a2 2 0 00-2 2v14a2 2 0 002 2h14a2 2 0 002-2v-7" />
    <path d="M18.5 2.5a2.121 2.121 0 013 3L12 15l-4 1 1-4 9.5-9.5z" />
  </svg>
)
const IconCheck = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.5} strokeLinecap="round" strokeLinejoin="round" className="w-5 h-5">
    <polyline points="20 6 9 17 4 12" />
  </svg>
)

const FEATURES = [
  {
    to: '/transactions',
    Icon: IconList,
    title: '交易明细',
    desc: '查看、筛选所有收支记录，标记上传与报销状态',
    color: 'bg-blue-50 border-blue-100',
    iconBg: 'bg-blue-100 text-blue-600',
  },
  {
    to: '/add',
    Icon: IconPlus,
    title: '添加交易',
    desc: '新增一笔收入或支出，支持多类别与项目归属',
    color: 'bg-green-50 border-green-100',
    iconBg: 'bg-green-100 text-green-600',
  },
  {
    to: '/match',
    Icon: IconSearch,
    title: '子集匹配',
    desc: '输入报销总额，自动寻找与之精确匹配的交易组合',
    color: 'bg-purple-50 border-purple-100',
    iconBg: 'bg-purple-100 text-purple-600',
  },
  {
    to: '/stats',
    Icon: IconChart,
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
          <h1 className="text-xl md:text-2xl font-bold text-gray-800 truncate">你好，{user?.username || user?.email?.split('@')[0]}</h1>
          <p className="text-sm text-gray-400 mt-1">FinArch · {new Date().toLocaleDateString('zh-CN')}</p>
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
          <p className="text-xl md:text-3xl font-bold leading-tight tabular-nums break-all">{balance ? fmt(balance.company_balance) : '—'}</p>
          <p className="text-xs opacity-60 mt-2">当前可用资金</p>
        </div>
        <div className="bg-gradient-to-br from-amber-500 to-orange-500 rounded-2xl p-5 text-white shadow-sm">
          <p className="text-xs font-semibold uppercase tracking-wider opacity-75 mb-2">个人待报销</p>
          <p className="text-xl md:text-3xl font-bold leading-tight tabular-nums break-all">{balance ? fmt(balance.personal_outstanding) : '—'}</p>
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
                <span className="w-8 h-8 rounded-lg bg-amber-100 text-amber-600 flex items-center justify-center shrink-0"><IconUpload /></span>
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium text-amber-800">有 {notUploaded.length} 笔垫付尚未上传系统</p>
                  <p className="text-xs text-amber-600 mt-0.5">上传后才可标记报销 → 点击前往交易明细</p>
                </div>
                <svg className="w-4 h-4 text-amber-400 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" /></svg>
              </Link>
            )}
            {uploadedNotReimbursed.length > 0 && (
              <Link to="/match" className="flex items-center gap-3 p-3 rounded-xl bg-blue-50 border border-blue-100 hover:border-blue-300 transition-colors">
                <span className="w-8 h-8 rounded-lg bg-blue-100 text-blue-600 flex items-center justify-center shrink-0"><IconSearch /></span>
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
              <span className={`w-9 h-9 rounded-xl flex items-center justify-center shrink-0 ${f.iconBg}`}>
                <f.Icon />
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
        {(() => {
          const steps = [
            { step: '1', Icon: IconPen,    title: '记录垫付', desc: '添加个人垫付的支出，填写类别和项目' },
            { step: '2', Icon: IconUpload, title: '标记上传', desc: '在交易明细中标记已上传到报销系统' },
            { step: '3', Icon: IconSearch, title: '子集匹配', desc: '输入报销单总额，自动找到对应交易组合' },
            { step: '4', Icon: IconCheck,  title: '完成报销', desc: '确认报销完成，标记相关交易为已报销' },
          ]
          return (
            <>
              {/* Mobile: 2-column grid */}
              <div className="grid grid-cols-2 gap-3 md:hidden">
                {steps.map((s) => (
                  <div key={s.step} className="bg-gray-50 rounded-xl p-3 flex items-start gap-2.5">
                    <div className="w-6 h-6 rounded-full bg-blue-600 text-white text-xs font-bold flex items-center justify-center shrink-0 mt-0.5">{s.step}</div>
                    <div>
                      <div className="text-gray-400 mb-1"><s.Icon /></div>
                      <p className="text-xs font-semibold text-gray-700">{s.title}</p>
                      <p className="text-[11px] text-gray-400 mt-0.5 leading-relaxed">{s.desc}</p>
                    </div>
                  </div>
                ))}
              </div>
              {/* Desktop: horizontal with connecting lines */}
              <div className="hidden md:flex items-start gap-0">
                {steps.map((s, i, arr) => (
                  <div key={s.step} className="flex items-start flex-1">
                    <div className="flex flex-col items-center">
                      <div className="w-8 h-8 rounded-full bg-blue-600 text-white text-xs font-bold flex items-center justify-center shrink-0">{s.step}</div>
                      <div className="text-center mt-2 px-1">
                        <div className="flex justify-center text-gray-400"><s.Icon /></div>
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
            </>
          )
        })()}
      </div>
    </div>
  )
}
