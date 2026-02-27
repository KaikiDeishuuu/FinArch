import { useEffect, useState, useCallback } from 'react'
import { Link } from 'react-router-dom'
import { listTransactions, toggleReimbursed, toggleUploaded } from '../api/client'
import type { Transaction } from '../api/client'

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
      className={`inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-xs font-semibold whitespace-nowrap transition-all disabled:opacity-40 disabled:cursor-not-allowed ${active ? activeClass : inactiveClass}`}
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
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-800">交易明细</h1>
          <p className="text-sm text-gray-400 mt-0.5">管理所有收入与支出记录</p>
        </div>
        <Link
          to="/add"
          className="bg-blue-600 hover:bg-blue-700 text-white text-sm font-semibold px-4 py-2.5 rounded-xl transition-colors shadow-sm"
        >
          + 添加交易
        </Link>
      </div>

      {/* Summary cards */}
      <div className="grid grid-cols-3 gap-3">
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-4">
          <p className="text-xs text-gray-400 uppercase tracking-wider mb-2">筛选结果</p>
          <p className="text-2xl font-bold text-gray-700">{filtered.length} <span className="text-base font-normal text-gray-400">笔</span></p>
        </div>
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-4">
          <p className="text-xs text-gray-400 uppercase tracking-wider mb-2">收入合计</p>
          <p className="text-2xl font-bold text-green-600">{fmt(totalIncome)}</p>
        </div>
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-4">
          <p className="text-xs text-gray-400 uppercase tracking-wider mb-2">支出合计</p>
          <p className="text-2xl font-bold text-red-500">{fmt(totalExpense)}</p>
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

      {/* Table */}
      <div className="bg-white rounded-2xl shadow-sm border border-gray-100 overflow-hidden">
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
            <table className="w-full text-sm">
              <thead>
                <tr className="bg-gray-50 text-gray-400 text-xs uppercase tracking-wider border-b border-gray-100">
                  <th className="px-5 py-3.5 text-left font-semibold">ID</th>
                  <th className="px-5 py-3.5 text-left font-semibold">日期</th>
                  <th className="px-5 py-3.5 text-left font-semibold">类别</th>
                  <th className="px-5 py-3.5 text-left font-semibold">项目</th>
                  <th className="px-5 py-3.5 text-left font-semibold w-48">备注</th>
                  <th className="px-5 py-3.5 text-left font-semibold">来源</th>
                  <th className="px-5 py-3.5 text-right font-semibold">金额</th>
                  <th className="px-5 py-3.5 text-center font-semibold">上传</th>
                  <th className="px-5 py-3.5 text-center font-semibold">报销</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((tx) => {
                  const done = tx.reimbursed && tx.uploaded
                  const urgent = !tx.reimbursed && !tx.uploaded && tx.source === 'personal'
                  return (
                    <tr
                      key={tx.id}
                      className={`border-b border-gray-50 last:border-0 transition-colors ${
                        done ? 'opacity-40' : 'hover:bg-gray-50/60'
                      }`}
                    >
                      <td className="px-5 py-3.5">
                        <button
                          onClick={() => copyId(tx.id)}
                          title="点击复制完整 ID"
                          className={`font-mono text-xs rounded-lg px-2 py-1 transition-all ${
                            copiedId === tx.id
                              ? 'bg-green-100 text-green-600'
                              : 'bg-gray-100 text-gray-400 hover:bg-blue-50 hover:text-blue-500'
                          }`}
                        >
                          {copiedId === tx.id ? '✓ 已复制' : tx.id.slice(0, 8) + '…'}
                        </button>
                      </td>
                      <td className="px-5 py-3.5 text-gray-500 tabular-nums whitespace-nowrap">{tx.occurred_at}</td>
                      <td className="px-5 py-3.5">
                        <span className={`inline-flex items-center gap-1.5 font-medium ${urgent ? 'text-gray-800' : 'text-gray-600'}`}>
                          <span className={`w-2 h-2 rounded-full shrink-0 ${tx.direction === 'income' ? 'bg-green-400' : 'bg-red-400'}`} />
                          {tx.category}
                        </span>
                      </td>
                      <td className="px-5 py-3.5">
                        {tx.project_id
                          ? <span className="text-xs font-mono bg-gray-100 text-gray-600 px-2 py-0.5 rounded-lg">{tx.project_id}</span>
                          : <span className="text-gray-300">—</span>
                        }
                      </td>
                      <td className="px-5 py-3.5 text-gray-500 text-xs w-48 max-w-[192px] truncate" title={tx.note ?? undefined}>
                        {tx.note || <span className="text-gray-300">—</span>}
                      </td>
                      <td className="px-5 py-3.5 whitespace-nowrap">
                        <span className={`inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-xs font-semibold whitespace-nowrap ${
                          tx.source === 'company'
                            ? 'bg-blue-50 text-blue-600'
                            : 'bg-amber-50 text-amber-600'
                        }`}>
                          {tx.source === 'company' ? '🏢 公司' : '👤 个人'}
                        </span>
                      </td>
                      <td className={`px-5 py-3.5 text-right font-bold tabular-nums whitespace-nowrap ${tx.direction === 'income' ? 'text-green-600' : 'text-red-500'}`}>
                        {tx.direction === 'income' ? '+' : '−'}{fmt(tx.amount_yuan)}
                      </td>
                      <td className="px-5 py-3.5 text-center whitespace-nowrap">
                        <StatusBadge
                          active={tx.uploaded}
                          activeLabel="已上传"
                          inactiveLabel="未上传"
                          activeClass="bg-purple-100 text-purple-700 hover:bg-purple-200"
                          inactiveClass="bg-gray-100 text-gray-400 hover:bg-purple-50 hover:text-purple-500"
                          onClick={() => handleToggleUpload(tx.id)}
                          disabled={togglingId === tx.id}
                          loading={togglingId === tx.id}
                        />
                      </td>
                      <td className="px-5 py-3.5 text-center whitespace-nowrap">
                        {tx.source === 'personal' ? (
                          <StatusBadge
                            active={tx.reimbursed}
                            activeLabel="已报销"
                            inactiveLabel="待报销"
                            activeClass="bg-green-100 text-green-700 hover:bg-green-200"
                            inactiveClass="bg-gray-100 text-gray-400 hover:bg-green-50 hover:text-green-500"
                            onClick={() => handleToggle(tx.id)}
                            disabled={togglingId === tx.id || !tx.uploaded}
                            loading={togglingId === tx.id}
                          />
                        ) : (
                          <span className="text-gray-200 text-lg">—</span>
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
