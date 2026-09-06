import { useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'
import {
  AlertCircle,
  ArrowDown,
  ArrowUp,
  Building2,
  CheckCircle2,
  Paperclip,
  Settings2,
  UserRound,
} from 'lucide-react'
import { createTransaction, deleteAttachment, linkAttachment } from '../api/client'
import type { Attachment, OCRSuggestion } from '../api/client'
import AttachmentUploader from '../components/AttachmentUploader'
import Select from '../components/Select'
import { Alert } from '../components/ui/alert'
import { Button, buttonVariants } from '../components/ui/button'
import { Card } from '../components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '../components/ui/dialog'
import { Input, Label } from '../components/ui/input'
import { PageHeader } from '../components/ui/page-header'
import { Segmented, SegmentedButton } from '../components/ui/segmented'
import { CURRENCY_SYMBOLS, SUPPORTED_CURRENCIES } from '../constants/currencies'
import { useAccounts } from '../hooks/useAccounts'
import { useHaptic } from '../hooks/useHaptic'
import { useMode } from '../hooks/useMode'
import { useRefreshFinanceData } from '../hooks/useRefreshFinanceData'
import { cn } from '../lib/utils'
import { accountModeForTransactionSource, transactionSourceForMode } from '../utils/accountScope'
import { CATEGORY_KEYS, categoryLabel } from '../utils/categoryLabel'
import { formatAmount } from '../utils/format'
import { shouldRotateIdempotencyKey } from '../utils/idempotency'
import { hasOCRSuggestion } from '../utils/ocr'

function currentLocalDateTime() {
  const now = new Date()
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}T${String(now.getHours()).padStart(2, '0')}:${String(now.getMinutes()).padStart(2, '0')}`
}

function apiDateTime(value: string) {
  return value ? `${value.replace('T', ' ')}:00` : currentLocalDateTime().replace('T', ' ') + ':00'
}

function OcrReviewDialog({
  suggestion,
  onApply,
  onClose,
}: {
  suggestion: OCRSuggestion | null
  onApply: (suggestion: OCRSuggestion) => void
  onClose: () => void
}) {
  const { t } = useTranslation()
  const visibleSuggestion = hasOCRSuggestion(suggestion) ? suggestion : null
  const rows = visibleSuggestion
    ? [
        ['amount', visibleSuggestion.amount_yuan ? String(visibleSuggestion.amount_yuan) : ''],
        ['date', visibleSuggestion.occurred_at || ''],
        ['merchant', visibleSuggestion.merchant || ''],
        ['category', visibleSuggestion.category || ''],
        ['note', visibleSuggestion.note || ''],
      ].filter(([, value]) => value)
    : []

  return (
    <Dialog open={Boolean(visibleSuggestion)} onOpenChange={(open) => { if (!open) onClose() }}>
      {visibleSuggestion ? (
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('attachments.ocr.reviewTitle')}</DialogTitle>
            <DialogDescription>{t('attachments.ocr.reviewDesc')}</DialogDescription>
          </DialogHeader>
          <div className="grid gap-2 rounded-lg border border-border bg-muted/60 p-3">
            {rows.length === 0 ? (
              <p className="text-sm text-muted-foreground">{t('attachments.ocr.noSuggestion')}</p>
            ) : rows.map(([key, value]) => (
              <div key={key} className="flex justify-between gap-3 text-sm">
                <span className="text-muted-foreground">{t(`attachments.ocr.fields.${key}`)}</span>
                <span className="text-right font-medium text-foreground">{value}</span>
              </div>
            ))}
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t('common.cancel')}
            </Button>
            <Button type="button" onClick={() => onApply(visibleSuggestion)}>
              {t('attachments.ocr.apply')}
            </Button>
          </DialogFooter>
        </DialogContent>
      ) : null}
    </Dialog>
  )
}

export default function AddTransactionPage() {
  const { mode, isWorkMode } = useMode()
  const [searchParams] = useSearchParams()
  const initialSource = transactionSourceForMode(mode, searchParams.get('source'))

  return (
    <AddTransactionForm
      key={`${mode}:${initialSource}`}
      mode={mode}
      isWorkMode={isWorkMode}
      initialSource={initialSource}
    />
  )
}

function AddTransactionForm({
  mode,
  isWorkMode,
  initialSource,
}: {
  mode: 'work' | 'life'
  isWorkMode: boolean
  initialSource: 'personal' | 'company'
}) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const refreshFinanceData = useRefreshFinanceData()
  const haptic = useHaptic()
  const [error, setError] = useState('')
  const [success, setSuccess] = useState(false)
  const [loading, setLoading] = useState(false)

  const [customCat, setCustomCat] = useState('')
  const [occurredAt, setOccurredAt] = useState(currentLocalDateTime())
  const [pendingAttachments, setPendingAttachments] = useState<Attachment[]>([])
  const [ocrSuggestion, setOcrSuggestion] = useState<OCRSuggestion | null>(null)
  const [createdTransactionId, setCreatedTransactionId] = useState<string | null>(null)
  const pendingAttachmentsRef = useRef<Attachment[]>([])
  const submissionInFlightRef = useRef(false)
  const createIdempotencyKeyRef = useRef<string | null>(null)

  const [form, setForm] = useState({
    direction: 'expense',
    source: initialSource,
    account_id: '',
    category: CATEGORY_KEYS[0],
    amount_yuan: '',
    currency: 'CNY',
    note: '',
    project_id: '',
  })
  const accountLookupMode = accountModeForTransactionSource(form.source as 'personal' | 'company')
  const { data: accounts = [], isLoading: accountsLoading, isError: accountsError, refetch: refetchAccounts, isFetching: accountsFetching } = useAccounts(accountLookupMode)

  // Auto-select first account matching current source type
  useEffect(() => {
    if (accounts.length === 0) return
    const targetType = form.source === 'personal' ? 'personal' : 'public'
    const match = accounts.find(a => a.type === targetType && a.is_active)
    if (match && form.account_id !== match.id) {
      setForm(prev => ({ ...prev, account_id: match.id }))
    } else if (!match) {
      setForm(prev => ({ ...prev, account_id: '' }))
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [form.source, accounts, isWorkMode])

  function set(key: string, value: string) {
    setForm((prev) => ({ ...prev, [key]: value }))
  }

  const sourceAccounts = accounts.filter(a =>
    a.is_active && a.type === (form.source === 'personal' ? 'personal' : 'public')
  )

  function replacePendingAttachments(next: Attachment[]) {
    pendingAttachmentsRef.current = next
    setPendingAttachments(next)
  }

  function addPendingAttachment(attachment: Attachment) {
    setPendingAttachments((previous) => {
      const next = [...previous, attachment]
      pendingAttachmentsRef.current = next
      return next
    })
  }

  async function discardPendingAttachments() {
    const pending = pendingAttachmentsRef.current
    replacePendingAttachments([])
    await Promise.allSettled(pending.map((attachment) => deleteAttachment(attachment.id)))
  }

  useEffect(() => {
    return () => {
      const pending = pendingAttachmentsRef.current
      pendingAttachmentsRef.current = []
      pending.forEach((attachment) => {
        void deleteAttachment(attachment.id).catch(() => undefined)
      })
    }
  }, [])

  async function linkPendingAttachments(transactionId: string, attachments: Attachment[]) {
    const settled = await Promise.allSettled(
      attachments.map((attachment) => linkAttachment(attachment.id, transactionId)),
    )
    return attachments.filter((_, index) => settled[index].status === 'rejected')
  }

  function finishCreatedTransaction(amount: number) {
    replacePendingAttachments([])
    createIdempotencyKeyRef.current = null
    haptic.success()
    refreshFinanceData()
    toast.success(t('addTransaction.toast.success'), {
      description: `${CURRENCY_SYMBOLS[form.currency] ?? form.currency}${amount.toFixed(2)}`,
    })
    setSuccess(true)
    window.setTimeout(() => navigate(`/transactions?source=${form.source}`), 1200)
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError('')
    if (submissionInFlightRef.current || success) return
    if (!createdTransactionId && accountsUnavailable) {
      haptic.error()
      setError(accountsError ? t('addTransaction.accountLoad.error') : t('addTransaction.accountLoad.empty'))
      return
    }
    const amount = parseFloat(form.amount_yuan)
    if (!createdTransactionId && (isNaN(amount) || amount <= 0)) {
      haptic.error()
      setError(t('addTransaction.toast.invalidAmount'))
      return
    }
    submissionInFlightRef.current = true
    setLoading(true)
    try {
      let transactionId = createdTransactionId
      if (!transactionId) {
        const idempotencyKey = createIdempotencyKeyRef.current ?? crypto.randomUUID()
        createIdempotencyKeyRef.current = idempotencyKey
        const created = await createTransaction({
          ...form,
          occurred_at: apiDateTime(occurredAt),
          account_id: form.account_id || undefined,
          project_id: form.project_id.trim() || undefined,
          amount_yuan: amount,
          mode,
        }, idempotencyKey)
        transactionId = created.id
        setCreatedTransactionId(transactionId)
        // The financial write succeeded even if a later attachment link fails.
        refreshFinanceData()
      }

      const attachments = pendingAttachmentsRef.current
      const failed = await linkPendingAttachments(transactionId, attachments)
      if (failed.length > 0) {
        replacePendingAttachments(failed)
        haptic.error()
        toast.warning(t('addTransaction.toast.createdAttachmentWarning', { count: failed.length }))
        return
      }

      finishCreatedTransaction(amount)
    } catch (err: unknown) {
      if (shouldRotateIdempotencyKey(err)) {
        // A definitive server rejection did not create a transaction; a
        // corrected form submission is a new idempotent operation.
        createIdempotencyKeyRef.current = null
      }
      haptic.error()
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      setError(msg || t('addTransaction.toast.error'))
      toast.error(msg || t('addTransaction.toast.error'))
    } finally {
      submissionInFlightRef.current = false
      setLoading(false)
    }
  }

  async function handleCancel() {
    if (submissionInFlightRef.current) return
    submissionInFlightRef.current = true
    setLoading(true)
    await discardPendingAttachments()
    createIdempotencyKeyRef.current = null
    submissionInFlightRef.current = false
    setLoading(false)
    if (createdTransactionId) {
      navigate(`/transactions?source=${form.source}`)
    } else {
      navigate(-1)
    }
  }

  const labelClass = 'mb-1.5 block text-xs font-medium text-muted-foreground'

  const isExpense = form.direction === 'expense'
  const isPersonal = form.source === 'personal'
  const accountsUnavailable = accountsLoading || accountsError || sourceAccounts.length === 0 || !form.account_id

  function applyOcrSuggestion(suggestion: OCRSuggestion) {
    if (suggestion.amount_yuan && suggestion.amount_yuan > 0) set('amount_yuan', String(suggestion.amount_yuan))
    if (suggestion.currency) set('currency', suggestion.currency)
    if (suggestion.category) {
      set('category', suggestion.category)
      if (!CATEGORY_KEYS.includes(suggestion.category as typeof CATEGORY_KEYS[number])) setCustomCat(suggestion.category)
    }
    const noteParts = [suggestion.merchant, suggestion.invoice_number ? `${t('attachments.ocr.invoiceNo')}: ${suggestion.invoice_number}` : '', suggestion.note].filter(Boolean)
    if (noteParts.length > 0) set('note', noteParts.join(' · '))
    if (suggestion.occurred_at) {
      const normalized = suggestion.occurred_at.replace(' ', 'T').slice(0, 16)
      setOccurredAt(normalized)
    }
    setOcrSuggestion(null)
  }

  return (
    <div className="mx-auto max-w-4xl space-y-5 pb-8">
      <PageHeader
        title={t('addTransaction.title')}
        description={t('addTransaction.subtitle')}
      />

      {success ? (
        <Alert variant="positive">
          <CheckCircle2 className="mt-0.5 size-4 shrink-0" />
          <span>{t('addTransaction.toast.successRedirect')}</span>
        </Alert>
      ) : null}

      <form onSubmit={handleSubmit} className="grid grid-cols-1 gap-4 md:grid-cols-2 md:items-stretch">
        <fieldset
          disabled={Boolean(createdTransactionId)}
          className="grid min-w-0 grid-cols-1 gap-4 md:col-span-2 md:grid-cols-2 md:items-stretch"
        >
          <Card className="order-1 space-y-5">
            <div>
              <p id="transaction-direction-label" className={labelClass}>{t('addTransaction.form.direction')}</p>
              <Segmented role="group" aria-labelledby="transaction-direction-label" className="grid w-full grid-cols-2">
                <SegmentedButton
                  onClick={() => set('direction', 'expense')}
                  aria-pressed={isExpense}
                  className="h-9 aria-pressed:text-negative"
                >
                  <ArrowDown className="size-4" />
                  {t('addTransaction.form.expense')}
                </SegmentedButton>
                <SegmentedButton
                  onClick={() => set('direction', 'income')}
                  aria-pressed={!isExpense}
                  className="h-9 aria-pressed:text-positive"
                >
                  <ArrowUp className="size-4" />
                  {t('addTransaction.form.income')}
                </SegmentedButton>
              </Segmented>
            </div>

            <div>
              <p id="transaction-source-label" className={labelClass}>{t('addTransaction.form.source')}</p>
              <Segmented role="group" aria-labelledby="transaction-source-label" className="grid w-full grid-cols-2">
                <SegmentedButton
                  onClick={() => set('source', 'personal')}
                  aria-pressed={isPersonal}
                  className="h-9 aria-pressed:text-mode"
                >
                  <UserRound className="size-4" />
                  {t('addTransaction.form.personalAdvance')}
                </SegmentedButton>
                <SegmentedButton
                  onClick={() => isWorkMode && set('source', 'company')}
                  disabled={!isWorkMode}
                  aria-pressed={!isPersonal}
                  className="h-9 aria-pressed:text-mode"
                >
                  <Building2 className="size-4" />
                  {t('addTransaction.form.publicAccount')}
                </SegmentedButton>
              </Segmented>
            </div>

            <div>
              <Label className="mb-1.5 block">{t('addTransaction.form.account')}</Label>
              {accountsLoading ? (
                <div className="rounded-lg border border-input bg-muted px-3 py-2.5 text-sm text-muted-foreground">
                  {t('addTransaction.accountLoad.loading')}
                </div>
              ) : accountsError ? (
                <Alert variant="negative" className="flex-col items-start">
                  <p>{t('addTransaction.accountLoad.error')}</p>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => refetchAccounts()}
                    disabled={accountsFetching}
                    className="h-7 px-0 text-negative hover:bg-transparent hover:text-negative"
                  >
                    {accountsFetching ? t('common.loading') : t('common.retry')}
                  </Button>
                </Alert>
              ) : sourceAccounts.length > 0 ? (
                <Select
                  value={form.account_id}
                  onChange={(value) => set('account_id', value)}
                  size="lg"
                  options={sourceAccounts.map((account) => ({
                    value: account.id,
                    label: `${account.name}（${t('common.balance')} ${formatAmount(account.balance_yuan, account.currency)}）`,
                  }))}
                />
              ) : (
                <Alert variant="warning" className="flex-col items-start">
                  <p>{t('addTransaction.accountLoad.empty')}</p>
                  <Link
                    to="/settings"
                    className={buttonVariants({ variant: 'ghost', size: 'sm', className: 'h-7 px-0 text-warning hover:bg-transparent hover:text-warning' })}
                  >
                    <Settings2 className="size-3.5" />
                    {t('addTransaction.accountLoad.settingsLink')}
                  </Link>
                </Alert>
              )}
            </div>
          </Card>

          <Card className="order-2 flex flex-col justify-between">
            <Label htmlFor="transaction-amount" className="mb-1.5 block">{t('addTransaction.form.amount')}</Label>
            <div className={cn(
              'flex items-center gap-2 rounded-lg border bg-card px-3 transition-[border-color,box-shadow] focus-within:ring-2',
              isExpense
                ? 'border-negative/40 focus-within:border-negative focus-within:ring-negative/15'
                : 'border-positive/40 focus-within:border-positive focus-within:ring-positive/15',
            )}>
              <span className={cn('shrink-0 whitespace-nowrap text-xl font-semibold', isExpense ? 'text-negative' : 'text-positive')}>
                {isExpense ? '−' : '+'}{CURRENCY_SYMBOLS[form.currency] ?? form.currency}
              </span>
              <input
                id="transaction-amount"
                type="number"
                required
                min="0.01"
                step="0.01"
                className="w-full min-w-20 flex-1 bg-transparent py-3 text-2xl font-semibold tabular-nums text-foreground outline-none placeholder:text-subtle"
                placeholder="0.00"
                value={form.amount_yuan}
                onChange={(event) => set('amount_yuan', event.target.value)}
              />
              <Select
                value={form.currency}
                onChange={(value) => set('currency', value)}
                size="sm"
                className="min-w-[4.5rem]"
                options={SUPPORTED_CURRENCIES.map((currency) => ({
                  value: currency.code,
                  label: currency.code,
                }))}
              />
            </div>
            <p className="mt-2 text-xs text-muted-foreground">
              {isExpense ? t('addTransaction.form.expenseHint') : t('addTransaction.form.incomeHint')}
            </p>
          </Card>

          <Card className="order-3 md:col-span-2">
            <Label className="mb-2 block">{t('addTransaction.form.category')}</Label>
            <div className="grid grid-cols-2 gap-2 min-[380px]:grid-cols-3 sm:grid-cols-5 md:grid-cols-7">
              {CATEGORY_KEYS.map((category) => (
                <button
                  key={category}
                  type="button"
                  onClick={() => { set('category', category); setCustomCat('') }}
                  aria-pressed={form.category === category}
                  className="min-h-10 rounded-lg border border-border bg-card px-2 py-2 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring aria-pressed:border-accent/40 aria-pressed:bg-accent-soft aria-pressed:text-accent"
                >
                  {categoryLabel(category)}
                </button>
              ))}
            </div>
            <div className="mt-3 flex items-center gap-2">
              <Label htmlFor="custom-category" className="shrink-0">{t('addTransaction.form.custom')}</Label>
              <Input
                id="custom-category"
                type="text"
                value={customCat}
                onChange={(event) => {
                  const value = event.target.value
                  setCustomCat(value)
                  set('category', value.trim() !== '' ? value.trim() : CATEGORY_KEYS[0])
                }}
                placeholder={t('addTransaction.form.customPlaceholder')}
                className={cn(
                  'h-8 text-xs',
                  !CATEGORY_KEYS.includes(form.category as typeof CATEGORY_KEYS[number]) && customCat.trim() !== '' &&
                    'border-accent bg-accent-soft text-accent',
                )}
              />
            </div>
          </Card>

          <Card className="order-4 md:col-span-2">
            <Label htmlFor="transaction-occurred-at" className="mb-1.5 block">{t('addTransaction.form.occurredAt')}</Label>
            <Input
              id="transaction-occurred-at"
              type="datetime-local"
              value={occurredAt}
              onChange={(event) => setOccurredAt(event.target.value)}
            />
          </Card>

          <Card className="order-5 space-y-3 md:col-span-2">
            <div className="flex items-start gap-2">
              <span className="grid size-8 shrink-0 place-items-center rounded-md bg-muted text-muted-foreground">
                <Paperclip className="size-4" />
              </span>
              <div>
                <Label className="block text-foreground">{t('attachments.title')}</Label>
                <p className="mt-0.5 text-xs text-muted-foreground">{t('attachments.addHint')}</p>
              </div>
            </div>
            <AttachmentUploader
              onUploaded={addPendingAttachment}
              onSuggestion={setOcrSuggestion}
            />
            {pendingAttachments.length > 0 ? (
              <div className="grid gap-1 rounded-lg border border-positive/25 bg-positive-soft p-3">
                {pendingAttachments.map((attachment) => (
                  <p key={attachment.id} className="text-xs text-positive">
                    {t('attachments.pendingLink', { name: attachment.original_filename })}
                  </p>
                ))}
              </div>
            ) : null}
          </Card>

          <Card className="order-6 grid gap-4 md:col-span-2 md:grid-cols-2">
            <div>
              <Label htmlFor="transaction-project" className="mb-1.5 block">
                {t('addTransaction.form.project')}{' '}
                <span className="font-normal text-muted-foreground">{t('addTransaction.form.optional')}</span>
              </Label>
              <Input
                id="transaction-project"
                type="text"
                placeholder={t('addTransaction.form.projectPlaceholder')}
                value={form.project_id}
                onChange={(event) => set('project_id', event.target.value)}
              />
            </div>
            <div>
              <Label htmlFor="transaction-note" className="mb-1.5 block">
                {t('addTransaction.form.note')}{' '}
                <span className="font-normal text-muted-foreground">{t('addTransaction.form.optional')}</span>
              </Label>
              <Input
                id="transaction-note"
                type="text"
                placeholder={t('addTransaction.form.notePlaceholder')}
                value={form.note}
                onChange={(event) => set('note', event.target.value)}
              />
            </div>
          </Card>
        </fieldset>

        <div className="order-7 space-y-3 md:col-span-2">
          {createdTransactionId && !success && pendingAttachments.length > 0 ? (
            <Alert variant="warning" className="items-start">
              <AlertCircle className="mt-0.5 size-4 shrink-0" />
              <div>
                <p className="font-semibold">{t('addTransaction.attachmentRecovery.title')}</p>
                <p className="mt-1 text-xs leading-5">{t('addTransaction.attachmentRecovery.description', { count: pendingAttachments.length })}</p>
              </div>
            </Alert>
          ) : null}
          {error ? (
            <Alert variant="negative" className="items-start">
              <AlertCircle className="mt-0.5 size-4 shrink-0" />
              <span>{error}</span>
            </Alert>
          ) : null}
          <Card className="flex flex-col-reverse gap-2 p-3 sm:flex-row sm:justify-end">
            <Button
              type="button"
              variant="outline"
              onClick={() => { void handleCancel() }}
              disabled={loading || success}
              className="sm:min-w-28"
            >
              {createdTransactionId ? t('addTransaction.form.continueWithoutAttachments') : t('common.cancel')}
            </Button>
            <Button
              type="submit"
              loading={loading}
              loadingText={t('addTransaction.form.submitting')}
              disabled={success || (!createdTransactionId && accountsUnavailable)}
              className="sm:min-w-44"
            >
              {createdTransactionId
                ? t('addTransaction.form.retryAttachments')
                : t('addTransaction.form.submit')}
            </Button>
          </Card>
        </div>
      </form>

      <OcrReviewDialog
        suggestion={ocrSuggestion}
        onApply={applyOcrSuggestion}
        onClose={() => setOcrSuggestion(null)}
      />
    </div>
  )
}
