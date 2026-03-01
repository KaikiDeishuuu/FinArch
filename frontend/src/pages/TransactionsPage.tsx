import { useState, useMemo, useRef } from 'react'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'
import { toggleReimbursed, toggleUploaded } from '../api/client'
import type { Transaction } from '../api/client'
import { useAuth } from '../contexts/AuthContext'
import { exportTransactionsPDF } from '../utils/exportTransactionsPDF'
import { formatAmount, sumInCNY } from '../utils/format'
import { useExchangeRates } from '../contexts/ExchangeRateContext'
import { useTransactions, useInvalidateTransactions } from '../hooks/useTransactions'
import { useWindowVirtualizer } from '@tanstack/react-virtual'

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
  const { rates } = useExchangeRates()
  const { data: txs = [], isLoading: loading } = useTransactions()
  const invalidate = useInvalidateTransactions()
  const [filter, setFilter] = useState<FilterTab>('all')
  const [filterCategory, setFilterCategory] = useState('')
  const [filterProject, setFilterProject] = useState('')
  const [copiedId, setCopiedId] = useState<string | null>(null)
  const [togglingId, setTogglingId] = useState<string | null>(null)

  // Filtering (inline — no hooks; must be before useWindowVirtualizer)
  const baseFiltered = txs.filter((t) => {
    if (filter === 'unreimbursed') return !t.reimbursed
    if (filter === 'reimbursed') return t.reimbursed
    return true
  })
  const filtered = baseFiltered
    .filter((t) => !filterCategory || t.category === filterCategory)
    .filter((t) => !filterProject || (t.project_id ?? '') === filterProject)

  const allCategories = useMemo(
    () => Array.from(new Set(txs.map(t => t.category).filter(Boolean))).sort() as string[],
    [txs]
  )
  const allProjects = useMemo(
    () => Array.from(new Set(txs.map(t => t.project_id).filter((p): p is string => !!p))).sort(),
    [txs]
  )

  // Mobile card list virtualizer (window-level scrolling)
  const mobileListRef = useRef<HTMLDivElement>(null)
  const mobileVirtualizer = useWindowVirtualizer({
    count: filtered.length,
    estimateSize: () => 112,
    overscan: 5,
    scrollMargin: mobileListRef.current?.offsetTop ?? 0,
  })

  const fmt = (t: Transaction) => formatAmount(t.amount_yuan, t.currency)

  function exportPDF() {
    const parts: string[] = [{ all: '全部', unreimbursed: '待报销', reimbursed: '已报销' }[filter]]
    if (filterCategory) parts.push(`类别: ${filterCategory}`)
    if (filterProject) parts.push(`项目: ${filterProject}`)
    exportTransactionsPDF(filtered, parts.join(' · '), user, rates)
  }

  async function handleToggle(id: string) {
    setTogglingId(id)
    try {
      await toggleReimbursed(id)
      invalidate()
    } catch {
      toast.error('报销状态切换失败')
    } finally {
      setTogglingId(null)
    }
  }

  async function handleToggleUpload(id: string) {
    setTogglingId(id)
    try {
      await toggleUploaded(id)
      invalidate()
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      toast.error(msg || '上传状态切换失败，请确认后端已重启')
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
    { key: 'unreimbursed', label: '待报销', count: txs.filter(t => !t.reimbursed).length },
    { key: 'reimbursed', label: '已报销', count: txs.filter(t => t.reimbursed).length },
  ]

  const incomeItems = filtered.filter(t => t.direction === 'income')
  const expenseItems = filtered.filter(t => t.direction === 'expense')
  const totalIncomeStr = formatAmount(sumInCNY(incomeItems, rates), 'CNY')
  const totalExpenseStr = formatAmount(sumInCNY(expenseItems, rates), 'CNY')

  return (
    <div className="space-y-5">
      {/* Header */}
      <div className="flex items-center justify-between gap-3">
        <div className="min-w-0">
          <h1 className="text-2xl font-bold text-gray-900 tracking-tight">交易明细</h1>
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
            className="shrink-0 bg-teal-600 hover:bg-teal-700 text-white text-sm font-semibold px-4 py-2.5 rounded-xl transition-colors shadow-sm"
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
          <p className="text-sm md:text-2xl font-bold text-emerald-500 tabular-nums truncate">{totalIncomeStr}</p>
        </div>
        <div className="bg-white rounded-2xl border border-gray-100 shadow-sm p-3 md:p-4">
          <p className="text-[10px] md:text-xs text-gray-400 uppercase tracking-wider mb-1 md:mb-2">支出</p>
          <p className="text-sm md:text-2xl font-bold text-rose-500 tabular-nums truncate">{totalExpenseStr}</p>
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
                filter === t.key ? 'bg-white shadow text-teal-600' : 'text-gray-500 hover:text-gray-700'
              }`}
            >
              {t.label}
              {t.count !== undefined && (
                <span className={`text-xs rounded-full px-1.5 py-0.5 font-semibold tabular-nums ${
                  filter === t.key ? 'bg-teal-100 text-teal-600' : 'bg-gray-200 text-gray-500'
                }`}>{t.count}</span>
              )}
            </button>
          ))}
        </div>

        {/* Category filter */}
        {allCategories.length > 0 && (
          <select
            value={filterCategory}
            onChange={e => setFilterCategory(e.target.value)}
            className={`h-9 rounded-xl border text-sm px-2.5 pr-7 outline-none transition-all appearance-none bg-no-repeat cursor-pointer ${
              filterCategory
                ? 'border-teal-400 bg-teal-50 text-teal-700 font-semibold'
                : 'border-gray-200 bg-gray-100 text-gray-500 hover:border-gray-300'
            }`}
            style={{ backgroundImage: `url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 24 24' fill='none' stroke='%236b7280' stroke-width='2.5'%3E%3Cpolyline points='6 9 12 15 18 9'/%3E%3C/svg%3E")`, backgroundPosition: 'right 8px center' }}
          >
            <option value="">全部类别</option>
            {allCategories.map(c => <option key={c} value={c}>{c}</option>)}
          </select>
        )}

        {/* Project filter */}
        {allProjects.length > 0 && (
          <select
            value={filterProject}
            onChange={e => setFilterProject(e.target.value)}
            className={`h-9 rounded-xl border text-sm px-2.5 pr-7 outline-none transition-all appearance-none bg-no-repeat cursor-pointer ${
              filterProject
                ? 'border-teal-400 bg-teal-50 text-teal-700 font-semibold'
                : 'border-gray-200 bg-gray-100 text-gray-500 hover:border-gray-300'
            }`}
            style={{ backgroundImage: `url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 24 24' fill='none' stroke='%236b7280' stroke-width='2.5'%3E%3Cpolyline points='6 9 12 15 18 9'/%3E%3C/svg%3E")`, backgroundPosition: 'right 8px center' }}
          >
            <option value="">全部项目</option>
            {allProjects.map(p => <option key={p} value={p}>{p}</option>)}
          </select>
        )}

        {/* Clear extra filters */}
        {(filterCategory || filterProject) && (
          <button
            onClick={() => { setFilterCategory(''); setFilterProject('') }}
            className="h-9 px-3 rounded-xl border border-gray-200 bg-gray-100 text-gray-400 hover:text-gray-600 hover:bg-gray-200 text-xs transition-all"
          >
            清除筛选
          </button>
        )}
      </div>

      {/* Mobile card list (virtualized) */}
      <div className="md:hidden">
        {loading ? (
          <div className="flex flex-col items-center justify-center py-16 gap-3">
            <div className="w-8 h-8 border-4 border-teal-500 border-t-transparent rounded-full animate-spin" />
            <p className="text-sm text-gray-400">加载中…</p>
          </div>
        ) : filtered.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12 gap-2">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5} strokeLinecap="round" strokeLinejoin="round" className="w-12 h-12 text-gray-200"><path d="M3 8l7.89 5.26a2 2 0 002.22 0L21 8M5 19h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"/></svg>
            <p className="text-sm text-gray-400">暂无记录</p>
          </div>
        ) : (
          <div
            ref={mobileListRef}
            style={{ height: `${mobileVirtualizer.getTotalSize()}px`, position: 'relative' }}
          >
            {mobileVirtualizer.getVirtualItems().map((vItem) => {
              const tx = filtered[vItem.index]
              const done = tx.reimbursed && tx.uploaded
              return (
                <div
                  key={vItem.key}
                  data-index={vItem.index}
                  ref={mobileVirtualizer.measureElement}
                  style={{
                    position: 'absolute',
                    top: 0,
                    left: 0,
                    width: '100%',
                    transform: `translateY(${vItem.start - mobileVirtualizer.options.scrollMargin}px)`,
                    paddingBottom: '8px',
                  }}
                >
                  <div
                    className={`bg-white rounded-2xl border border-gray-100 shadow-sm p-4 transition-opacity ${
                      done ? 'opacity-40' : ''
                    }`}
                  >
                    {/* Row 1: category + amount */}
                    <div className="flex items-start justify-between gap-2 mb-2.5">
                      <div className="flex items-center gap-1.5 min-w-0 pt-0.5">
                        <span className={`w-2.5 h-2.5 rounded-full shrink-0 ${
                          tx.direction === 'income' ? 'bg-emerald-400' : 'bg-rose-400'
                        }`} />
                        <span className="font-semibold text-gray-800 text-sm truncate">{tx.category}</span>
                        {tx.project_id && (
                          <span className="text-xs font-mono bg-gray-100 text-gray-500 px-1.5 py-0.5 rounded shrink-0">
                            {tx.project_id}
                          </span>
                        )}
                      </div>
                      <span className={`font-bold tabular-nums text-base shrink-0 ml-2 ${
                        tx.direction === 'income' ? 'text-emerald-500' : 'text-rose-500'
                      }`}>
                        {tx.direction === 'income' ? '+' : '−'}{fmt(tx)}
                      </span>
                    </div>
                    {/* Row 2: date + source */}
                    <div className="flex items-center gap-2 mb-1.5">
                      <span className="text-xs text-gray-400 tabular-nums">{tx.occurred_at}</span>
                      <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-semibold ${
                        tx.source === 'company' ? 'bg-teal-50 text-teal-600' : 'bg-amber-50 text-amber-600'
                      }`}>
                        {tx.source === 'company' ? '公司' : '个人'}
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
                      <StatusBadge
                          active={tx.reimbursed}
                          activeLabel="已报销"
                          inactiveLabel="待报销"
                          activeClass="bg-emerald-100 text-emerald-700"
                          inactiveClass="bg-gray-100 text-gray-400"
                          onClick={() => handleToggle(tx.id)}
                          disabled={togglingId === tx.id || !tx.uploaded}
                          loading={togglingId === tx.id}
                        />
                      <button
                        onClick={() => copyId(tx.id)}
                        className={`ml-auto font-mono text-xs rounded-lg px-2 py-1 transition-all ${
                          copiedId === tx.id
                            ? 'bg-emerald-100 text-emerald-500'
                            : 'bg-gray-100 text-gray-400'
                        }`}
                      >
                        {copiedId === tx.id ? '✓ 已复制' : tx.id.slice(0, 8) + '…'}
                      </button>
                    </div>
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </div>

      {/* Desktop Table */}
      <div className="hidden md:block bg-white rounded-2xl shadow-sm border border-gray-100 overflow-hidden">
        {loading ? (
          <div className="flex flex-col items-center justify-center py-16 gap-3">
            <div className="w-8 h-8 border-4 border-teal-500 border-t-transparent rounded-full animate-spin" />
            <p className="text-sm text-gray-400">加载中…</p>
          </div>
        ) : filtered.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-16 gap-2">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5} strokeLinecap="round" strokeLinejoin="round" className="w-12 h-12 text-gray-200"><path d="M3 8l7.89 5.26a2 2 0 002.22 0L21 8M5 19h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"/></svg>
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
                  const urgent = !tx.reimbursed && !tx.uploaded
                  return (
                    <tr
                      key={tx.id}
                      className={`border-b border-gray-100 last:border-0 transition-colors ${
                        done ? 'opacity-40' : 'hover:bg-teal-50/30'
                      }`}
                    >
                      <td className="px-5 py-4 whitespace-nowrap">
                        <p className="text-sm font-medium text-gray-700 tabular-nums">{tx.occurred_at}</p>
                        <button
                          onClick={() => copyId(tx.id)}
                          title="点击复制完整 ID"
                          className={`font-mono text-[11px] rounded px-1 py-0.5 transition-all mt-0.5 block ${
                            copiedId === tx.id
                              ? 'bg-emerald-100 text-emerald-500'
                              : 'text-gray-300 hover:text-teal-400 hover:bg-teal-50'
                          }`}
                        >
                          {copiedId === tx.id ? '✓ 已复制' : tx.id.slice(0, 8) + '…'}
                        </button>
                      </td>
                      <td className="px-5 py-4">
                        <span className={`inline-flex items-center gap-2 text-sm font-semibold ${urgent ? 'text-gray-900' : 'text-gray-700'}`}>
                          <span className={`w-2.5 h-2.5 rounded-full shrink-0 ${tx.direction === 'income' ? 'bg-emerald-500' : 'bg-rose-500'}`} />
                          {tx.category}
                        </span>
                      </td>
                      <td className="px-5 py-4 max-w-[140px]">
                        {tx.project_id
                          ? <span className="inline-flex items-center text-xs font-mono font-semibold bg-teal-50 text-teal-700 border border-teal-200 px-2 py-0.5 rounded-md">{tx.project_id}</span>
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
                            ? 'bg-teal-100 text-teal-700'
                            : 'bg-amber-100 text-amber-700'
                        }`}>
                          {tx.source === 'company' ? '公司' : '个人'}
                        </span>
                      </td>
                      <td className={`px-5 py-4 text-right font-bold text-base tabular-nums whitespace-nowrap ${tx.direction === 'income' ? 'text-emerald-500' : 'text-rose-500'}`}>
                        {tx.direction === 'income' ? '+' : '−'}{fmt(tx)}
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
                        <StatusBadge
                            active={tx.reimbursed}
                            activeLabel="已报销"
                            inactiveLabel="待报销"
                            activeClass="bg-emerald-100 text-emerald-700 hover:bg-emerald-200"
                            inactiveClass="bg-gray-100 text-gray-500 hover:bg-emerald-50 hover:text-emerald-500"
                            onClick={() => handleToggle(tx.id)}
                            disabled={togglingId === tx.id || !tx.uploaded}
                            loading={togglingId === tx.id}
                          />
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

