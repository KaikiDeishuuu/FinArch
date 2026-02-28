import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { listTransactions } from '../api/client'
import type { Transaction } from '../api/client'
import { useAuth } from '../contexts/AuthContext'
import { useExchangeRates } from '../contexts/ExchangeRateContext'
import { toCNY, formatAmount } from '../utils/format'

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
  const { rates, rateDate, loading: ratesLoading } = useExchangeRates()
  const [transactions, setTransactions] = useState<Transaction[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    listTransactions()
      .then((txs) => setTransactions(txs ?? []))
      .catch((err) => {
        const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
        setError(msg || '数据加载失败，请刷新页面重试')
      })
      .finally(() => setLoading(false))
  }, [])

  const companyBalance = useMemo(() =>
    transactions.reduce((s, t) => {
      if (t.source !== 'company') return s
      const cny = toCNY(t.amount_yuan, t.currency || 'CNY', rates)
      return s + (t.direction === 'income' ? cny : -cny)
    }, 0),
    [transactions, rates]
  )

  const personalOutstanding = useMemo(() =>
    transactions
      .filter(t => t.source === 'personal' && t.direction === 'expense' && !t.reimbursed)
      .reduce((s, t) => s + toCNY(t.amount_yuan, t.currency || 'CNY', rates), 0),
    [transactions, rates]
  )

  const fmt = (n: number) => formatAmount(n, 'CNY')

  const pendingTxs = transactions.filter(t => t.source === 'personal' && !t.reimbursed)
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
          <h1 className="text-2xl font-bold text-gray-900 tracking-tight truncate">你好，{user?.username || user?.email?.split('@')[0]}</h1>
          <div className="flex items-center gap-2 mt-1 flex-wrap">
            <p className="text-sm text-gray-400">FinArch · {new Date().toLocaleDateString('zh-CN', { year: 'numeric', month: 'long', day: 'numeric' })}</p>
            {ratesLoading
              ? <span className="text-[10px] px-1.5 py-0.5 rounded-full bg-gray-100 text-gray-400">汇率加载中…</span>
              : rateDate
                ? <span className="text-[10px] px-1.5 py-0.5 rounded-full bg-emerald-50 text-emerald-600 font-medium">实时汇率 · $ {rates.USD?.toFixed(2)} · € {rates.EUR?.toFixed(2)} · {rateDate}</span>
                : <span className="text-[10px] px-1.5 py-0.5 rounded-full bg-amber-50 text-amber-600 font-medium">备用汇率 · $ {rates.USD?.toFixed(2)} · € {rates.EUR?.toFixed(2)}</span>
            }
          </div>
        </div>
        <Link
          to="/add"
          className="shrink-0 bg-blue-600 hover:bg-blue-700 active:bg-blue-800 text-white text-sm font-semibold px-4 py-2.5 rounded-xl transition-colors shadow-sm shadow-blue-200"
        >
          + 添加
        </Link>
      </div>

      {/* Balance cards */}
      <div className="grid grid-cols-2 gap-4">
        <div className="relative bg-white rounded-2xl border border-gray-100 shadow-sm p-5 overflow-hidden">
          <div className="absolute inset-0 bg-gradient-to-br from-emerald-50 to-green-100/60 pointer-events-none" />
          <div className="relative">
            <div className="flex items-center gap-2 mb-3">
              <div className="w-7 h-7 rounded-lg bg-emerald-500 flex items-center justify-center shrink-0">
                <svg viewBox="0 0 24 24" fill="none" stroke="white" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-4 h-4"><rect x="2" y="7" width="20" height="14" rx="2"/><path d="M16 7V5a2 2 0 00-4 0v2"/><line x1="12" y1="12" x2="12" y2="16"/><line x1="10" y1="14" x2="14" y2="14"/></svg>
              </div>
              <p className="text-xs font-semibold text-emerald-700 uppercase tracking-wider">公司账户</p>
            </div>
            <p className="text-xl md:text-2xl font-bold text-emerald-700 leading-tight tabular-nums break-all">{fmt(companyBalance)}</p>
            <p className="text-xs text-emerald-500/80 mt-1.5">当前可用资金</p>
          </div>
        </div>
        <div className="relative bg-white rounded-2xl border border-gray-100 shadow-sm p-5 overflow-hidden">
          <div className="absolute inset-0 bg-gradient-to-br from-amber-50 to-orange-100/60 pointer-events-none" />
          <div className="relative">
            <div className="flex items-center gap-2 mb-3">
              <div className="w-7 h-7 rounded-lg bg-amber-500 flex items-center justify-center shrink-0">
                <svg viewBox="0 0 24 24" fill="none" stroke="white" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-4 h-4"><circle cx="12" cy="12" r="10"/><path d="M9 12l2 2 4-4"/></svg>
              </div>
              <p className="text-xs font-semibold text-amber-700 uppercase tracking-wider">待报销</p>
            </div>
            <p className="text-xl md:text-2xl font-bold text-amber-700 leading-tight tabular-nums break-all">{fmt(personalOutstanding)}</p>
            <p className="text-xs text-amber-500/80 mt-1.5">个人垫付未报销合计</p>
          </div>
        </div>
      </div>

      {/* Pending action hints */}
      {(notUploaded.length > 0 || uploadedNotReimbursed.length > 0) && (
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-5">
          <h2 className="text-xs font-semibold text-gray-400 mb-3 uppercase tracking-wider">待处理事项</h2>
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
        <h2 className="text-xs font-semibold text-gray-400 mb-4 uppercase tracking-wider">功能导航</h2>
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
        <h2 className="text-xs font-semibold text-gray-400 mb-4 uppercase tracking-wider">使用流程</h2>
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
