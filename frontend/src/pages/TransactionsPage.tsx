import { useState, useMemo, useRef, useCallback, useEffect } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'
import { downloadAttachment, toggleReimbursed, toggleUploaded } from '../api/client'
import type { Attachment, Transaction, Account } from '../api/client'
import { StaggerContainer, StaggerItem, CardSkeleton, RowSkeleton } from '../motion'
import Select from '../components/Select'
import { useAuth } from '../hooks/useAuth'
import { exportTransactionsPDF } from '../utils/exportTransactionsPDF'
import { formatAmount, sumInCNY } from '../utils/format'
import { useExchangeRates } from '../hooks/useExchangeRates'
import { useTransactions } from '../hooks/useTransactions'
import { useAccounts } from '../hooks/useAccounts'
import { useVirtualizer } from '@tanstack/react-virtual'
import { categoryLabel } from '../utils/categoryLabel'
import { useMode } from '../hooks/useMode'
import { useRefreshFinanceData } from '../hooks/useRefreshFinanceData'
import { useAttachmentMutations, useTransactionAttachments } from '../hooks/useAttachments'
import AttachmentUploader from '../components/AttachmentUploader'
import { clampLifecycleTimestamp } from '../utils/timestamp'
import { accountModeForTransactionSource, transactionSourceForMode } from '../utils/accountScope'
import OcrTextDisclosure from '../components/OcrTextDisclosure'
import {
  isTransactionInWorkflowTab,
  type TransactionWorkflowKind,
  type TransactionWorkflowTab,
} from '../utils/transactionWorkflow'

function splitTimestamp(value: string) {
  const normalized = value.includes('T') ? value : value.replace(' ', 'T')
  const parsed = new Date(normalized)
  if (Number.isNaN(parsed.getTime())) {
    const [date = value, time = ''] = value.split(' ')
    return { date, time: time.slice(0, 8) }
  }
  const formatted = clampLifecycleTimestamp(value, value)
  if (!formatted) return { date: value, time: '' }
  const [date, time = ''] = formatted.split(' ')
  return { date, time }
}

const TagIcon = () => <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} className="h-3.5 w-3.5"><path d="m20 12-8 8-8-8 8-8h8z" /><circle cx="16" cy="8" r="1.5" /></svg>
const BuildingIcon = () => <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} className="h-3.5 w-3.5"><path d="M3 21h18" /><path d="M5 21V7l7-3v17" /><path d="M19 21V11l-7-2" /></svg>
const FolderIcon = () => <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} className="h-3.5 w-3.5"><path d="M3 7h6l2 2h10v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" /></svg>
const ClockIcon = () => <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} className="h-3.5 w-3.5"><circle cx="12" cy="12" r="8" /><path d="M12 8v4l3 2" /></svg>
const PaperclipIcon = () => <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" className="h-3.5 w-3.5"><path d="M21.44 11.05 12.25 20.24a6 6 0 0 1-8.49-8.49l9.19-9.19a4 4 0 1 1 5.66 5.66L9.41 17.41a2 2 0 0 1-2.83-2.83l8.49-8.49" /></svg>

function formatFileSize(bytes: number) {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

function AttachmentPanel({ transactionId }: { transactionId: string }) {
  const { t } = useTranslation()
  const { data: attachments = [], isLoading } = useTransactionAttachments(transactionId)
  const mutations = useAttachmentMutations(transactionId)

  async function removeAttachment(attachment: Attachment) {
    try {
      await mutations.remove.mutateAsync(attachment.id)
      toast.success(t('attachments.toast.deleted'))
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      toast.error(msg || t('attachments.toast.failed'))
    }
  }

  async function runOCR(attachment: Attachment) {
    try {
      const updated = await mutations.runOCR.mutateAsync(attachment.id)
      if (updated.ocr_status === 'done') toast.success(t('attachments.ocr.done'))
      else if (updated.ocr_status === 'unavailable') toast.message(t('attachments.ocr.unavailable'))
      else if (updated.ocr_status === 'failed') toast.error(updated.ocr_error || t('attachments.ocr.failed'))
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      toast.error(msg || t('attachments.ocr.failed'))
    }
  }

  return (
    <div className="mt-3 rounded-[8px] border border-[hsl(var(--mode-accent))]/25 bg-[hsl(var(--mode-accent-wash))] p-3">
      <div className="mb-2 flex items-center justify-between gap-3">
        <p className="text-xs font-semibold text-[hsl(var(--mode-accent-strong))]">{t('attachments.title')}</p>
        <span className="font-data text-[10px] font-semibold text-[hsl(var(--mode-accent-strong))]/75">{attachments.length}</span>
      </div>
      <AttachmentUploader transactionId={transactionId} compact />
      <div className="mt-3 space-y-2">
        {isLoading ? (
          <div className="h-10 animate-pulse rounded-lg bg-white/70 dark:bg-gray-800/60" />
        ) : attachments.length === 0 ? (
          <p className="text-xs text-[hsl(var(--muted-foreground))]">{t('attachments.empty')}</p>
        ) : attachments.map((attachment) => (
          <div key={attachment.id} className="rounded-lg bg-white px-3 py-2 text-xs shadow-sm dark:bg-gray-900/45">
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <p className="truncate font-semibold text-gray-700 dark:text-gray-200" title={attachment.original_filename}>{attachment.original_filename}</p>
                <p className="mt-0.5 text-gray-400 dark:text-gray-500">{formatFileSize(attachment.size_bytes)} · {t(`attachments.ocr.status.${attachment.ocr_status}`)}</p>
              </div>
              <div className="flex shrink-0 gap-1">
                <button type="button" onClick={() => downloadAttachment(attachment.id, attachment.original_filename)} className="rounded-[5px] px-2 py-1 font-semibold text-[hsl(var(--mode-accent-strong))] transition-colors hover:bg-[hsl(var(--mode-accent-wash))]">{t('common.download')}</button>
                <button type="button" onClick={() => runOCR(attachment)} disabled={mutations.runOCR.isPending} className="rounded-md px-2 py-1 font-semibold text-cyan-600 hover:bg-cyan-50 disabled:opacity-50 dark:text-cyan-300 dark:hover:bg-cyan-500/10">{t('attachments.ocr.run')}</button>
                <button type="button" onClick={() => removeAttachment(attachment)} disabled={mutations.remove.isPending} className="rounded-md px-2 py-1 font-semibold text-rose-500 hover:bg-rose-50 disabled:opacity-50 dark:text-rose-300 dark:hover:bg-rose-500/10">{t('common.delete')}</button>
              </div>
            </div>
            {attachment.ocr_error && <p className="mt-1 text-rose-500 dark:text-rose-300">{attachment.ocr_error}</p>}
            <OcrTextDisclosure attachment={attachment} />
          </div>
        ))}
      </div>
    </div>
  )
}

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
      className={`inline-flex min-h-8 items-center gap-1 px-3 py-1.5 rounded-full text-xs font-bold whitespace-nowrap transition-all duration-200 ease-in-out transform active:scale-95 ${disabled && !loading ? 'opacity-40 cursor-not-allowed' : ''
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
  const { mode, isWorkMode } = useMode()
  const [searchParams, setSearchParams] = useSearchParams()
  const effectiveSourceFilter = transactionSourceForMode(mode, searchParams.get('source'))

  function selectSource(source: 'personal' | 'company') {
    if (!isWorkMode || source === effectiveSourceFilter) return
    const next = new URLSearchParams(searchParams)
    next.set('source', source)
    setSearchParams(next, { replace: true })
  }

  return (
    <TransactionsLedger
      key={`${mode}:${effectiveSourceFilter}`}
      isWorkMode={isWorkMode}
      effectiveSourceFilter={effectiveSourceFilter}
      selectSource={selectSource}
    />
  )
}

function TransactionsLedger({
  isWorkMode,
  effectiveSourceFilter,
  selectSource,
}: {
  isWorkMode: boolean
  effectiveSourceFilter: 'personal' | 'company'
  selectSource: (source: 'personal' | 'company') => void
}) {
  const { t } = useTranslation()
  const { user } = useAuth()
  const { rates } = useExchangeRates()
  const { data: txs = [], isLoading: loading, isError, refetch, isFetching } = useTransactions()
  const accountLookupMode = accountModeForTransactionSource(effectiveSourceFilter)
  const { data: accounts = [] } = useAccounts(accountLookupMode)
  const refreshFinanceData = useRefreshFinanceData()
  const [filter, setFilter] = useState<TransactionWorkflowTab>('all')
  const [filterCategory, setFilterCategory] = useState('')
  const [filterProject, setFilterProject] = useState('')
  const [filterAccount, setFilterAccount] = useState('')
  const [copiedId, setCopiedId] = useState<string | null>(null)
  const [togglingAction, setTogglingAction] = useState<{ id: string; type: 'uploaded' | 'reimbursed' } | null>(null)
  const [optimisticState, setOptimisticState] = useState<Record<string, { uploaded: boolean; reimbursed: boolean }>>({})
  const [attachmentPanelId, setAttachmentPanelId] = useState<string | null>(null)
  const isReimbursementView = isWorkMode && effectiveSourceFilter === 'personal'
  const workflowKind: TransactionWorkflowKind = isReimbursementView ? 'reimbursement' : 'upload'

  const tabLabelAll = t('transactions.reimbursementTabs.all')
  const tabLabelPending = isReimbursementView ? t('transactions.reimbursementTabs.pending') : t('transactions.uploadTabs.pending')
  const tabLabelDone = isReimbursementView ? t('transactions.reimbursementTabs.done') : t('transactions.uploadTabs.done')
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

  const filtered = useMemo(() => txsView
    .filter((t) => isTransactionInWorkflowTab(t, filter, workflowKind))
    .filter((t) => t.source === effectiveSourceFilter)
    .filter((t) => !filterCategory || t.category === filterCategory)
    .filter((t) => !filterProject || (t.project_id ?? '') === filterProject)
    .filter((t) => !filterAccount || t.account_id === filterAccount),
    [txsView, filter, workflowKind, effectiveSourceFilter, filterCategory, filterProject, filterAccount]
  )

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
    estimateSize: () => 176,
    overscan: 8,
    getScrollElement,
    scrollMargin: mobileListRef.current?.offsetTop ?? 0,
  })

  const fmt = (t: Transaction) => formatAmount(t.amount_yuan, t.currency)

  function exportPDF() {
    const parts: string[] = [{ all: tabLabelAll, unreimbursed: tabLabelPending, reimbursed: tabLabelDone }[filter]]
    if (filterCategory) parts.push(`${t('transactions.table.category')}: ${categoryLabel(filterCategory)}`)
    if (filterProject) parts.push(`${t('transactions.table.project')}: ${filterProject}`)
    exportTransactionsPDF(filtered, parts.join(' · '), user, rates, workflowKind, accountMap)
  }

  async function handleToggle(id: string) {
    const tx = txsView.find((t) => t.id === id)
    if (!tx || !isWorkMode || tx.source !== 'personal' || tx.direction !== 'expense') return
    const prev = { uploaded: tx.uploaded, reimbursed: tx.reimbursed }
    const next = { uploaded: tx.uploaded, reimbursed: !tx.reimbursed }
    setOptimisticState((curr) => ({ ...curr, [id]: next }))
    setTogglingAction({ id, type: 'reimbursed' })
    try {
      await toggleReimbursed(id)
      refreshFinanceData()
    } catch {
      setOptimisticState((curr) => ({ ...curr, [id]: prev }))
      toast.error(t('transactions.toast.reimbursedError'))
    } finally {
      setTogglingAction(null)
    }
  }

  async function handleToggleUpload(id: string) {
    // Rollback protection: if uploaded AND reimbursed, must cancel reimburse first
    const tx = txsView.find(t => t.id === id)
    if (!tx) return
    if (isReimbursementView && tx.uploaded && tx.reimbursed) {
      toast.error(t('transactions.toast.cancelReimburseFirst'))
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

  const tabCounts = useMemo(() => ({
    all: txsView.filter(t => t.source === effectiveSourceFilter).length,
    unreimbursed: txsView.filter(t => t.source === effectiveSourceFilter && isTransactionInWorkflowTab(t, 'unreimbursed', workflowKind)).length,
    reimbursed: txsView.filter(t => t.source === effectiveSourceFilter && isTransactionInWorkflowTab(t, 'reimbursed', workflowKind)).length,
  }), [txsView, effectiveSourceFilter, workflowKind])
  const tabs: { key: TransactionWorkflowTab; label: string; count?: number }[] = [
    { key: 'all', label: tabLabelAll, count: tabCounts.all },
    { key: 'unreimbursed', label: tabLabelPending, count: tabCounts.unreimbursed },
    { key: 'reimbursed', label: tabLabelDone, count: tabCounts.reimbursed },
  ]

  const totals = useMemo(() => {
    const incomeItems = filtered.filter(t => t.direction === 'income')
    const expenseItems = filtered.filter(t => t.direction === 'expense')
    return {
      income: formatAmount(sumInCNY(incomeItems, rates), 'CNY'),
      expense: formatAmount(sumInCNY(expenseItems, rates), 'CNY'),
    }
  }, [filtered, rates])
  const totalIncomeStr = totals.income
  const totalExpenseStr = totals.expense

  if (isError) {
    return (
      <div className="ledger-panel flex flex-col items-center justify-center gap-3 py-16" role="alert">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5} className="w-12 h-12 text-rose-300 dark:text-rose-500/70"><path strokeLinecap="round" strokeLinejoin="round" d="M12 9v4m0 4h.01M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0Z" /></svg>
        <div className="text-center">
          <p className="text-sm font-semibold text-gray-700 dark:text-gray-200">{t('transactions.error.title')}</p>
          <p className="text-xs text-gray-400 dark:text-gray-500 mt-1">{t('transactions.error.desc')}</p>
        </div>
        <button
          type="button"
          onClick={() => refetch()}
          disabled={isFetching}
          className="rounded-[7px] bg-[hsl(var(--primary))] px-4 py-2 text-sm font-semibold text-[hsl(var(--primary-foreground))] transition-colors hover:brightness-95 disabled:opacity-50"
        >
          {isFetching ? t('common.loading') : t('common.retry')}
        </button>
      </div>
    )
  }

  return (
    <div className="space-y-5">
      {/* Header */}
      <div className="ledger-rail flex items-center justify-between gap-3 pl-5">
        <div className="min-w-0">
          <h1 className="font-display text-2xl font-semibold tracking-[-0.025em] text-[hsl(var(--foreground))] md:text-[1.75rem]">{t('transactions.title')}</h1>
          <p className="mt-1 hidden text-sm text-[hsl(var(--muted-foreground))] sm:block">{t('transactions.subtitle')}</p>
        </div>
        <div className="flex items-center gap-2 shrink-0">
          <button
            onClick={exportPDF}
            disabled={loading || filtered.length === 0}
            className="fin-control flex items-center gap-1.5 px-3.5 py-2.5 text-sm font-semibold text-[hsl(var(--foreground))] disabled:cursor-not-allowed disabled:opacity-40"
            title={t('transactions.exportTooltip')}
          >
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M12 10v6m0 0l-3-3m3 3l3-3M3 17V7a2 2 0 012-2h6l2 2h4a2 2 0 012 2v8a2 2 0 01-2 2H5a2 2 0 01-2-2z" />
            </svg>
            <span className="hidden sm:inline">{t('transactions.exportPdf')}</span>
          </button>
          <Link
            to={`/add?source=${effectiveSourceFilter}`}
            className="shrink-0 rounded-[7px] bg-[hsl(var(--primary))] px-4 py-2.5 text-sm font-semibold text-[hsl(var(--primary-foreground))] transition-colors hover:brightness-95"
          >
            {t('common.add')}
          </Link>
        </div>
      </div>

      {/* Summary cards */}
      <StaggerContainer className="grid grid-cols-3 gap-2 md:gap-3">
        <StaggerItem>
          <div className="ledger-panel border-t-2 border-t-[hsl(var(--mode-accent))] p-3 md:p-4">
            <p className="page-kicker mb-1 md:mb-2">{t('transactions.summary.filtered')}</p>
            <p className="font-data text-lg font-semibold text-[hsl(var(--foreground))] md:text-2xl">{filtered.length} <span className="font-body text-sm font-normal text-[hsl(var(--muted-foreground))] md:text-base">{t('transactions.unit')}</span></p>
          </div>
        </StaggerItem>
        <StaggerItem>
          <div className="ledger-panel border-t-2 border-t-[hsl(var(--income))] p-3 md:p-4">
            <p className="page-kicker mb-1 md:mb-2">{t('transactions.summary.income')}</p>
            <p className="font-data truncate text-sm font-semibold tabular-nums text-[hsl(var(--income))] md:text-2xl">{totalIncomeStr}</p>
          </div>
        </StaggerItem>
        <StaggerItem>
          <div className="ledger-panel border-t-2 border-t-[hsl(var(--expense))] p-3 md:p-4">
            <p className="page-kicker mb-1 md:mb-2">{t('transactions.summary.expense')}</p>
            <p className="font-data truncate text-sm font-semibold tabular-nums text-[hsl(var(--expense))] md:text-2xl">{totalExpenseStr}</p>
          </div>
        </StaggerItem>
      </StaggerContainer>

      {/* Tabs */}
      <div role="group" aria-label={t('transactions.workflowFilterLabel')} className="flex w-full gap-0.5 overflow-x-auto rounded-[8px] border border-[hsl(var(--border))] bg-[hsl(var(--muted))] p-1 sm:w-fit">
        {tabs.map((tb) => (
          <button
            key={tb.key}
            onClick={() => setFilter(tb.key)}
            aria-pressed={filter === tb.key}
            className={`flex items-center gap-1.5 rounded-[6px] px-3 py-1.5 text-sm font-medium transition-colors ${filter === tb.key ? 'bg-[hsl(var(--card))] text-[hsl(var(--mode-accent-strong))] shadow-[0_1px_2px_rgb(18_32_26_/_0.08)]' : 'text-[hsl(var(--muted-foreground))] hover:text-[hsl(var(--foreground))]'
              }`}
          >
            {tb.label}
            {tb.count !== undefined && (
              <span className={`rounded px-1.5 py-0.5 font-data text-xs font-semibold tabular-nums ${filter === tb.key ? 'bg-[hsl(var(--mode-accent-wash))] text-[hsl(var(--mode-accent-strong))]' : 'bg-[hsl(var(--border))]/60 text-[hsl(var(--muted-foreground))]'
                }`}>{tb.count}</span>
            )}
          </button>
        ))}
      </div>

      {/* Filters */}
      <div className="flex items-center gap-2 flex-wrap">
        {/* Source filter */}
        {isWorkMode ? (
          <div role="group" aria-label={t('transactions.sourceFilterLabel')} className="inline-flex h-8 items-center gap-0.5 rounded-lg border border-gray-200 bg-gray-100 p-0.5 dark:border-gray-700 dark:bg-gray-800">
            {(['company', 'personal'] as const).map((source) => (
              <button
                key={source}
                type="button"
                onClick={() => selectSource(source)}
                aria-pressed={effectiveSourceFilter === source}
                className={`h-7 rounded-md px-2.5 text-xs font-semibold transition-colors ${effectiveSourceFilter === source
                  ? source === 'company'
                    ? 'bg-white text-sky-600 shadow-sm dark:bg-gray-700 dark:text-sky-400'
                    : 'bg-white text-amber-600 shadow-sm dark:bg-gray-700 dark:text-amber-400'
                  : 'text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200'
                }`}
              >
                {t(`transactions.sourceTabs.${source}`)}
              </button>
            ))}
          </div>
        ) : (
          <div className="h-8 px-2.5 inline-flex items-center rounded-lg border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-800 text-xs font-medium text-gray-500 dark:text-gray-400">
            {t('common.personal')}
          </div>
        )}

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
              const done = isExpense && (isReimbursementView ? tx.reimbursed && tx.uploaded : tx.uploaded)
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
                    className={`ledger-panel border-l-2 p-4 transition-colors ${tx.direction === 'income' ? 'border-l-[hsl(var(--income))]' : 'border-l-[hsl(var(--expense))]'} ${done ? 'bg-[hsl(var(--muted))]/55' : ''
                      }`}
                  >
                    {/* Row 1: category + amount */}
                    <div className="flex items-start justify-between gap-3 mb-2.5">
                      <div className="flex items-center gap-1.5 min-w-0 pt-0.5 overflow-hidden">
                        <span className={`w-2.5 h-2.5 rounded-full shrink-0 ${tx.direction === 'income' ? 'bg-emerald-400' : 'bg-rose-400'
                          }`} />
                        <span className="font-semibold text-gray-800 dark:text-gray-200 text-sm truncate min-w-0">{categoryLabel(tx.category)}</span>
                        {tx.project_id && (
                          <span className="inline-flex max-w-[96px] shrink-0 items-center gap-1 truncate rounded border border-[hsl(var(--border))] bg-[hsl(var(--muted))] px-1.5 py-0.5 font-data text-xs font-medium text-[hsl(var(--foreground))]" title={tx.project_id}>
                            <FolderIcon /> <span className="truncate">{tx.project_id}</span>
                          </span>
                        )}
                      </div>
                      <span className={`font-data ml-2 shrink-0 text-base font-semibold tabular-nums ${tx.direction === 'income' ? 'text-[hsl(var(--income))]' : 'text-[hsl(var(--expense))]'
                        }`}>
                        {tx.direction === 'income' ? '+' : '−'}{fmt(tx)}
                      </span>
                    </div>
                    {/* Row 2: date + source + account */}
                    <div className="flex items-center gap-1.5 flex-nowrap overflow-hidden mb-2">
                      <span className="text-xs text-gray-400 dark:text-gray-500 tabular-nums shrink-0">{splitTimestamp(tx.occurred_at).date}</span>
                      <span className="text-[11px] text-gray-300 dark:text-gray-600 shrink-0">·</span>
                      <span className="text-xs text-gray-400 dark:text-gray-500 tabular-nums shrink-0">{splitTimestamp(tx.occurred_at).time}</span>
                      <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-semibold shrink-0 ${tx.source === 'company' ? 'bg-sky-50 dark:bg-sky-500/15 text-sky-600 dark:text-sky-400' : 'bg-amber-50 dark:bg-amber-500/15 text-amber-600 dark:text-amber-400'
                        }`}>
                        {tx.source === 'company' ? t('common.company') : t('common.personal')}
                      </span>
                      {tx.account_id && accountMap[tx.account_id] && (
                        <span className="inline-flex max-w-[108px] min-w-0 items-center gap-1 truncate rounded border border-[hsl(var(--mode-accent))]/25 bg-[hsl(var(--mode-accent-wash))] px-1.5 py-0.5 text-xs font-medium text-[hsl(var(--mode-accent-strong))]" title={accountMap[tx.account_id]}>
                          <BuildingIcon /> <span className="truncate">{accountMap[tx.account_id]}</span>
                        </span>
                      )}
                    </div>
                    {/* Row 3: note */}
                    <p className="text-xs text-gray-400 dark:text-gray-500 mb-2.5 truncate min-h-4" title={tx.note || undefined}>{tx.note || ' '}</p>
                    {/* Row 4: action badges + copy ID */}
                    <div className="flex items-center gap-2 flex-nowrap pt-1 overflow-hidden">
                      {tx.direction === 'expense' ? (
                        <>
                          <StatusBadge
                            active={tx.uploaded}
                            activeLabel={t('transactions.badges.uploaded')}
                            inactiveLabel={t('transactions.badges.notUploaded')}
                            activeClass="bg-[hsl(var(--mode-accent-wash))] text-[hsl(var(--mode-accent-strong))]"
                            inactiveClass="bg-gray-100 dark:bg-gray-800 text-gray-400 dark:text-gray-500"
                            onClick={() => handleToggleUpload(tx.id)}
                            disabled={!!togglingAction || (isReimbursementView && tx.uploaded && tx.reimbursed)}
                            loading={togglingAction?.id === tx.id && togglingAction.type === 'uploaded'}
                            locked={isReimbursementView && tx.uploaded && tx.reimbursed}
                            lockedTitle={t('transactions.lockTitle')}
                          />
                          {isReimbursementView && <StatusBadge
                            active={tx.reimbursed}
                            activeLabel={t('transactions.badges.reimbursed')}
                            inactiveLabel={t('transactions.badges.pending')}
                            activeClass="bg-emerald-100 dark:bg-emerald-500/20 text-emerald-700 dark:text-emerald-300"
                            inactiveClass="bg-gray-100 dark:bg-gray-800 text-gray-400 dark:text-gray-500"
                            onClick={() => handleToggle(tx.id)}
                            disabled={!!togglingAction || !tx.uploaded}
                            loading={togglingAction?.id === tx.id && togglingAction.type === 'reimbursed'}
                          />}
                        </>
                      ) : (
                        <span className="min-h-8 inline-flex items-center min-w-0 truncate text-xs text-gray-300 dark:text-gray-600 px-1">{incomeHint}</span>
                      )}
                      <button
                        type="button"
                        onClick={() => setAttachmentPanelId(attachmentPanelId === tx.id ? null : tx.id)}
                        className={`ml-auto shrink-0 min-h-8 inline-flex items-center gap-1 rounded-lg px-2.5 py-1 text-xs font-semibold transition-all ${attachmentPanelId === tx.id || tx.has_attachment
                          ? 'bg-[hsl(var(--mode-accent-wash))] text-[hsl(var(--mode-accent-strong))]'
                          : 'bg-gray-100 text-gray-400 dark:bg-gray-800 dark:text-gray-500'
                          }`}
                        title={t('attachments.title')}
                      >
                        <PaperclipIcon />
                        <span>{t('attachments.short')}</span>
                      </button>
                      <button
                        onClick={() => copyId(tx.id)}
                        title={t('transactions.copyIdTooltip')}
                        className={`shrink-0 min-h-8 font-mono text-xs rounded-lg px-2.5 py-1 transition-all ${copiedId === tx.id
                          ? 'bg-emerald-100 dark:bg-emerald-500/20 text-emerald-500 dark:text-emerald-400'
                          : 'bg-gray-100 dark:bg-gray-800 text-gray-400 dark:text-gray-500'
                          }`}
                      >
                        {copiedId === tx.id ? t('common.copied') : tx.id.slice(0, 8) + '…'}
                      </button>
                    </div>
                    {attachmentPanelId === tx.id && <AttachmentPanel transactionId={tx.id} />}
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
          <div className="ledger-panel divide-y divide-[hsl(var(--border))] p-4">
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
              const done = isExpense && (isReimbursementView ? tx.reimbursed && tx.uploaded : tx.uploaded)
              const urgent = isExpense && !tx.uploaded
              return (
                <div key={tx.id} className="space-y-2">
                  <article
                    className={`transaction-feed-row group ${done ? 'is-complete' : ''}`}
                  >
                  <div className="min-w-0 flex flex-1 items-center gap-3">
                    <div className={`h-9 w-9 shrink-0 rounded-xl flex items-center justify-center ${tx.direction === 'income' ? 'bg-emerald-50 text-emerald-600 dark:bg-emerald-500/15 dark:text-emerald-300' : 'bg-rose-50 text-rose-600 dark:bg-rose-500/15 dark:text-rose-300'}`}>
                      <span className="text-sm">{tx.direction === 'income' ? '↗' : '↙'}</span>
                    </div>
                    <div className="min-w-0 flex-1">
                      <p className="text-[16px] leading-6 font-semibold text-gray-900 dark:text-gray-100 truncate" title={tx.note || categoryLabel(tx.category)}>
                        {tx.note || categoryLabel(tx.category)}
                      </p>
                      <div className="mt-1 flex flex-wrap items-center gap-1.5 text-xs">
                        <span className="inline-flex items-center gap-1 rounded-md bg-gray-50 dark:bg-gray-800/80 text-gray-600 dark:text-gray-300 font-medium px-1.5 py-0.5 border border-gray-200/50 dark:border-gray-700/50 shadow-sm"><TagIcon /> {categoryLabel(tx.category)}</span>
                        {tx.account_id && accountMap[tx.account_id] && <span className="inline-flex items-center gap-1 rounded border border-[hsl(var(--mode-accent))]/25 bg-[hsl(var(--mode-accent-wash))] px-1.5 py-0.5 font-medium text-[hsl(var(--mode-accent-strong))]"><BuildingIcon /> {accountMap[tx.account_id]}</span>}
                        {tx.project_id && <span className="inline-flex items-center gap-1 rounded border border-[hsl(var(--border))] bg-[hsl(var(--muted))] px-1.5 py-0.5 font-data font-medium text-[hsl(var(--foreground))]"><FolderIcon /> {tx.project_id}</span>}
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
                    <div className={`amount font-data text-[20px] font-semibold leading-6 tabular-nums ${tx.direction === 'income' ? 'amount-income text-[hsl(var(--income))]' : 'amount-expense text-[hsl(var(--expense))]'}`}>
                      {tx.direction === 'income' ? '+' : '−'}{fmt(tx)}
                    </div>
                    {tx.direction === 'expense' ? (
                      <div className="status-block inline-flex flex-col items-end gap-1.5">
                        <div className="inline-flex items-center gap-1.5">
                          <StatusBadge
                            active={tx.uploaded}
                            activeLabel={t('transactions.badges.uploaded')}
                            inactiveLabel={t('transactions.badges.notUploaded')}
                            activeClass="bg-[hsl(var(--mode-accent-wash))] text-[hsl(var(--mode-accent-strong))]"
                            inactiveClass="bg-gray-100 dark:bg-gray-800 text-gray-500 dark:text-gray-400"
                            onClick={() => handleToggleUpload(tx.id)}
                            disabled={!!togglingAction || (isReimbursementView && tx.uploaded && tx.reimbursed)}
                            loading={togglingAction?.id === tx.id && togglingAction.type === 'uploaded'}
                            locked={isReimbursementView && tx.uploaded && tx.reimbursed}
                            lockedTitle={t('transactions.lockTitle')}
                          />
                          {isReimbursementView && <StatusBadge
                            active={tx.reimbursed}
                            activeLabel={t('transactions.badges.reimbursed')}
                            inactiveLabel={t('transactions.badges.pending')}
                            activeClass="bg-[hsl(var(--income))]/10 text-[hsl(var(--income))]"
                            inactiveClass="bg-gray-100 dark:bg-gray-800 text-gray-500 dark:text-gray-400"
                            onClick={() => handleToggle(tx.id)}
                            disabled={!!togglingAction || !tx.uploaded}
                            loading={togglingAction?.id === tx.id && togglingAction.type === 'reimbursed'}
                          />}
                        </div>
                        {(clampLifecycleTimestamp(tx.reimbursed_at, tx.created_at) || clampLifecycleTimestamp(tx.reported_at, tx.created_at)) && (
                          <span className="status-time text-[12px] text-gray-500 dark:text-gray-400 tabular-nums">
                            {clampLifecycleTimestamp(tx.reimbursed_at, tx.created_at) || clampLifecycleTimestamp(tx.reported_at, tx.created_at)}
                          </span>
                        )}
                      </div>
                    ) : (
                      <span className="text-[12px] text-gray-400 dark:text-gray-500">{incomeHint}</span>
                    )}
                    <div className="flex items-center gap-1.5">
                      <button
                        type="button"
                        onClick={() => setAttachmentPanelId(attachmentPanelId === tx.id ? null : tx.id)}
                        className={`inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-semibold transition-all ${attachmentPanelId === tx.id || tx.has_attachment
                          ? 'bg-[hsl(var(--mode-accent-wash))] text-[hsl(var(--mode-accent-strong))]'
                          : 'text-[hsl(var(--muted-foreground))]/55 hover:bg-[hsl(var(--mode-accent-wash))] hover:text-[hsl(var(--mode-accent-strong))]'
                        }`}
                        title={t('attachments.title')}
                      >
                        <PaperclipIcon />
                        {t('attachments.short')}
                      </button>
                      <button
                        onClick={() => copyId(tx.id)}
                        title={t('transactions.copyIdTooltip')}
                        className={`font-mono text-[10px] rounded px-1.5 py-0.5 transition-all ${copiedId === tx.id
                          ? 'bg-emerald-100 dark:bg-emerald-500/20 text-emerald-500 dark:text-emerald-400'
                          : 'text-[hsl(var(--muted-foreground))]/55 hover:bg-[hsl(var(--mode-accent-wash))] hover:text-[hsl(var(--mode-accent-strong))]'
                          }`}
                      >
                        {copiedId === tx.id ? t('common.copied') : tx.id.slice(0, 8) + '…'}
                      </button>
                    </div>
                  </div>
                    {urgent && <span className="absolute left-3 top-3 h-2 w-2 rounded-full bg-amber-400" />}
                  </article>
                  {attachmentPanelId === tx.id && <AttachmentPanel transactionId={tx.id} />}
                </div>
              )
            })}
          </div>
        )}
      </div>
    </div>
  )
}
