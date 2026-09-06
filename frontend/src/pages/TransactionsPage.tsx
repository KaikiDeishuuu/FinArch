import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useVirtualizer } from '@tanstack/react-virtual'
import {
  AlertTriangle,
  ArrowDownLeft,
  ArrowUpRight,
  Building2,
  Check,
  Clock3,
  Copy,
  Download,
  Folder,
  Lock,
  Paperclip,
  RefreshCw,
  ScanText,
  Tag,
  Trash2,
} from 'lucide-react'
import { toast } from 'sonner'
import { downloadAttachment, toggleReimbursed, toggleSettled, toggleUploaded } from '../api/client'
import type { Account, Attachment, Transaction } from '../api/client'
import AttachmentUploader from '../components/AttachmentUploader'
import OcrTextDisclosure from '../components/OcrTextDisclosure'
import Select from '../components/Select'
import { Badge } from '../components/ui/badge'
import { Button, ButtonLink } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { EmptyState } from '../components/ui/empty-state'
import { PageHeader } from '../components/ui/page-header'
import { Segmented, SegmentedButton } from '../components/ui/segmented'
import { CardSkeleton, RowSkeleton } from '../components/ui/skeleton'
import { Spinner } from '../components/ui/spinner'
import { StatTile } from '../components/ui/stat-tile'
import { useAttachmentMutations, useTransactionAttachments } from '../hooks/useAttachments'
import { useAuth } from '../hooks/useAuth'
import { useExchangeRates } from '../hooks/useExchangeRates'
import { useMode } from '../hooks/useMode'
import { useRefreshFinanceData } from '../hooks/useRefreshFinanceData'
import { useTransactions } from '../hooks/useTransactions'
import { useAccounts } from '../hooks/useAccounts'
import { cn } from '../lib/utils'
import { accountModeForTransactionSource, transactionSourceForMode } from '../utils/accountScope'
import { categoryLabel } from '../utils/categoryLabel'
import { exportTransactionsPDF } from '../utils/exportTransactionsPDF'
import { formatAmount, sumInCNY } from '../utils/format'
import { clampLifecycleTimestamp } from '../utils/timestamp'
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
    <div className="rounded-lg border border-border bg-muted/55 p-3">
      <div className="mb-2 flex items-center justify-between gap-3">
        <p className="flex items-center gap-1.5 text-xs font-semibold text-foreground">
          <Paperclip className="size-3.5 text-muted-foreground" />
          {t('attachments.title')}
        </p>
        <Badge>{attachments.length}</Badge>
      </div>
      <AttachmentUploader transactionId={transactionId} compact />
      <div className="mt-3 space-y-2">
        {isLoading ? (
          <div className="h-10 animate-pulse rounded-md bg-card" />
        ) : attachments.length === 0 ? (
          <p className="text-xs text-muted-foreground">{t('attachments.empty')}</p>
        ) : attachments.map((attachment) => (
          <div key={attachment.id} className="rounded-md border border-border bg-card px-3 py-2 text-xs">
            <div className="flex flex-wrap items-start justify-between gap-2">
              <div className="min-w-0 flex-1">
                <p className="truncate font-semibold text-foreground" title={attachment.original_filename}>
                  {attachment.original_filename}
                </p>
                <p className="mt-0.5 text-muted-foreground">
                  {formatFileSize(attachment.size_bytes)} · {t(`attachments.ocr.status.${attachment.ocr_status}`)}
                </p>
              </div>
              <div className="flex shrink-0 items-center gap-1">
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => downloadAttachment(attachment.id, attachment.original_filename)}
                  className="h-7 px-2"
                >
                  <Download className="size-3.5" />
                  {t('common.download')}
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => runOCR(attachment)}
                  disabled={mutations.runOCR.isPending}
                  className="h-7 px-2"
                >
                  <ScanText className="size-3.5" />
                  {t('attachments.ocr.run')}
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => removeAttachment(attachment)}
                  disabled={mutations.remove.isPending}
                  className="h-7 px-2 text-negative hover:text-negative"
                >
                  <Trash2 className="size-3.5" />
                  {t('common.delete')}
                </Button>
              </div>
            </div>
            {attachment.ocr_error ? <p className="mt-1 text-negative">{attachment.ocr_error}</p> : null}
            <OcrTextDisclosure attachment={attachment} />
          </div>
        ))}
      </div>
    </div>
  )
}

function StatusBadge({
  active,
  activeLabel,
  inactiveLabel,
  activeClass,
  inactiveClass,
  onClick,
  disabled,
  loading,
  locked,
  lockedTitle,
}: {
  active: boolean
  activeLabel: string
  inactiveLabel: string
  activeClass: string
  inactiveClass: string
  onClick: () => void
  disabled?: boolean
  loading?: boolean
  locked?: boolean
  lockedTitle?: string
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      title={locked ? lockedTitle : undefined}
      className={cn(
        'inline-flex min-h-7 items-center gap-1 rounded-full border border-transparent px-2.5 py-1 text-[11px] font-semibold whitespace-nowrap transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
        active ? activeClass : inactiveClass,
        disabled && !loading && 'cursor-not-allowed opacity-45',
        locked && 'cursor-not-allowed border-border',
      )}
    >
      {loading ? (
        <Spinner className="size-3" />
      ) : active ? (
        <Check className="size-3" strokeWidth={2.5} />
      ) : locked ? (
        <Lock className="size-3" />
      ) : (
        <span className="size-2 rounded-full border border-current opacity-60" />
      )}
      {loading ? '…' : active ? activeLabel : inactiveLabel}
    </button>
  )
}

type TogglingAction = { id: string; type: 'uploaded' | 'reimbursed' | 'settled' } | null

interface TransactionRowProps {
  tx: Transaction
  accountMap: Record<string, string>
  isReimbursementView: boolean
  isSettlementView: boolean
  incomeHint: string
  togglingAction: TogglingAction
  attachmentPanelId: string | null
  copiedId: string | null
  formatTransaction: (transaction: Transaction) => string
  handleToggle: (id: string) => void
  handleToggleUpload: (id: string) => void
  handleToggleSettle: (id: string) => void
  toggleAttachmentPanel: (id: string) => void
  copyId: (id: string) => void
  semantic?: boolean
}

function TransactionRow({
  tx,
  accountMap,
  isReimbursementView,
  isSettlementView,
  incomeHint,
  togglingAction,
  attachmentPanelId,
  copiedId,
  formatTransaction,
  handleToggle,
  handleToggleUpload,
  handleToggleSettle,
  toggleAttachmentPanel,
  copyId,
  semantic = true,
}: TransactionRowProps) {
  const { t } = useTranslation()
  const isExpense = tx.direction === 'expense'
  const settleDone = tx.settled && tx.uploaded
  const done = isExpense && (
    isReimbursementView ? tx.reimbursed && tx.uploaded
      : isSettlementView ? settleDone
        : tx.uploaded
  )
  const urgent = isExpense && !tx.uploaded
  const timestamp = splitTimestamp(tx.occurred_at)
  const lifecycleTimestamp = clampLifecycleTimestamp(tx.reimbursed_at, tx.created_at)
    || clampLifecycleTimestamp(tx.settled_at, tx.created_at)
    || clampLifecycleTimestamp(tx.reported_at, tx.created_at)

  const RowElement = semantic ? 'article' : 'div'

  return (
    <div className="space-y-2">
      <RowElement
        className={cn(
          'relative rounded-xl border border-border bg-card p-4 shadow-xs transition-[border-color,opacity] hover:border-input md:flex md:items-center md:gap-4',
          done && 'opacity-55',
        )}
      >
        {urgent ? (
          <span
            aria-label={t('transactions.badges.notUploaded')}
            className="absolute left-3 top-3 size-2 rounded-full bg-warning"
          />
        ) : null}

        <div className="min-w-0 flex-1">
          <div className="flex items-start justify-between gap-3">
            <div className="flex min-w-0 items-start gap-3">
              <span
                aria-hidden="true"
                className={cn(
                  'grid size-9 shrink-0 place-items-center rounded-md',
                  isExpense ? 'bg-negative-soft text-negative' : 'bg-positive-soft text-positive',
                )}
              >
                {isExpense ? <ArrowDownLeft className="size-4" /> : <ArrowUpRight className="size-4" />}
              </span>
              <div className="min-w-0">
                <h2 className="truncate text-sm font-semibold text-foreground" title={tx.note || categoryLabel(tx.category)}>
                  {tx.note || categoryLabel(tx.category)}
                </h2>
                <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
                  <span className="inline-flex items-center gap-1 rounded-md bg-muted px-1.5 py-0.5 text-[11px] font-medium text-muted-foreground">
                    <Tag className="size-3" />
                    {categoryLabel(tx.category)}
                  </span>
                  {tx.account_id && accountMap[tx.account_id] ? (
                    <span
                      className="inline-flex max-w-36 items-center gap-1 truncate rounded-md bg-muted px-1.5 py-0.5 text-[11px] font-medium text-muted-foreground"
                      title={accountMap[tx.account_id]}
                    >
                      <Building2 className="size-3 shrink-0" />
                      <span className="truncate">{accountMap[tx.account_id]}</span>
                    </span>
                  ) : null}
                  {tx.project_id ? (
                    <span
                      className="inline-flex max-w-32 items-center gap-1 truncate rounded-md bg-accent-soft px-1.5 py-0.5 font-mono text-[11px] font-medium text-accent"
                      title={tx.project_id}
                    >
                      <Folder className="size-3 shrink-0" />
                      <span className="truncate">{tx.project_id}</span>
                    </span>
                  ) : null}
                </div>
              </div>
            </div>

            <p className={cn('shrink-0 text-base font-semibold tabular-nums md:hidden', isExpense ? 'text-negative' : 'text-positive')}>
              {isExpense ? '−' : '+'}{formatTransaction(tx)}
            </p>
          </div>

          <div className="mt-2 flex flex-wrap items-center gap-1.5 pl-12 text-[11px] text-muted-foreground">
            <Clock3 className="size-3" />
            <span className="tabular-nums">{timestamp.date}</span>
            <span aria-hidden="true">·</span>
            <span className="tabular-nums">{timestamp.time}</span>
            <Badge className="ml-0.5 h-5 px-1.5 text-[10px]">
              {tx.source === 'company' ? t('common.company') : t('common.personal')}
            </Badge>
          </div>
        </div>

        <div className="mt-3 flex min-w-0 flex-wrap items-center gap-1.5 border-t border-border pt-3 md:mt-0 md:w-[22rem] md:shrink-0 md:justify-end md:border-0 md:pt-0">
          <div className="hidden min-w-28 text-right md:block">
            <p className={cn('text-lg font-semibold tabular-nums', isExpense ? 'text-negative' : 'text-positive')}>
              {isExpense ? '−' : '+'}{formatTransaction(tx)}
            </p>
            {lifecycleTimestamp ? <p className="mt-0.5 text-[10px] tabular-nums text-muted-foreground">{lifecycleTimestamp}</p> : null}
          </div>

          {isExpense ? (
            <>
              <StatusBadge
                active={tx.uploaded}
                activeLabel={t('transactions.badges.uploaded')}
                inactiveLabel={t('transactions.badges.notUploaded')}
                activeClass="bg-accent-soft text-accent"
                inactiveClass="bg-muted text-muted-foreground"
                onClick={() => handleToggleUpload(tx.id)}
                disabled={!!togglingAction || (isReimbursementView && tx.uploaded && tx.reimbursed) || (isSettlementView && settleDone)}
                loading={togglingAction?.id === tx.id && togglingAction.type === 'uploaded'}
                locked={(isReimbursementView && tx.uploaded && tx.reimbursed) || (isSettlementView && settleDone)}
                lockedTitle={isSettlementView ? t('transactions.settleLockTitle') : t('transactions.lockTitle')}
              />
              {isReimbursementView && <StatusBadge
                active={tx.reimbursed}
                activeLabel={t('transactions.badges.reimbursed')}
                inactiveLabel={t('transactions.badges.pending')}
                activeClass="bg-positive-soft text-positive"
                inactiveClass="bg-muted text-muted-foreground"
                onClick={() => handleToggle(tx.id)}
                disabled={!!togglingAction || !tx.uploaded}
                loading={togglingAction?.id === tx.id && togglingAction.type === 'reimbursed'}
              />}
              {isSettlementView && <StatusBadge
                active={tx.settled}
                activeLabel={t('transactions.badges.settled')}
                inactiveLabel={t('transactions.badges.pendingSettlement')}
                activeClass="bg-positive-soft text-positive"
                inactiveClass="bg-muted text-muted-foreground"
                onClick={() => handleToggleSettle(tx.id)}
                disabled={!!togglingAction || !tx.uploaded}
                loading={togglingAction?.id === tx.id && togglingAction.type === 'settled'}
              />}
            </>
          ) : (
            <span className="min-w-0 flex-1 truncate text-xs text-muted-foreground md:max-w-36" title={incomeHint}>
              {incomeHint}
            </span>
          )}

          <button
            type="button"
            onClick={() => toggleAttachmentPanel(tx.id)}
            className={cn(
              'ml-auto inline-flex min-h-7 shrink-0 items-center gap-1 rounded-md px-2 py-1 text-[11px] font-semibold transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring md:ml-0',
              attachmentPanelId === tx.id || tx.has_attachment
                ? 'bg-accent-soft text-accent'
                : 'bg-muted text-muted-foreground hover:text-foreground',
            )}
            title={t('attachments.title')}
            aria-expanded={attachmentPanelId === tx.id}
          >
            <Paperclip className="size-3" />
            {t('attachments.short')}
          </button>
          <button
            type="button"
            onClick={() => copyId(tx.id)}
            title={t('transactions.copyIdTooltip')}
            className={cn(
              'inline-flex min-h-7 shrink-0 items-center gap-1 rounded-md px-2 py-1 font-mono text-[10px] transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
              copiedId === tx.id
                ? 'bg-positive-soft text-positive'
                : 'bg-muted text-muted-foreground hover:text-foreground',
            )}
          >
            <Copy className="size-3" />
            {copiedId === tx.id ? t('common.copied') : `${tx.id.slice(0, 8)}…`}
          </button>
        </div>
      </RowElement>
      {attachmentPanelId === tx.id ? <AttachmentPanel transactionId={tx.id} /> : null}
    </div>
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
  const [togglingAction, setTogglingAction] = useState<TogglingAction>(null)
  const [optimisticState, setOptimisticState] = useState<Record<string, { uploaded: boolean; reimbursed: boolean; settled: boolean }>>({})
  const [attachmentPanelId, setAttachmentPanelId] = useState<string | null>(null)
  const isReimbursementView = isWorkMode && effectiveSourceFilter === 'personal'
  // Public-account WORK expenses clear with finance instead of being reimbursed.
  const isSettlementView = isWorkMode && effectiveSourceFilter === 'company'
  const workflowKind: TransactionWorkflowKind = isReimbursementView
    ? 'reimbursement'
    : isSettlementView ? 'settlement' : 'upload'

  const tabLabelAll = t('transactions.reimbursementTabs.all')
  const tabLabelPending = isReimbursementView
    ? t('transactions.reimbursementTabs.pending')
    : isSettlementView ? t('transactions.settlementTabs.pending') : t('transactions.uploadTabs.pending')
  const tabLabelDone = isReimbursementView
    ? t('transactions.reimbursementTabs.done')
    : isSettlementView ? t('transactions.settlementTabs.done') : t('transactions.uploadTabs.done')
  const incomeHint = isSettlementView
    ? t('transactions.incomeNoSettle')
    : isWorkMode ? t('transactions.incomeNoReimburse') : t('transactions.life.incomeNoProcess')

  const txsView = useMemo(() =>
    txs.map((tx) => {
      const override = optimisticState[tx.id]
      return override
        ? { ...tx, uploaded: override.uploaded, reimbursed: override.reimbursed, settled: override.settled }
        : tx
    }),
    [txs, optimisticState],
  )

  useEffect(() => {
    setOptimisticState((prev) => {
      const next = { ...prev }
      let changed = false
      for (const tx of txs) {
        const state = next[tx.id]
        if (state && state.uploaded === tx.uploaded && state.reimbursed === tx.reimbursed && state.settled === tx.settled) {
          delete next[tx.id]
          changed = true
        }
      }
      return changed ? next : prev
    })
  }, [txs])

  const activeAccounts = useMemo(() =>
    accounts.filter((account: Account) => account.is_active),
    [accounts],
  )

  const accountMap = useMemo(
    () => Object.fromEntries(accounts.map((account: Account) => [account.id, account.name])),
    [accounts],
  )

  const filteredAccounts = useMemo(() => {
    const accountType = effectiveSourceFilter === 'company' ? 'public' : 'personal'
    return activeAccounts.filter((account: Account) => account.type === accountType)
  }, [activeAccounts, effectiveSourceFilter])

  const filtered = useMemo(() => txsView
    .filter((transaction) => isTransactionInWorkflowTab(transaction, filter, workflowKind))
    .filter((transaction) => transaction.source === effectiveSourceFilter)
    .filter((transaction) => !filterCategory || transaction.category === filterCategory)
    .filter((transaction) => !filterProject || (transaction.project_id ?? '') === filterProject)
    .filter((transaction) => !filterAccount || transaction.account_id === filterAccount),
    [txsView, filter, workflowKind, effectiveSourceFilter, filterCategory, filterProject, filterAccount],
  )

  const allCategories = useMemo(
    () => Array.from(new Set(txsView
      .filter((transaction) => transaction.source === effectiveSourceFilter)
      .map((transaction) => transaction.category)
      .filter(Boolean))).sort() as string[],
    [txsView, effectiveSourceFilter],
  )
  const allProjects = useMemo(
    () => Array.from(new Set(txsView
      .filter((transaction) => transaction.source === effectiveSourceFilter)
      .map((transaction) => transaction.project_id)
      .filter((project): project is string => !!project))).sort(),
    [txsView, effectiveSourceFilter],
  )

  const mobileListRef = useRef<HTMLDivElement>(null)
  const getScrollElement = useCallback(
    () => document.querySelector<HTMLElement>('.scroll-main'),
    [],
  )
  const mobileVirtualizer = useVirtualizer({
    count: filtered.length,
    estimateSize: () => 196,
    overscan: 8,
    getScrollElement,
    scrollMargin: mobileListRef.current?.offsetTop ?? 0,
  })

  const formatTransaction = (transaction: Transaction) => formatAmount(transaction.amount_yuan, transaction.currency)

  function exportPDF() {
    const parts: string[] = [{ all: tabLabelAll, unreimbursed: tabLabelPending, reimbursed: tabLabelDone }[filter]]
    if (filterCategory) parts.push(`${t('transactions.table.category')}: ${categoryLabel(filterCategory)}`)
    if (filterProject) parts.push(`${t('transactions.table.project')}: ${filterProject}`)
    exportTransactionsPDF(filtered, parts.join(' · '), user, rates, workflowKind, accountMap)
  }

  async function handleToggle(id: string) {
    const tx = txsView.find((transaction) => transaction.id === id)
    if (!tx || !isWorkMode || tx.source !== 'personal' || tx.direction !== 'expense') return
    const prev = { uploaded: tx.uploaded, reimbursed: tx.reimbursed, settled: tx.settled }
    const next = { uploaded: tx.uploaded, reimbursed: !tx.reimbursed, settled: tx.settled }
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

  async function handleToggleSettle(id: string) {
    const tx = txsView.find((transaction) => transaction.id === id)
    if (!tx || !isWorkMode || tx.source !== 'company' || tx.direction !== 'expense') return
    const prev = { uploaded: tx.uploaded, reimbursed: tx.reimbursed, settled: tx.settled }
    const next = { uploaded: tx.uploaded, reimbursed: tx.reimbursed, settled: !tx.settled }
    setOptimisticState((curr) => ({ ...curr, [id]: next }))
    setTogglingAction({ id, type: 'settled' })
    try {
      await toggleSettled(id)
      refreshFinanceData()
    } catch {
      setOptimisticState((curr) => ({ ...curr, [id]: prev }))
      toast.error(t('transactions.toast.settledError'))
    } finally {
      setTogglingAction(null)
    }
  }

  async function handleToggleUpload(id: string) {
    const tx = txsView.find((transaction) => transaction.id === id)
    if (!tx) return
    if (isReimbursementView && tx.uploaded && tx.reimbursed) {
      toast.error(t('transactions.toast.cancelReimburseFirst'))
      return
    }
    if (isSettlementView && tx.uploaded && tx.settled) {
      toast.error(t('transactions.toast.cancelSettleFirst'))
      return
    }
    const prev = { uploaded: tx.uploaded, reimbursed: tx.reimbursed, settled: tx.settled }
    const next = { uploaded: !tx.uploaded, reimbursed: tx.reimbursed, settled: tx.settled }
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
    void navigator.clipboard.writeText(id).then(() => {
      setCopiedId(id)
      window.setTimeout(() => setCopiedId(null), 1500)
    })
  }

  function toggleAttachmentPanel(id: string) {
    setAttachmentPanelId((current) => current === id ? null : id)
  }

  const tabCounts = useMemo(() => ({
    all: txsView.filter((transaction) => transaction.source === effectiveSourceFilter).length,
    unreimbursed: txsView.filter((transaction) => transaction.source === effectiveSourceFilter
      && isTransactionInWorkflowTab(transaction, 'unreimbursed', workflowKind)).length,
    reimbursed: txsView.filter((transaction) => transaction.source === effectiveSourceFilter
      && isTransactionInWorkflowTab(transaction, 'reimbursed', workflowKind)).length,
  }), [txsView, effectiveSourceFilter, workflowKind])
  const tabs: { key: TransactionWorkflowTab; label: string; count?: number }[] = [
    { key: 'all', label: tabLabelAll, count: tabCounts.all },
    { key: 'unreimbursed', label: tabLabelPending, count: tabCounts.unreimbursed },
    { key: 'reimbursed', label: tabLabelDone, count: tabCounts.reimbursed },
  ]

  const totals = useMemo(() => {
    const incomeItems = filtered.filter((transaction) => transaction.direction === 'income')
    const expenseItems = filtered.filter((transaction) => transaction.direction === 'expense')
    return {
      income: formatAmount(sumInCNY(incomeItems, rates), 'CNY'),
      expense: formatAmount(sumInCNY(expenseItems, rates), 'CNY'),
    }
  }, [filtered, rates])

  const rowProps = {
    accountMap,
    isReimbursementView,
    isSettlementView,
    incomeHint,
    togglingAction,
    attachmentPanelId,
    copiedId,
    formatTransaction,
    handleToggle,
    handleToggleUpload,
    handleToggleSettle,
    toggleAttachmentPanel,
    copyId,
  }

  if (isError) {
    return (
      <EmptyState
        icon={<AlertTriangle />}
        title={t('transactions.error.title')}
        description={t('transactions.error.desc')}
        action={(
          <Button type="button" onClick={() => void refetch()} loading={isFetching} loadingText={t('common.loading')}>
            <RefreshCw className="size-4" />
            {t('common.retry')}
          </Button>
        )}
      />
    )
  }

  return (
    <div className="space-y-5">
      <PageHeader
        title={t('transactions.title')}
        description={t('transactions.subtitle')}
        actions={(
          <>
            <Button
              type="button"
              variant="outline"
              onClick={exportPDF}
              disabled={loading || filtered.length === 0}
              title={t('transactions.exportTooltip')}
            >
              <Download className="size-4" />
              <span className="hidden sm:inline">{t('transactions.exportPdf')}</span>
            </Button>
            <ButtonLink to={`/add?source=${effectiveSourceFilter}`}>
              {t('common.add')}
            </ButtonLink>
          </>
        )}
      />

      <div className="grid grid-cols-3 gap-2 md:gap-3">
        <StatTile
          label={t('transactions.summary.filtered')}
          value={<>{filtered.length} <span className="text-sm font-normal text-muted-foreground">{t('transactions.unit')}</span></>}
        />
        <StatTile
          label={t('transactions.summary.income')}
          value={<span className="block truncate text-sm sm:text-xl">{totals.income}</span>}
          tone="positive"
        />
        <StatTile
          label={t('transactions.summary.expense')}
          value={<span className="block truncate text-sm sm:text-xl">{totals.expense}</span>}
          tone="negative"
        />
      </div>

      <Segmented role="group" aria-label={t('transactions.workflowFilterLabel')} className="w-full overflow-x-auto sm:w-fit">
        {tabs.map((tb) => (
          <SegmentedButton
            key={tb.key}
            onClick={() => setFilter(tb.key)}
            aria-pressed={filter === tb.key}
            className="flex-1 px-3 sm:flex-none"
          >
            {tb.label}
            {tb.count !== undefined ? (
              <span className={cn(
                'rounded-full bg-muted-2 px-1.5 py-0.5 text-[10px] font-semibold tabular-nums',
                filter === tb.key && 'bg-accent-soft text-accent',
              )}>
                {tb.count}
              </span>
            ) : null}
          </SegmentedButton>
        ))}
      </Segmented>

      <Card className="flex flex-wrap items-center gap-2 p-3">
        {isWorkMode ? (
          <Segmented role="group" aria-label={t('transactions.sourceFilterLabel')}>
            {(['company', 'personal'] as const).map((source) => (
              <SegmentedButton
                key={source}
                type="button"
                onClick={() => selectSource(source)}
                aria-pressed={effectiveSourceFilter === source}
                className="aria-pressed:text-mode"
              >
                {t(`transactions.sourceTabs.${source}`)}
              </SegmentedButton>
            ))}
          </Segmented>
        ) : (
          <Badge className="h-8 rounded-lg px-2.5">{t('common.personal')}</Badge>
        )}

        {filteredAccounts.length > 1 ? (
          <div className="w-fit min-w-[7rem]">
            <Select
              value={filterAccount}
              onChange={setFilterAccount}
              placeholder={t('transactions.filterPlaceholders.account')}
              size="sm"
              activeHighlight
              options={[
                { value: '', label: t('transactions.filterPlaceholders.account') },
                ...filteredAccounts.map((account: Account) => ({ value: account.id, label: account.name })),
              ]}
            />
          </div>
        ) : null}

        {allCategories.length > 0 ? (
          <div className="w-fit min-w-[7rem]">
            <Select
              value={filterCategory}
              onChange={setFilterCategory}
              placeholder={t('transactions.filterPlaceholders.category')}
              size="sm"
              activeHighlight
              options={[
                { value: '', label: t('transactions.filterPlaceholders.category') },
                ...allCategories.map((category) => ({ value: category, label: categoryLabel(category) })),
              ]}
            />
          </div>
        ) : null}

        {allProjects.length > 0 ? (
          <div className="w-fit min-w-[7rem]">
            <Select
              value={filterProject}
              onChange={setFilterProject}
              placeholder={t('transactions.filterPlaceholders.project')}
              size="sm"
              activeHighlight
              options={[
                { value: '', label: t('transactions.filterPlaceholders.project') },
                ...allProjects.map((project) => ({ value: project, label: project })),
              ]}
            />
          </div>
        ) : null}

        {filterCategory || filterProject || filterAccount ? (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => {
              setFilterCategory('')
              setFilterProject('')
              setFilterAccount('')
            }}
          >
            {t('common.clear')}
          </Button>
        ) : null}
      </Card>

      <div className="md:hidden">
        {loading ? (
          <div className="space-y-2">
            {[0, 1, 2, 3].map((index) => <CardSkeleton key={index} />)}
          </div>
        ) : filtered.length === 0 ? (
          <EmptyState title={t('transactions.noRecords')} />
        ) : (
          <div
            ref={mobileListRef}
            style={{ height: `${mobileVirtualizer.getTotalSize()}px`, position: 'relative' }}
          >
            {mobileVirtualizer.getVirtualItems().map((virtualItem) => {
              const tx = filtered[virtualItem.index]
              return (
                <div
                  key={virtualItem.key}
                  data-index={virtualItem.index}
                  ref={mobileVirtualizer.measureElement}
                  style={{
                    position: 'absolute',
                    top: 0,
                    left: 0,
                    width: '100%',
                    transform: `translateY(${virtualItem.start - (mobileVirtualizer.options.scrollMargin ?? 0)}px)`,
                    paddingBottom: '8px',
                  }}
                >
                  <TransactionRow tx={tx} {...rowProps} semantic={false} />
                </div>
              )
            })}
          </div>
        )}
      </div>

      <div className="hidden space-y-2 md:block">
        {loading ? (
          <div className="divide-y divide-border rounded-xl border border-border bg-card px-2 py-1">
            {[0, 1, 2, 3, 4, 5].map((index) => <RowSkeleton key={index} />)}
          </div>
        ) : filtered.length === 0 ? (
          <EmptyState title={t('transactions.noRecords')} />
        ) : (
          filtered.map((tx) => <TransactionRow key={tx.id} tx={tx} {...rowProps} />)
        )}
      </div>
    </div>
  )
}
