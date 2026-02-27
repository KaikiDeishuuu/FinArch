import { useEffect, useState, useCallback } from 'react'
import { Link } from 'react-router-dom'
import { listTransactions, toggleReimbursed, toggleUploaded } from '../api/client'
import type { Transaction } from '../api/client'
import { useAuth } from '../contexts/AuthContext'
import { exportTransactionsPDF } from '../utils/exportTransactionsPDF'

type FilterTab = 'all' | 'unreimbursed' | 'reimbursed'

function StatusBadge({
  active, activeLabel, inactiveLabel, activeClass, inactiveClass,
  onClick, disabled, loading,
}: {
  active: boolean; activeLabel: string; inactiveLabel: string
  activeClass: string; inactiveClass: string
  onClick: () => void; disabled?: boolean; loading?: boolean
}) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      className={`inline-flex items-center gap-1 px-3 py-1.5 rounded-full text-xs font-bold whitespace-nowrap transition-all disabled:opacity-40 disabled:cursor-not-allowed ${active ? activeClass : inactiveClass}`}
    >
      {loading ? (
        <span className="w-3 h-3 border-2 border-current border-t-transparent rounded-full animate-spin" />
      ) : active ? (
        <svg className="w-3 h-3" fill="currentColor" viewBox="0 0 20 20"><path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" /></svg>
      ) : (
        <span className="w-3 h-3 rounded-full border-2 border-current opacity-50" />
      )}
      {loading ? '…' : active ? activeLabel : inactiveLabel}
    </button>
  )
}

export default function TransactionsPage() {
  const { user } = useAuth()
  const [txs, setTxs] = useState<Transaction[]>([])
  const [filter, setFilter] = useState<FilterTab>('all')
  const [loading, setLoading] = useState(true)
  const [copiedId, setCopiedId] = useState<string | null>(null)
  const [togglingId, setTogglingId] = useState<string | null>(null)
  const [toggleError, setToggleError] = useState('')

  const load = useCallback(() => {
    setLoading(true)
    listTransactions()
      .then(res => setTxs(res ?? []))
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => { load() }, [load])

  const filtered = txs.filter((t) => {
    if (filter === 'unreimbursed') return !t.reimbursed && t.source === 'personal'
    if (filter === 'reimbursed') return t.reimbursed
    return true
  })

  const fmt = (n: number) => `¥${n.toLocaleString('zh-CN', { minimumFractionDigits: 2 })}`

  function exportPDF() {
    const filterLabel = { all: '全部', unreimbursed: '待报销', reimbursed: '已报销' }[filter]
    exportTransactionsPDF(filtered, filterLabel, user)
  }

  async function handleToggle(id: string) {
    setTogglingId(id)
    try {
      await toggleReimbursed(id)
      load()
    } catch {
      setToggleError('报销状态切换失败')
    } finally {
      setTogglingId(null)
    }
  }

  async function handleToggleUpload(id: string) {
    setTogglingId(id)
    setToggleError('')
    try {
      await toggleUploaded(id)
      load()
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      setToggleError(msg || '上传状态切换失败，请确认后端已重启')
    } finally {
      setTogglingId(null)
    }
  }

  function copyId(id: string) {
    navigator.clipboard.writeText(id).then(() => {
      setCopiedId(id)
      setTimeout(() => setCopiedId(null), 1500)
    })
  }

  const tabs: { key: FilterTab; label: string; count?: number }[] = [
    { key: 'all', label: '全部', count: txs.length },
    { key: 'unreimbursed', label: '待报销', count: txs.filter(t => !t.reimbursed && t.source === 'personal').length },
    { key: 'reimbursed', label: '已报销', count: txs.filter(t => t.reimbursed).length },
  ]

  const totalIncome = filtered.filter(t => t.direction === 'income').reduce((s, t) => s + t.amount_yuan, 0)
  const totalExpense = filtered.filter(t => t.direction === 'expense').reduce((s, t) => s + t.amount_yuan, 0)

  return (
    <div className="space-y-5">
      {/* Header */}
      <div className="flex items-center justify-between gap-3">
        <div className="min-w-0">
          <h1 className="text-xl md:text-2xl font-bold text-gray-800">交易明细</h1>
          <p className="text-sm text-gray-400 mt-0.5 hidden sm:block">管理所有收入与支出记录</p>
        </div>
        <div className="flex items-center gap-2 shrink-0">
          <button
            onClick={exportPDF}
            disabled={loading || filtered.length === 0}
            className="flex items-center gap-1.5 bg-gray-100 hover:bg-gray-200 text-gray-700 text-sm font-semibold px-3.5 py-2.5 rounded-xl transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
            title="导出当前筛选结果为 PDF"
          >
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M12 10v6m0 0l-3-3m3 3l3-3M3 17V7a2 2 0 012-2h6l2 2h4a2 2 0 012 2v8a2 2 0 01-2 2H5a2 2 0 01-2-2z" />
            </svg>
            <span className="hidden sm:inline">导出 PDF</span>
          </button>
          <Link
            to="/add"
            className="shrink-0 bg-blue-600 hover:bg-blue-700 text-white text-sm font-semibold px-4 py-2.5 rounded-xl transition-colors shadow-sm"
          >
            + 添加
          </Link>
        </div>
      </div>

      {/* Summary cards */}
      <div className="grid grid-cols-3 gap-2 md:gap-3">
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-3 md:p-4">
          <p className="text-[10px] md:text-xs text-gray-400 uppercase tracking-wider mb-1 md:mb-2">筛选结果</p>
          <p className="text-lg md:text-2xl font-bold text-gray-700">{filtered.length} <span className="text-sm md:text-base font-normal text-gray-400">笔</span></p>
        </div>
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-3 md:p-4">
          <p className="text-[10px] md:text-xs text-gray-400 uppercase tracking-wider mb-1 md:mb-2">收入</p>
          <p className="text-sm md:text-2xl font-bold text-green-600 tabular-nums truncate">{fmt(totalIncome)}</p>
        </div>
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-3 md:p-4">
          <p className="text-[10px] md:text-xs text-gray-400 uppercase tracking-wider mb-1 md:mb-2">支出</p>
          <p className="text-sm md:text-2xl font-bold text-red-500 tabular-nums truncate">{fmt(totalExpense)}</p>
        </div>
      </div>

      {/* Tabs + error */}
      <div className="flex items-center gap-3 flex-wrap">
        <div className="flex rounded-xl bg-gray-100 p-1 gap-0.5">
          {tabs.map((t) => (
            <button
              key={t.key}
              onClick={() => setFilter(t.key)}
              className={`flex items-center gap-1.5 px-4 py-1.5 rounded-lg text-sm font-medium transition-all ${
                filter === t.key ? 'bg-white shadow text-blue-600' : 'text-gray-500 hover:text-gray-700'
              }`}
            >
              {t.label}
              {t.count !== undefined && (
                <span className={`text-xs rounded-full px-1.5 py-0.5 font-semibold tabular-nums ${
                  filter === t.key ? 'bg-blue-100 text-blue-600' : 'bg-gray-200 text-gray-500'
                }`}>{t.count}</span>
              )}
            </button>
          ))}
        </div>
        {toggleError && (
          <div className="flex-1 bg-red-50 border border-red-200 text-red-700 rounded-xl px-4 py-2 text-sm flex items-center justify-between">
            <span>{toggleError}</span>
            <button onClick={() => setToggleError('')} className="ml-3 text-red-400 hover:text-red-600 text-lg leading-none">×</button>
          </div>
        )}
      </div>

      {/* Mobile card list */}
      <div className="md:hidden space-y-2">
        {loading ? (
          <div className="flex flex-col items-center justify-center py-16 gap-3">
            <div className="w-8 h-8 border-4 border-blue-500 border-t-transparent rounded-full animate-spin" />
            <p className="text-sm text-gray-400">加载中…</p>
          </div>
        ) : filtered.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12 gap-2">
            <span className="text-4xl">📭</span>
            <p className="text-sm text-gray-400">暂无记录</p>
          </div>
        ) : filtered.map((tx) => {
          const done = tx.reimbursed && tx.uploaded
          return (
            <div
              key={tx.id}
              className={`bg-white rounded-2xl border border-gray-100 shadow-sm p-4 transition-opacity ${
                done ? 'opacity-40' : ''
              }`}
            >
              {/* Row 1: category + amount */}
              <div className="flex items-start justify-between gap-2 mb-2.5">
                <div className="flex items-center gap-1.5 min-w-0 pt-0.5">
                  <span className={`w-2.5 h-2.5 rounded-full shrink-0 ${
                    tx.direction === 'income' ? 'bg-green-400' : 'bg-red-400'
                  }`} />
                  <span className="font-semibold text-gray-800 text-sm truncate">{tx.category}</span>
                  {tx.project_id && (
                    <span className="text-xs font-mono bg-gray-100 text-gray-500 px-1.5 py-0.5 rounded shrink-0">
                      {tx.project_id}
                    </span>
                  )}
                </div>
                <span className={`font-bold tabular-nums text-base shrink-0 ml-2 ${
                  tx.direction === 'income' ? 'text-green-600' : 'text-red-500'
                }`}>
                  {tx.direction === 'income' ? '+' : '−'}{fmt(tx.amount_yuan)}
                </span>
              </div>
              {/* Row 2: date + source */}
              <div className="flex items-center gap-2 mb-1.5">
                <span className="text-xs text-gray-400 tabular-nums">{tx.occurred_at}</span>
                <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-semibold ${
                  tx.source === 'company' ? 'bg-blue-50 text-blue-600' : 'bg-amber-50 text-amber-600'
                }`}>
                  {tx.source === 'company' ? '🏢 公司' : '👤 个人'}
                </span>
              </div>
              {/* Row 3: note (if any) */}
              {tx.note && (
                <p className="text-xs text-gray-400 mb-2.5 truncate">{tx.note}</p>
              )}
              {/* Row 4: action badges + copy ID */}
              <div className="flex items-center gap-2 flex-wrap pt-0.5">
                <StatusBadge
                  active={tx.uploaded}
                  activeLabel="已上传"
                  inactiveLabel="未上传"
                  activeClass="bg-purple-100 text-purple-700"
                  inactiveClass="bg-gray-100 text-gray-400"
                  onClick={() => handleToggleUpload(tx.id)}
                  disabled={togglingId === tx.id}
                  loading={togglingId === tx.id}
                />
                {tx.source === 'personal' ? (
                  <StatusBadge
                    active={tx.reimbursed}
                    activeLabel="已报销"
                    inactiveLabel="待报销"
                    activeClass="bg-green-100 text-green-700"
                    inactiveClass="bg-gray-100 text-gray-400"
                    onClick={() => handleToggle(tx.id)}
                    disabled={togglingId === tx.id || !tx.uploaded}
                    loading={togglingId === tx.id}
                  />
                ) : (
                  <span className="text-gray-200 text-sm">—</span>
                )}
                <button
                  onClick={() => copyId(tx.id)}
                  className={`ml-auto font-mono text-xs rounded-lg px-2 py-1 transition-all ${
                    copiedId === tx.id
                      ? 'bg-green-100 text-green-600'
                      : 'bg-gray-100 text-gray-400'
                  }`}
                >
                  {copiedId === tx.id ? '✓ 已复制' : tx.id.slice(0, 8) + '…'}
                </button>
              </div>
            </div>
          )
        })}
      </div>

      {/* Desktop Table */}
      <div className="hidden md:block bg-white rounded-2xl shadow-sm border border-gray-100 overflow-hidden">
        {loading ? (
          <div className="flex flex-col items-center justify-center py-16 gap-3">
            <div className="w-8 h-8 border-4 border-blue-500 border-t-transparent rounded-full animate-spin" />
            <p className="text-sm text-gray-400">加载中…</p>
          </div>
        ) : filtered.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-16 gap-2">
            <span className="text-4xl">📭</span>
            <p className="text-sm text-gray-400">暂无记录</p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="bg-gray-50 border-b-2 border-gray-200">
                  <th className="px-5 py-4 text-left text-sm font-bold text-gray-700">日期</th>
                  <th className="px-5 py-4 text-left text-sm font-bold text-gray-700">类别</th>
                  <th className="px-5 py-4 text-left text-sm font-bold text-gray-700">项目</th>
                  <th className="px-5 py-4 text-left text-sm font-bold text-gray-700">备注</th>
                  <th className="px-5 py-4 text-left text-sm font-bold text-gray-700">来源</th>
                  <th className="px-5 py-4 text-right text-sm font-bold text-gray-700">金额</th>
                  <th className="px-5 py-4 text-center text-sm font-bold text-gray-700">上传</th>
                  <th className="px-5 py-4 text-center text-sm font-bold text-gray-700">报销</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((tx) => {
                  const done = tx.reimbursed && tx.uploaded
                  const urgent = !tx.reimbursed && !tx.uploaded && tx.source === 'personal'
                  return (
                    <tr
                      key={tx.id}
                      className={`border-b border-gray-100 last:border-0 transition-colors ${
                        done ? 'opacity-40' : 'hover:bg-blue-50/30'
                      }`}
                    >
                      <td className="px-5 py-4 whitespace-nowrap">
                        <p className="text-sm font-medium text-gray-700 tabular-nums">{tx.occurred_at}</p>
                        <button
                          onClick={() => copyId(tx.id)}
                          title="点击复制完整 ID"
                          className={`font-mono text-[11px] rounded px-1 py-0.5 transition-all mt-0.5 block ${
                            copiedId === tx.id
                              ? 'bg-green-100 text-green-600'
                              : 'text-gray-300 hover:text-blue-400 hover:bg-blue-50'
                          }`}
                        >
                          {copiedId === tx.id ? '✓ 已复制' : tx.id.slice(0, 8) + '…'}
                        </button>
                      </td>
                      <td className="px-5 py-4">
                        <span className={`inline-flex items-center gap-2 text-sm font-semibold ${urgent ? 'text-gray-900' : 'text-gray-700'}`}>
                          <span className={`w-2.5 h-2.5 rounded-full shrink-0 ${tx.direction === 'income' ? 'bg-green-500' : 'bg-red-500'}`} />
                          {tx.category}
                        </span>
                      </td>
                      <td className="px-5 py-4 max-w-[140px]">
                        {tx.project_id
                          ? <span className="inline-flex items-center text-xs font-mono font-semibold bg-blue-50 text-blue-700 border border-blue-200 px-2 py-0.5 rounded-md">{tx.project_id}</span>
                          : <span className="text-gray-300 text-sm">—</span>
                        }
                      </td>
                      <td className="px-5 py-4 max-w-[180px]">
                        {tx.note
                          ? <span className="text-sm text-gray-600 truncate block" title={tx.note}>{tx.note}</span>
                          : <span className="text-gray-300 text-sm">—</span>
                        }
                      </td>
                      <td className="px-5 py-4 whitespace-nowrap">
                        <span className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-bold ${
                          tx.source === 'company'
                            ? 'bg-blue-100 text-blue-700'
                            : 'bg-amber-100 text-amber-700'
                        }`}>
                          {tx.source === 'company' ? '🏢 公司' : '👤 个人'}
                        </span>
                      </td>
                      <td className={`px-5 py-4 text-right font-bold text-base tabular-nums whitespace-nowrap ${tx.direction === 'income' ? 'text-green-600' : 'text-red-500'}`}>
                        {tx.direction === 'income' ? '+' : '−'}{fmt(tx.amount_yuan)}
                      </td>
                      <td className="px-5 py-4 text-center whitespace-nowrap">
                        <StatusBadge
                          active={tx.uploaded}
                          activeLabel="已上传"
                          inactiveLabel="未上传"
                          activeClass="bg-purple-100 text-purple-700 hover:bg-purple-200"
                          inactiveClass="bg-gray-100 text-gray-500 hover:bg-purple-50 hover:text-purple-600"
                          onClick={() => handleToggleUpload(tx.id)}
                          disabled={togglingId === tx.id}
                          loading={togglingId === tx.id}
                        />
                      </td>
                      <td className="px-5 py-4 text-center whitespace-nowrap">
                        {tx.source === 'personal' ? (
                          <StatusBadge
                            active={tx.reimbursed}
                            activeLabel="已报销"
                            inactiveLabel="待报销"
                            activeClass="bg-green-100 text-green-700 hover:bg-green-200"
                            inactiveClass="bg-gray-100 text-gray-500 hover:bg-green-50 hover:text-green-600"
                            onClick={() => handleToggle(tx.id)}
                            disabled={togglingId === tx.id || !tx.uploaded}
                            loading={togglingId === tx.id}
                          />
                        ) : (
                          <span className="text-gray-300 text-lg">—</span>
                        )}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}

