import { useState, useMemo, useRef, useCallback } from 'react'
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
      className={`inline-flex items-center gap-1 px-3 py-1.5 rounded-full text-xs font-bold whitespace-nowrap transition-all duration-200 ease-in-out transform active:scale-95 ${
        disabled && !loading ? 'opacity-40 cursor-not-allowed' : ''
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
  const [togglingId, setTogglingId] = useState<string | null>(null)
  const effectiveSourceFilter: 'personal' | 'company' = isWorkMode ? 'company' : 'personal'

  const tabLabelAll = t('transactions.reimbursementTabs.all')
  const tabLabelPending = isWorkMode ? t('transactions.reimbursementTabs.pending') : t('transactions.life.tabs.pending')
  const tabLabelDone = isWorkMode ? t('transactions.reimbursementTabs.done') : t('transactions.life.tabs.done')
  const processedLabel = isWorkMode ? t('transactions.badges.reimbursed') : t('transactions.life.badges.processed')
  const processedHeader = isWorkMode ? t('transactions.table.reimbursed') : t('transactions.life.table.processed')
  const lockTitle = isWorkMode ? t('transactions.lockTitle') : t('transactions.life.lockTitle')
  const incomeHint = isWorkMode ? t('transactions.incomeNoReimburse') : t('transactions.life.incomeNoProcess')

  // Build account lookup for filter display
  const activeAccounts = useMemo(() =>
    accounts.filter((a: Account) => a.is_active),
    [accounts]
  )

  // Filter accounts by selected source tab
  const filteredAccounts = useMemo(() => {
    const acctType = effectiveSourceFilter === 'company' ? 'public' : 'personal'
    return activeAccounts.filter((a: Account) => a.type === acctType)
  }, [activeAccounts, effectiveSourceFilter])

  // Filtering (inline — no hooks; must be before useWindowVirtualizer)
  const baseFiltered = txs.filter((t) => {
    if (filter === 'unreimbursed') return !t.reimbursed
    if (filter === 'reimbursed') return t.reimbursed
    return true
  })
  const filtered = baseFiltered
    .filter((t) => t.source === effectiveSourceFilter)
    .filter((t) => !filterCategory || t.category === filterCategory)
    .filter((t) => !filterProject || (t.project_id ?? '') === filterProject)
    .filter((t) => !filterAccount || t.account_id === filterAccount)

  const allCategories = useMemo(
    () => Array.from(new Set(txs.filter(t => t.source === effectiveSourceFilter).map(t => t.category).filter(Boolean))).sort() as string[],
    [txs, effectiveSourceFilter]
  )
  const allProjects = useMemo(
    () => Array.from(new Set(txs.filter(t => t.source === effectiveSourceFilter).map(t => t.project_id).filter((p): p is string => !!p))).sort(),
    [txs, effectiveSourceFilter]
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
    exportTransactionsPDF(filtered, parts.join(' · '), user, rates)
  }

  async function handleToggle(id: string) {
    setTogglingId(id)
    try {
      await toggleReimbursed(id)
      refreshFinanceData()
    } catch {
      toast.error(isWorkMode ? t('transactions.toast.reimbursedError') : t('transactions.life.toast.processError'))
    } finally {
      setTogglingId(null)
    }
  }

  async function handleToggleUpload(id: string) {
    // Rollback protection: if uploaded AND reimbursed, must cancel reimburse first
    const tx = txs.find(t => t.id === id)
    if (tx && tx.uploaded && tx.reimbursed) {
      toast.error(isWorkMode ? t('transactions.toast.cancelReimburseFirst') : t('transactions.life.toast.cancelProcessFirst'))
      return
    }
    setTogglingId(id)
    try {
      await toggleUploaded(id)
      refreshFinanceData()
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      toast.error(msg || t('transactions.toast.uploadError'))
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
    { key: 'all', label: tabLabelAll, count: txs.length },
    { key: 'unreimbursed', label: tabLabelPending, count: txs.filter(t => !t.reimbursed).length },
    { key: 'reimbursed', label: tabLabelDone, count: txs.filter(t => t.reimbursed).length },
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
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm font-medium transition-all ${
              filter === tb.key ? 'bg-white dark:bg-gray-700 shadow text-violet-600 dark:text-violet-400' : 'text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200'
            }`}
          >
            {tb.label}
            {tb.count !== undefined && (
              <span className={`text-xs rounded-full px-1.5 py-0.5 font-semibold tabular-nums ${
                filter === tb.key ? 'bg-violet-100 dark:bg-violet-500/20 text-violet-600 dark:text-violet-400' : 'bg-gray-200 dark:bg-gray-700 text-gray-500 dark:text-gray-400'
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
            {[0,1,2,3].map(i => <CardSkeleton key={i} />)}
          </div>
        ) : filtered.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12 gap-2">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5} strokeLinecap="round" strokeLinejoin="round" className="w-12 h-12 text-gray-200 dark:text-gray-700"><path d="M3 8l7.89 5.26a2 2 0 002.22 0L21 8M5 19h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"/></svg>
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
                    className={`bg-white dark:bg-[hsl(260,15%,11%)] rounded-2xl border border-gray-100/80 dark:border-gray-800/50 p-4 shadow-sm transition-opacity ${
                      done ? 'opacity-40' : ''
                    }`}
                  >
                    {/* Row 1: category + amount */}
                    <div className="flex items-start justify-between gap-2 mb-2.5">
                      <div className="flex items-center gap-1.5 min-w-0 pt-0.5">
                        <span className={`w-2.5 h-2.5 rounded-full shrink-0 ${
                          tx.direction === 'income' ? 'bg-emerald-400' : 'bg-rose-400'
                        }`} />
                        <span className="font-semibold text-gray-800 dark:text-gray-200 text-sm truncate">{categoryLabel(tx.category)}</span>
                        {tx.project_id && (
                          <span className="text-xs font-mono bg-gray-100 dark:bg-gray-800 text-gray-500 dark:text-gray-400 px-1.5 py-0.5 rounded shrink-0">
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
                      <span className="text-xs text-gray-400 dark:text-gray-500 tabular-nums">{tx.occurred_at}</span>
                      <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-semibold ${
                        tx.source === 'company' ? 'bg-sky-50 dark:bg-sky-500/15 text-sky-600 dark:text-sky-400' : 'bg-amber-50 dark:bg-amber-500/15 text-amber-600 dark:text-amber-400'
                      }`}>
                        {tx.source === 'company' ? t('common.company') : t('common.personal')}
                      </span>
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
                            disabled={togglingId === tx.id || (tx.uploaded && tx.reimbursed)}
                            loading={togglingId === tx.id}
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
                            disabled={togglingId === tx.id || !tx.uploaded}
                            loading={togglingId === tx.id}
                          />
                        </>
                      ) : (
                        <span className="text-xs text-gray-300 dark:text-gray-600 px-1">{incomeHint}</span>
                      )}
                      <button
                        onClick={() => copyId(tx.id)}
                        className={`ml-auto font-mono text-xs rounded-lg px-2 py-1 transition-all ${
                          copiedId === tx.id
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

{/* Desktop Table */}
      <div className="hidden md:block bg-white dark:bg-[hsl(260,15%,11%)] rounded-2xl border border-gray-100/80 dark:border-gray-800/50 shadow-sm overflow-hidden">
        {loading ? (
          <div className="divide-y divide-gray-100 dark:divide-gray-800">
            {[0,1,2,3,4,5].map(i => <RowSkeleton key={i} />)}
          </div>
        ) : filtered.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-16 gap-2">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5} strokeLinecap="round" strokeLinejoin="round" className="w-12 h-12 text-gray-200 dark:text-gray-700"><path d="M3 8l7.89 5.26a2 2 0 002.22 0L21 8M5 19h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"/></svg>
            <p className="text-sm text-gray-400 dark:text-gray-500">{t('transactions.noRecords')}</p>
          </div>
        ) : (
          <div className="overflow-x-auto xl:overflow-visible">
            <table className="w-full table-fixed" style={{ minWidth: '1120px' }}>
              <colgroup>
                <col style={{ width: '124px' }} />
                <col style={{ width: '170px' }} />
                <col style={{ width: '130px' }} />
                <col />
                <col style={{ width: '110px' }} />
                <col style={{ width: '140px' }} />
                <col style={{ width: '120px' }} />
                <col style={{ width: '120px' }} />
              </colgroup>
              <thead>
                <tr className="bg-gradient-to-r from-gray-50/90 to-gray-50/50 dark:from-gray-800/50 dark:to-gray-800/20 border-b border-gray-100 dark:border-gray-800">
                  <th className="px-4 py-3 text-left text-[11px] font-semibold text-gray-400 dark:text-gray-500 uppercase tracking-wider">{t('transactions.table.date')}</th>
                  <th className="px-3 py-3 text-left text-[11px] font-semibold text-gray-400 dark:text-gray-500 uppercase tracking-wider">{t('transactions.table.category')}</th>
                  <th className="px-3 py-3 text-left text-[11px] font-semibold text-gray-400 dark:text-gray-500 uppercase tracking-wider">{t('transactions.table.project')}</th>
                  <th className="px-3 py-3 text-left text-[11px] font-semibold text-gray-400 dark:text-gray-500 uppercase tracking-wider">{t('transactions.table.note')}</th>
                  <th className="px-3 py-3 text-left text-[11px] font-semibold text-gray-400 dark:text-gray-500 uppercase tracking-wider">{t('transactions.table.source')}</th>
                  <th className="px-3 py-3 text-right text-[11px] font-semibold text-gray-400 dark:text-gray-500 uppercase tracking-wider">{t('transactions.table.amount')}</th>
                  <th className="px-3 py-3 text-center text-[11px] font-semibold text-gray-400 dark:text-gray-500 uppercase tracking-wider">{t('transactions.table.uploaded')}</th>
                  <th className="px-3 py-3 text-center text-[11px] font-semibold text-gray-400 dark:text-gray-500 uppercase tracking-wider pr-4">{processedHeader}</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((tx) => {
                  const isExpense = tx.direction === 'expense'
                  const done = isExpense && tx.reimbursed && tx.uploaded
                  const urgent = isExpense && !tx.reimbursed && !tx.uploaded
                  return (
                    <tr
                      key={tx.id}
                      className={`border-b border-gray-100/80 dark:border-gray-800/50 last:border-0 transition-colors ${
                        done ? 'opacity-40' : 'hover:bg-violet-50/30 dark:hover:bg-violet-500/5'
                      }`}
                    >
                      <td className="px-4 py-3 whitespace-nowrap">
                        <p className="text-[13px] font-medium text-gray-700 dark:text-gray-300 tabular-nums leading-tight">{tx.occurred_at}</p>
                        <button
                          onClick={() => copyId(tx.id)}
                          title={t('transactions.copyIdTooltip')}
                          className={`font-mono text-[10px] rounded px-1 py-0.5 transition-all mt-0.5 block leading-none ${
                            copiedId === tx.id
                              ? 'bg-emerald-100 dark:bg-emerald-500/20 text-emerald-500 dark:text-emerald-400'
                              : 'text-gray-300 dark:text-gray-600 hover:text-violet-400 dark:hover:text-violet-400 hover:bg-violet-50 dark:hover:bg-violet-500/10'
                          }`}
                        >
                          {copiedId === tx.id ? t('common.copied') : tx.id.slice(0, 8) + '…'}
                        </button>
                      </td>
                      <td className="px-3 py-3">
                        <span className={`inline-flex items-center gap-1.5 text-[13px] font-medium ${urgent ? 'text-gray-900 dark:text-gray-100' : 'text-gray-700 dark:text-gray-300'}`}>
                          <span className={`w-2 h-2 rounded-full shrink-0 ${tx.direction === 'income' ? 'bg-emerald-500' : 'bg-rose-500'}`} />
                          <span className="truncate">{categoryLabel(tx.category)}</span>
                        </span>
                      </td>
                      <td className="px-3 py-3">
                        {tx.project_id
                          ? <span className="inline-flex items-center text-[11px] font-mono font-semibold bg-violet-50 dark:bg-violet-500/15 text-violet-600 dark:text-violet-400 border border-violet-200/60 dark:border-violet-500/30 px-1.5 py-0.5 rounded-md truncate max-w-full">{tx.project_id}</span>
                          : <span className="text-gray-300 dark:text-gray-600 text-[13px]">—</span>
                        }
                      </td>
                      <td className="px-3 py-3">
                        {tx.note
                          ? <span className="text-[13px] text-gray-500 dark:text-gray-400 truncate block leading-snug" title={tx.note}>{tx.note}</span>
                          : <span className="text-gray-300 dark:text-gray-600 text-[13px]">—</span>
                        }
                      </td>
                      <td className="px-3 py-3 whitespace-nowrap">
                        <span className={`inline-flex items-center px-2 py-0.5 rounded-full text-[11px] font-semibold ${
                          tx.source === 'company'
                            ? 'bg-sky-50 dark:bg-sky-500/15 text-sky-600 dark:text-sky-400'
                            : 'bg-amber-50 dark:bg-amber-500/15 text-amber-600 dark:text-amber-400'
                        }`}>
                          {tx.source === 'company' ? t('common.company') : t('common.personal')}
                        </span>
                      </td>
                      <td className={`px-3 py-3 text-right font-bold text-[13px] tabular-nums whitespace-nowrap ${tx.direction === 'income' ? 'text-emerald-500' : 'text-rose-500'}`} style={{ letterSpacing: '-0.02em' }}>
                        {tx.direction === 'income' ? '+' : '−'}{fmt(tx)}
                      </td>
                          <td className="px-3 py-3 text-center whitespace-nowrap">
                            {tx.direction === 'expense' ? (
                              <StatusBadge
                                active={tx.uploaded}
                                activeLabel={t('transactions.badges.uploaded')}
                                inactiveLabel={t('transactions.badges.notUploaded')}
                                activeClass="bg-purple-100 dark:bg-purple-500/20 text-purple-700 dark:text-purple-300 hover:bg-purple-200 dark:hover:bg-purple-500/30"
                                inactiveClass="bg-gray-100 dark:bg-gray-800 text-gray-500 dark:text-gray-400 hover:bg-purple-50 dark:hover:bg-purple-500/10 hover:text-purple-600 dark:hover:text-purple-400"
                                onClick={() => handleToggleUpload(tx.id)}
                                disabled={togglingId === tx.id || (tx.uploaded && tx.reimbursed)}
                                loading={togglingId === tx.id}
                                locked={tx.uploaded && tx.reimbursed}
                                lockedTitle={lockTitle}
                              />
                            ) : (
                              <span className="text-gray-300 dark:text-gray-600 text-[13px]">—</span>
                            )}
                          </td>
                          <td className="px-3 py-3 text-center whitespace-nowrap pr-4">
                            {tx.direction === 'expense' ? (
                              <StatusBadge
                                active={tx.reimbursed}
                                activeLabel={processedLabel}
                                inactiveLabel={t('transactions.badges.pending')}
                                activeClass="bg-emerald-100 dark:bg-emerald-500/20 text-emerald-700 dark:text-emerald-300 hover:bg-emerald-200 dark:hover:bg-emerald-500/30"
                                inactiveClass="bg-gray-100 dark:bg-gray-800 text-gray-500 dark:text-gray-400 hover:bg-emerald-50 dark:hover:bg-emerald-500/10 hover:text-emerald-500 dark:hover:text-emerald-400"
                                onClick={() => handleToggle(tx.id)}
                                disabled={togglingId === tx.id || !tx.uploaded}
                                loading={togglingId === tx.id}
                              />
                            ) : (
                              <span className="text-gray-300 dark:text-gray-600 text-[13px]">—</span>
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
