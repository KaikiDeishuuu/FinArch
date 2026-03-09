import { useState, useMemo, useRef, useCallback, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'
import { toggleReimbursed, toggleUploaded } from '../api/client'
import type { Transaction, Account } from '../api/client'
import { StaggerContainer, StaggerItem, CardSkeleton, RowSkeleton } from '../motion'
import Select from '../components/Select'
import { useAuth } from '../contexts/AuthContext'
import { exportTransactionsPDF } from '../utils/exportTransactionsPDF'
import { formatAmount, sumInCNY } from '../utils/format'
import { useExchangeRates } from '../contexts/ExchangeRateContext'
import { useTransactions } from '../hooks/useTransactions'
import { useAccounts } from '../hooks/useAccounts'
import { useVirtualizer } from '@tanstack/react-virtual'
import { categoryLabel } from '../utils/categoryLabel'
import { useMode } from '../contexts/ModeContext'
import { useRefreshFinanceData } from '../hooks/useRefreshFinanceData'

type FilterTab = 'all' | 'unreimbursed' | 'reimbursed'

function splitTimestamp(value: string) {
  const normalized = value.includes('T') ? value : value.replace(' ', 'T')
  const hasTimezone = /([zZ]|[+\-]\d{2}:?\d{2})$/.test(normalized)
  const parsed = new Date(hasTimezone ? normalized : `${normalized}Z`)
  if (Number.isNaN(parsed.getTime())) {
    const [date = value, time = ''] = value.split(' ')
    return { date, time: time.slice(0, 5) }
  }
  return {
    date: parsed.toLocaleDateString(),
    time: parsed.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hour12: false }),
  }
}

function formatLifecycleTimestamp(value?: string | null) {
  if (!value) return null
  const { date, time } = splitTimestamp(value)
  return `${date} ${time}`.trim()
}

const TagIcon = () => <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} className="h-3.5 w-3.5"><path d="m20 12-8 8-8-8 8-8h8z" /><circle cx="16" cy="8" r="1.5" /></svg>
const BuildingIcon = () => <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} className="h-3.5 w-3.5"><path d="M3 21h18" /><path d="M5 21V7l7-3v17" /><path d="M19 21V11l-7-2" /></svg>
const FolderIcon = () => <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} className="h-3.5 w-3.5"><path d="M3 7h6l2 2h10v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" /></svg>
const ClockIcon = () => <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} className="h-3.5 w-3.5"><circle cx="12" cy="12" r="8" /><path d="M12 8v4l3 2" /></svg>

function StatusBadge({
  active, activeLabel, inactiveLabel, activeClass, inactiveClass,
  onClick, disabled, loading, locked, lockedTitle,
}: {
  active: boolean; activeLabel: string; inactiveLabel: string
  activeClass: string; inactiveClass: string
  onClick: () => void; disabled?: boolean; loading?: boolean; locked?: boolean; lockedTitle?: string
}) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      title={locked ? lockedTitle : undefined}
      className={`inline-flex items-center gap-1 px-3 py-1.5 rounded-full text-xs font-bold whitespace-nowrap transition-all duration-200 ease-in-out transform active:scale-95 ${disabled && !loading ? 'opacity-40 cursor-not-allowed' : ''
        } ${locked ? 'opacity-50 cursor-not-allowed ring-1 ring-gray-200 dark:ring-gray-600' : ''} ${active ? activeClass : inactiveClass}`}
    >
      {loading ? (
        <span className="w-3 h-3 border-2 border-current border-t-transparent rounded-full animate-spin" />
      ) : active ? (
        <svg className="w-3 h-3 transition-transform duration-200" fill="currentColor" viewBox="0 0 20 20"><path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" /></svg>
      ) : locked ? (
        <svg className="w-3 h-3 opacity-50" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><rect x="3" y="11" width="18" height="11" rx="2" /><path d="M7 11V7a5 5 0 0110 0v4" /></svg>
      ) : (
        <span className="w-3 h-3 rounded-full border-2 border-current opacity-50 transition-all duration-200" />
      )}
      {loading ? '…' : active ? activeLabel : inactiveLabel}
    </button>
  )
}

export default function TransactionsPage() {
  const { t } = useTranslation()
  const { isWorkMode } = useMode()
  const { user } = useAuth()
  const { rates } = useExchangeRates()
  const { data: txs = [], isLoading: loading } = useTransactions()
  const { data: accounts = [] } = useAccounts()
  const refreshFinanceData = useRefreshFinanceData()
  const [filter, setFilter] = useState<FilterTab>('all')
  const [filterCategory, setFilterCategory] = useState('')
  const [filterProject, setFilterProject] = useState('')
  const [filterAccount, setFilterAccount] = useState('')
  const [copiedId, setCopiedId] = useState<string | null>(null)
  const [togglingAction, setTogglingAction] = useState<{ id: string; type: 'uploaded' | 'reimbursed' } | null>(null)
  const [optimisticState, setOptimisticState] = useState<Record<string, { uploaded: boolean; reimbursed: boolean }>>({})
  const effectiveSourceFilter: 'personal' | 'company' = isWorkMode ? 'company' : 'personal'

  const tabLabelAll = t('transactions.reimbursementTabs.all')
  const tabLabelPending = isWorkMode ? t('transactions.reimbursementTabs.pending') : t('transactions.life.tabs.pending')
  const tabLabelDone = isWorkMode ? t('transactions.reimbursementTabs.done') : t('transactions.life.tabs.done')
  const processedLabel = isWorkMode ? t('transactions.badges.reimbursed') : t('transactions.life.badges.processed')
  const lockTitle = isWorkMode ? t('transactions.lockTitle') : t('transactions.life.lockTitle')
  const incomeHint = isWorkMode ? t('transactions.incomeNoReimburse') : t('transactions.life.incomeNoProcess')

  const txsView = useMemo(() =>
    txs.map((tx) => {
      const override = optimisticState[tx.id]
      return override
        ? { ...tx, uploaded: override.uploaded, reimbursed: override.reimbursed }
        : tx
    }),
    [txs, optimisticState]
  )

  useEffect(() => {
    setOptimisticState((prev) => {
      const next = { ...prev }
      let changed = false
      for (const tx of txs) {
        const state = next[tx.id]
        if (state && state.uploaded === tx.uploaded && state.reimbursed === tx.reimbursed) {
          delete next[tx.id]
          changed = true
        }
      }
      return changed ? next : prev
    })
  }, [txs])

  // Build account lookup for filter display
  const activeAccounts = useMemo(() =>
    accounts.filter((a: Account) => a.is_active),
    [accounts]
  )

  // Map account id → name for display in transaction rows
  const accountMap = useMemo(
    () => Object.fromEntries(accounts.map((a: Account) => [a.id, a.name])),
    [accounts]
  )

  // Filter accounts by selected source tab
  const filteredAccounts = useMemo(() => {
    const acctType = effectiveSourceFilter === 'company' ? 'public' : 'personal'
    return activeAccounts.filter((a: Account) => a.type === acctType)
  }, [activeAccounts, effectiveSourceFilter])

  // Filtering (inline — no hooks; must be before useWindowVirtualizer)
  // Rule: income transactions are never "pending" — they count as inherently "done".
  const baseFiltered = txsView.filter((t) => {
    if (filter === 'unreimbursed') return t.direction === 'expense' && !t.reimbursed
    if (filter === 'reimbursed') return t.reimbursed || t.direction === 'income'
    return true
  })
  const filtered = baseFiltered
    .filter((t) => t.source === effectiveSourceFilter)
    .filter((t) => !filterCategory || t.category === filterCategory)
    .filter((t) => !filterProject || (t.project_id ?? '') === filterProject)
    .filter((t) => !filterAccount || t.account_id === filterAccount)

  const allCategories = useMemo(
    () => Array.from(new Set(txsView.filter(t => t.source === effectiveSourceFilter).map(t => t.category).filter(Boolean))).sort() as string[],
    [txsView, effectiveSourceFilter]
  )
  const allProjects = useMemo(
    () => Array.from(new Set(txsView.filter(t => t.source === effectiveSourceFilter).map(t => t.project_id).filter((p): p is string => !!p))).sort(),
    [txsView, effectiveSourceFilter]
  )

  // Mobile card list virtualizer — uses <main> as scroll container
  const mobileListRef = useRef<HTMLDivElement>(null)
  const getScrollElement = useCallback(
    () => document.querySelector<HTMLElement>('.scroll-main'),
    [],
  )
  const mobileVirtualizer = useVirtualizer({
    count: filtered.length,
    estimateSize: () => 112,
    overscan: 5,
    getScrollElement,
    scrollMargin: mobileListRef.current?.offsetTop ?? 0,
  })

  const fmt = (t: Transaction) => formatAmount(t.amount_yuan, t.currency)

  function exportPDF() {
    const parts: string[] = [{ all: tabLabelAll, unreimbursed: tabLabelPending, reimbursed: tabLabelDone }[filter]]
    if (filterCategory) parts.push(`${t('transactions.table.category')}: ${categoryLabel(filterCategory)}`)
    if (filterProject) parts.push(`${t('transactions.table.project')}: ${filterProject}`)
    exportTransactionsPDF(filtered, parts.join(' · '), user, rates, isWorkMode, accountMap)
  }

  async function handleToggle(id: string) {
    const tx = txsView.find((t) => t.id === id)
    if (!tx) return
    const prev = { uploaded: tx.uploaded, reimbursed: tx.reimbursed }
    const next = { uploaded: tx.uploaded, reimbursed: !tx.reimbursed }
    setOptimisticState((curr) => ({ ...curr, [id]: next }))
    setTogglingAction({ id, type: 'reimbursed' })
    try {
      await toggleReimbursed(id)
      refreshFinanceData()
    } catch {
      setOptimisticState((curr) => ({ ...curr, [id]: prev }))
      toast.error(isWorkMode ? t('transactions.toast.reimbursedError') : t('transactions.life.toast.processError'))
    } finally {
      setTogglingAction(null)
    }
  }

  async function handleToggleUpload(id: string) {
    // Rollback protection: if uploaded AND reimbursed, must cancel reimburse first
    const tx = txsView.find(t => t.id === id)
    if (!tx) return
    if (tx.uploaded && tx.reimbursed) {
      toast.error(isWorkMode ? t('transactions.toast.cancelReimburseFirst') : t('transactions.life.toast.cancelProcessFirst'))
      return
    }
    const prev = { uploaded: tx.uploaded, reimbursed: tx.reimbursed }
    const next = { uploaded: !tx.uploaded, reimbursed: tx.reimbursed }
    setOptimisticState((curr) => ({ ...curr, [id]: next }))
    setTogglingAction({ id, type: 'uploaded' })
    try {
      await toggleUploaded(id)
      refreshFinanceData()
    } catch (err: unknown) {
      setOptimisticState((curr) => ({ ...curr, [id]: prev }))
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      toast.error(msg || t('transactions.toast.uploadError'))
    } finally {
      setTogglingAction(null)
    }
  }

  function copyId(id: string) {
    navigator.clipboard.writeText(id).then(() => {
      setCopiedId(id)
      setTimeout(() => setCopiedId(null), 1500)
    })
  }

  const tabs: { key: FilterTab; label: string; count?: number }[] = [
    { key: 'all', label: tabLabelAll, count: txsView.length },
    { key: 'unreimbursed', label: tabLabelPending, count: txsView.filter(t => t.direction === 'expense' && !t.reimbursed).length },
    { key: 'reimbursed', label: tabLabelDone, count: txsView.filter(t => t.reimbursed || t.direction === 'income').length },
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
          <h1 className="text-2xl font-bold text-gray-900 dark:text-gray-100 tracking-tight">{t('transactions.title')}</h1>
          <p className="text-sm text-gray-400 dark:text-gray-500 mt-0.5 hidden sm:block">{t('transactions.subtitle')}</p>
        </div>
        <div className="flex items-center gap-2 shrink-0">
          <button
            onClick={exportPDF}
            disabled={loading || filtered.length === 0}
            className="flex items-center gap-1.5 bg-gray-100 dark:bg-gray-800 hover:bg-gray-200 dark:hover:bg-gray-700 text-gray-700 dark:text-gray-300 text-sm font-semibold px-3.5 py-2.5 rounded-xl transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
            title={t('transactions.exportTooltip')}
          >
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M12 10v6m0 0l-3-3m3 3l3-3M3 17V7a2 2 0 012-2h6l2 2h4a2 2 0 012 2v8a2 2 0 01-2 2H5a2 2 0 01-2-2z" />
            </svg>
            <span className="hidden sm:inline">{t('transactions.exportPdf')}</span>
          </button>
          <Link
            to="/add"
            className="shrink-0 bg-gradient-to-r from-violet-600 to-purple-600 hover:from-violet-700 hover:to-purple-700 text-white text-sm font-semibold px-4 py-2.5 rounded-xl transition-all shadow-md shadow-violet-300/30 dark:shadow-violet-900/30"
          >
            {t('common.add')}
          </Link>
        </div>
      </div>

      {/* Summary cards */}
      <StaggerContainer className="grid grid-cols-3 gap-2 md:gap-3">
        <StaggerItem>
          <div className="bg-white dark:bg-[hsl(260,15%,11%)] rounded-2xl border border-gray-100/80 dark:border-gray-800/50 p-3 md:p-4 shadow-sm">
            <p className="text-[10px] md:text-xs text-gray-400 dark:text-gray-500 uppercase tracking-wider mb-1 md:mb-2">{t('transactions.summary.filtered')}</p>
            <p className="text-lg md:text-2xl font-bold text-gray-700 dark:text-gray-200">{filtered.length} <span className="text-sm md:text-base font-normal text-gray-400 dark:text-gray-500">{t('transactions.unit')}</span></p>
          </div>
        </StaggerItem>
        <StaggerItem>
          <div className="bg-white dark:bg-[hsl(260,15%,11%)] rounded-2xl border border-gray-100/80 dark:border-gray-800/50 p-3 md:p-4 shadow-sm">
            <p className="text-[10px] md:text-xs text-gray-400 dark:text-gray-500 uppercase tracking-wider mb-1 md:mb-2">{t('transactions.summary.income')}</p>
            <p className="text-sm md:text-2xl font-bold text-emerald-500 tabular-nums truncate">{totalIncomeStr}</p>
          </div>
        </StaggerItem>
        <StaggerItem>
          <div className="bg-white dark:bg-[hsl(260,15%,11%)] rounded-2xl border border-gray-100/80 dark:border-gray-800/50 p-3 md:p-4 shadow-sm">
            <p className="text-[10px] md:text-xs text-gray-400 dark:text-gray-500 uppercase tracking-wider mb-1 md:mb-2">{t('transactions.summary.expense')}</p>
            <p className="text-sm md:text-2xl font-bold text-rose-500 tabular-nums truncate">{totalExpenseStr}</p>
          </div>
        </StaggerItem>
      </StaggerContainer>

      {/* Tabs */}
      <div className="flex rounded-xl bg-gray-100 dark:bg-gray-800 p-1 gap-0.5 w-full sm:w-fit overflow-x-auto">
        {tabs.map((tb) => (
          <button
            key={tb.key}
            onClick={() => setFilter(tb.key)}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm font-medium transition-all ${filter === tb.key ? 'bg-white dark:bg-gray-700 shadow text-violet-600 dark:text-violet-400' : 'text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200'
              }`}
          >
            {tb.label}
            {tb.count !== undefined && (
              <span className={`text-xs rounded-full px-1.5 py-0.5 font-semibold tabular-nums ${filter === tb.key ? 'bg-violet-100 dark:bg-violet-500/20 text-violet-600 dark:text-violet-400' : 'bg-gray-200 dark:bg-gray-700 text-gray-500 dark:text-gray-400'
                }`}>{tb.count}</span>
            )}
          </button>
        ))}
      </div>

      {/* Filters */}
      <div className="flex items-center gap-2 flex-wrap">
        {/* Source filter */}
        <div className="h-8 px-2.5 inline-flex items-center rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-800 text-xs font-medium text-gray-500 dark:text-gray-400">
          {isWorkMode ? t('common.company') : t('common.personal')}
        </div>

        {/* Account filter */}
        {filteredAccounts.length > 1 && (
          <div className="w-fit min-w-[6.5rem]">
            <Select
              value={filterAccount}
              onChange={setFilterAccount}
              placeholder={t('transactions.filterPlaceholders.account')}
              size="sm"
              activeHighlight
              options={[
                { value: '', label: t('transactions.filterPlaceholders.account') },
                ...filteredAccounts.map((a: Account) => ({ value: a.id, label: a.name })),
              ]}
            />
          </div>
        )}

        {/* Category filter */}
        {allCategories.length > 0 && (
          <div className="w-fit min-w-[6.5rem]">
            <Select
              value={filterCategory}
              onChange={setFilterCategory}
              placeholder={t('transactions.filterPlaceholders.category')}
              size="sm"
              activeHighlight
              options={[
                { value: '', label: t('transactions.filterPlaceholders.category') },
                ...allCategories.map(c => ({ value: c, label: categoryLabel(c) })),
              ]}
            />
          </div>
        )}

        {/* Project filter */}
        {allProjects.length > 0 && (
          <div className="w-fit min-w-[6.5rem]">
            <Select
              value={filterProject}
              onChange={setFilterProject}
              placeholder={t('transactions.filterPlaceholders.project')}
              size="sm"
              activeHighlight
              options={[
                { value: '', label: t('transactions.filterPlaceholders.project') },
                ...allProjects.map(p => ({ value: p, label: p })),
              ]}
            />
          </div>
        )}

        {/* Clear extra filters */}
        {(filterCategory || filterProject || filterAccount) && (
          <button
            onClick={() => { setFilterCategory(''); setFilterProject(''); setFilterAccount('') }}
            className="h-8 px-2.5 rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-800 text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 text-xs transition-all"
          >
            {t('common.clear')}
          </button>
        )}
      </div>

      {/* Mobile card list (virtualized) */}
      <div className="md:hidden">
        {loading ? (
          <div className="space-y-2">
            {[0, 1, 2, 3].map(i => <CardSkeleton key={i} />)}
          </div>
        ) : filtered.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12 gap-2">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5} strokeLinecap="round" strokeLinejoin="round" className="w-12 h-12 text-gray-200 dark:text-gray-700"><path d="M3 8l7.89 5.26a2 2 0 002.22 0L21 8M5 19h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" /></svg>
            <p className="text-sm text-gray-400 dark:text-gray-500">{t('transactions.noRecords')}</p>
          </div>
        ) : (
          <div
            ref={mobileListRef}
            style={{ height: `${mobileVirtualizer.getTotalSize()}px`, position: 'relative' }}
          >
            {mobileVirtualizer.getVirtualItems().map((vItem) => {
              const tx = filtered[vItem.index]
              const isExpense = tx.direction === 'expense'
              const done = isExpense && tx.reimbursed && tx.uploaded
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
                    transform: `translateY(${vItem.start - (mobileVirtualizer.options.scrollMargin ?? 0)}px)`,
                    paddingBottom: '8px',
                  }}
                >
                  <div
                    className={`bg-white dark:bg-[hsl(260,15%,11%)] rounded-2xl border border-gray-100/80 dark:border-gray-800/50 p-4 shadow-sm transition-opacity ${done ? 'opacity-40' : ''
                      }`}
                  >
                    {/* Row 1: category + amount */}
                    <div className="flex items-start justify-between gap-2 mb-2.5">
                      <div className="flex items-center gap-1.5 min-w-0 pt-0.5">
                        <span className={`w-2.5 h-2.5 rounded-full shrink-0 ${tx.direction === 'income' ? 'bg-emerald-400' : 'bg-rose-400'
                          }`} />
                        <span className="font-semibold text-gray-800 dark:text-gray-200 text-sm truncate">{categoryLabel(tx.category)}</span>
                        {tx.project_id && (
                          <span className="text-xs font-mono bg-gray-100 dark:bg-gray-800 text-gray-500 dark:text-gray-400 px-1.5 py-0.5 rounded shrink-0">
                            {tx.project_id}
                          </span>
                        )}
                      </div>
                      <span className={`font-bold tabular-nums text-base shrink-0 ml-2 ${tx.direction === 'income' ? 'text-emerald-500' : 'text-rose-500'
                        }`}>
                        {tx.direction === 'income' ? '+' : '−'}{fmt(tx)}
                      </span>
                    </div>
                    {/* Row 2: date + source + account */}
                    <div className="flex items-center gap-2 flex-wrap mb-1.5">
                      <span className="text-xs text-gray-400 dark:text-gray-500 tabular-nums">{tx.occurred_at}</span>
                      <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-semibold ${tx.source === 'company' ? 'bg-sky-50 dark:bg-sky-500/15 text-sky-600 dark:text-sky-400' : 'bg-amber-50 dark:bg-amber-500/15 text-amber-600 dark:text-amber-400'
                        }`}>
                        {tx.source === 'company' ? t('common.company') : t('common.personal')}
                      </span>
                      {tx.account_id && accountMap[tx.account_id] && (
                        <span className="text-xs bg-gray-100 dark:bg-gray-800 text-gray-500 dark:text-gray-400 px-1.5 py-0.5 rounded shrink-0 truncate max-w-[120px]" title={accountMap[tx.account_id]}>
                          {accountMap[tx.account_id]}
                        </span>
                      )}
                    </div>
                    {/* Row 3: note */}
                    {tx.note && (
                      <p className="text-xs text-gray-400 dark:text-gray-500 mb-2.5 truncate">{tx.note}</p>
                    )}
                    {/* Row 4: action badges + copy ID */}
                    <div className="flex items-center gap-2 flex-wrap pt-0.5">
                      {tx.direction === 'expense' ? (
                        <>
                          <StatusBadge
                            active={tx.uploaded}
                            activeLabel={t('transactions.badges.uploaded')}
                            inactiveLabel={t('transactions.badges.notUploaded')}
                            activeClass="bg-purple-100 dark:bg-purple-500/20 text-purple-700 dark:text-purple-300"
                            inactiveClass="bg-gray-100 dark:bg-gray-800 text-gray-400 dark:text-gray-500"
                            onClick={() => handleToggleUpload(tx.id)}
                            disabled={!!togglingAction || (tx.uploaded && tx.reimbursed)}
                            loading={togglingAction?.id === tx.id && togglingAction.type === 'uploaded'}
                            locked={tx.uploaded && tx.reimbursed}
                            lockedTitle={lockTitle}
                          />
                          <StatusBadge
                            active={tx.reimbursed}
                            activeLabel={processedLabel}
                            inactiveLabel={t('transactions.badges.pending')}
                            activeClass="bg-emerald-100 dark:bg-emerald-500/20 text-emerald-700 dark:text-emerald-300"
                            inactiveClass="bg-gray-100 dark:bg-gray-800 text-gray-400 dark:text-gray-500"
                            onClick={() => handleToggle(tx.id)}
                            disabled={!!togglingAction || !tx.uploaded}
                            loading={togglingAction?.id === tx.id && togglingAction.type === 'reimbursed'}
                          />
                        </>
                      ) : (
                        <span className="text-xs text-gray-300 dark:text-gray-600 px-1">{incomeHint}</span>
                      )}
                      <button
                        onClick={() => copyId(tx.id)}
                        className={`ml-auto font-mono text-xs rounded-lg px-2 py-1 transition-all ${copiedId === tx.id
                          ? 'bg-emerald-100 dark:bg-emerald-500/20 text-emerald-500 dark:text-emerald-400'
                          : 'bg-gray-100 dark:bg-gray-800 text-gray-400 dark:text-gray-500'
                          }`}
                      >
                        {copiedId === tx.id ? t('common.copied') : tx.id.slice(0, 8) + '…'}
                      </button>
                    </div>
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </div>

      {/* Desktop Feed */}
      <div className="hidden md:block space-y-2.5">
        {loading ? (
          <div className="divide-y divide-gray-100 dark:divide-gray-800 rounded-2xl bg-white dark:bg-[hsl(260,15%,11%)] border border-gray-100/80 dark:border-gray-800/60 p-4">
            {[0, 1, 2, 3, 4, 5].map(i => <RowSkeleton key={i} />)}
          </div>
        ) : filtered.length === 0 ? (
          <div className="transaction-table-container flex flex-col items-center justify-center py-16 gap-2 rounded-2xl">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5} strokeLinecap="round" strokeLinejoin="round" className="w-12 h-12 text-gray-200 dark:text-gray-700"><path d="M3 8l7.89 5.26a2 2 0 002.22 0L21 8M5 19h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" /></svg>
            <p className="text-sm text-gray-400 dark:text-gray-500">{t('transactions.noRecords')}</p>
          </div>
        ) : (
          <div className="transaction-feed">
            {filtered.map((tx) => {
              const isExpense = tx.direction === 'expense'
              const done = isExpense && tx.reimbursed && tx.uploaded
              const urgent = isExpense && !tx.reimbursed && !tx.uploaded
              return (
                <article
                  key={tx.id}
                  className={`transaction-feed-row group ${done ? 'opacity-55' : ''}`}
                >
                  <div className="min-w-0 flex flex-1 items-center gap-3">
                    <div className={`h-9 w-9 shrink-0 rounded-xl flex items-center justify-center ${tx.direction === 'income' ? 'bg-emerald-50 text-emerald-600 dark:bg-emerald-500/15 dark:text-emerald-300' : 'bg-rose-50 text-rose-600 dark:bg-rose-500/15 dark:text-rose-300'}`}>
                      <span className="text-sm">{tx.direction === 'income' ? '↗' : '↙'}</span>
                    </div>
                    <div className="min-w-0 flex-1">
                      <p className="text-[16px] leading-6 font-semibold text-gray-900 dark:text-gray-100 truncate" title={tx.note || categoryLabel(tx.category)}>
                        {tx.note || categoryLabel(tx.category)}
                      </p>
                      <div className="mt-1 flex flex-wrap items-center gap-1.5 text-xs text-gray-500 dark:text-gray-400">
                        <span className="inline-flex items-center gap-1 rounded-md bg-gray-100 px-1.5 py-0.5"><TagIcon /> {categoryLabel(tx.category)}</span>
                        {tx.account_id && accountMap[tx.account_id] && <span className="inline-flex items-center gap-1 rounded-md bg-gray-100 px-1.5 py-0.5"><BuildingIcon /> {accountMap[tx.account_id]}</span>}
                        {tx.project_id && <span className="inline-flex items-center gap-1 rounded-md bg-gray-100 px-1.5 py-0.5"><FolderIcon /> {tx.project_id}</span>}
                      </div>
                      <div className="mt-1.5 flex items-center gap-1.5 text-[12px] text-gray-400 dark:text-gray-500 tabular-nums">
                        <ClockIcon />
                        <span>{splitTimestamp(tx.occurred_at).date}</span>
                        <span>·</span>
                        <span>{splitTimestamp(tx.occurred_at).time}</span>
                      </div>
                    </div>
                  </div>

                  <div className="ml-4 flex shrink-0 flex-col items-end justify-center gap-1.5 text-right">
                    <div className={`amount text-[20px] leading-6 font-semibold tabular-nums ${tx.direction === 'income' ? 'amount-income text-emerald-600' : 'amount-expense text-rose-600'}`}>
                      {tx.direction === 'income' ? '+' : '−'}{fmt(tx)}
                    </div>
                    {tx.direction === 'expense' ? (
                      <div className="status-block inline-flex flex-col items-end gap-1.5">
                        <div className="inline-flex items-center gap-1.5">
                          <StatusBadge
                            active={tx.uploaded}
                            activeLabel={t('transactions.badges.uploaded')}
                            inactiveLabel={t('transactions.badges.notUploaded')}
                            activeClass="bg-[#DBEAFE] text-[#1D4ED8]"
                            inactiveClass="bg-gray-100 dark:bg-gray-800 text-gray-500 dark:text-gray-400"
                            onClick={() => handleToggleUpload(tx.id)}
                            disabled={!!togglingAction || (tx.uploaded && tx.reimbursed)}
                            loading={togglingAction?.id === tx.id && togglingAction.type === 'uploaded'}
                            locked={tx.uploaded && tx.reimbursed}
                            lockedTitle={lockTitle}
                          />
                          <StatusBadge
                            active={tx.reimbursed}
                            activeLabel={processedLabel}
                            inactiveLabel={t('transactions.badges.pending')}
                            activeClass="bg-[#DCFCE7] text-[#166534]"
                            inactiveClass="bg-gray-100 dark:bg-gray-800 text-gray-500 dark:text-gray-400"
                            onClick={() => handleToggle(tx.id)}
                            disabled={!!togglingAction || !tx.uploaded}
                            loading={togglingAction?.id === tx.id && togglingAction.type === 'reimbursed'}
                          />
                        </div>
                        {(formatLifecycleTimestamp(tx.reimbursed_at) || formatLifecycleTimestamp(tx.reported_at)) && (
                          <span className="status-time text-[12px] text-gray-500 dark:text-gray-400 tabular-nums">
                            {formatLifecycleTimestamp(tx.reimbursed_at) || formatLifecycleTimestamp(tx.reported_at)}
                          </span>
                        )}
                      </div>
                    ) : (
                      <span className="text-[12px] text-gray-400 dark:text-gray-500">{incomeHint}</span>
                    )}
                    <button
                      onClick={() => copyId(tx.id)}
                      title={t('transactions.copyIdTooltip')}
                      className={`font-mono text-[10px] rounded px-1.5 py-0.5 transition-all ${copiedId === tx.id
                        ? 'bg-emerald-100 dark:bg-emerald-500/20 text-emerald-500 dark:text-emerald-400'
                        : 'text-gray-300 dark:text-gray-600 hover:text-violet-400 dark:hover:text-violet-400 hover:bg-violet-50 dark:hover:bg-violet-500/10'
                        }`}
                    >
                      {copiedId === tx.id ? t('common.copied') : tx.id.slice(0, 8) + '…'}
                    </button>
                  </div>
                  {urgent && <span className="absolute left-3 top-3 h-2 w-2 rounded-full bg-amber-400" />}
                </article>
              )
            })}
          </div>
        )}
      </div>
    </div>
  )
}
