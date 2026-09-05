import { useState, useEffect, useRef } from 'react'
import type { FormEvent } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'
import { createTransaction, deleteAttachment, linkAttachment } from '../api/client'
import type { Attachment, OCRSuggestion } from '../api/client'
import { useAccounts } from '../hooks/useAccounts'
import { useHaptic } from '../hooks/useHaptic'
import Select from '../components/Select'
import { CATEGORY_KEYS, categoryLabel } from '../utils/categoryLabel'
import { useMode } from '../hooks/useMode'
import { useRefreshFinanceData } from '../hooks/useRefreshFinanceData'
import { CURRENCY_SYMBOLS, SUPPORTED_CURRENCIES } from '../constants/currencies'
import AttachmentUploader from '../components/AttachmentUploader'
import OcrReviewModal from '../components/OcrReviewModal'
import { formatAmount } from '../utils/format'
import { shouldRotateIdempotencyKey } from '../utils/idempotency'
import { accountModeForTransactionSource, transactionSourceForMode } from '../utils/accountScope'

function currentLocalDateTime() {
  const now = new Date()
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}T${String(now.getHours()).padStart(2, '0')}:${String(now.getMinutes()).padStart(2, '0')}`
}

function apiDateTime(value: string) {
  return value ? `${value.replace('T', ' ')}:00` : currentLocalDateTime().replace('T', ' ') + ':00'
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

  const inputClass = 'fin-input w-full px-3.5 py-2.5 text-sm'
  const labelClass = 'page-kicker mb-2 block'

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
    <div className="max-w-4xl pb-8">
      <div className="ledger-rail mb-6 pl-5">
        <h1 className="font-display text-2xl font-semibold tracking-[-0.025em] text-[hsl(var(--foreground))] md:text-[1.75rem]">{t('addTransaction.title')}</h1>
        <p className="mt-1 text-sm text-[hsl(var(--muted-foreground))]">{t('addTransaction.subtitle')}</p>
      </div>

      {success && (
        <div className="mb-4 bg-emerald-50 dark:bg-emerald-500/10 border border-green-200 dark:border-emerald-500/30 text-emerald-700 dark:text-emerald-400 rounded-xl px-4 py-3 text-sm flex items-center gap-2">
          <svg className="w-4 h-4 shrink-0" fill="currentColor" viewBox="0 0 20 20"><path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" /></svg>
          {t('addTransaction.toast.successRedirect')}
        </div>
      )}

      <form onSubmit={handleSubmit} className="ledger-panel grid grid-cols-1 overflow-hidden md:grid-cols-2 md:items-stretch">
        <fieldset disabled={Boolean(createdTransactionId)} className="contents">

        {/* Direction + Source */}
        <div className="order-1 space-y-5 border-b border-[hsl(var(--border))] p-4 md:border-r md:p-6">
          <div>
            <p id="transaction-direction-label" className={labelClass}>{t('addTransaction.form.direction')}</p>
            <div role="group" aria-labelledby="transaction-direction-label" className="grid grid-cols-2 gap-2">
              <button type="button"
                onClick={() => set('direction', 'expense')}
                aria-pressed={isExpense}
                className={`flex min-h-11 items-center justify-center gap-2 rounded-[7px] border py-2.5 text-sm font-semibold transition-colors ${
                  isExpense
                    ? 'border-[hsl(var(--expense))] bg-[hsl(var(--expense))]/10 text-[hsl(var(--expense))]'
                    : 'border-[hsl(var(--border))] bg-[hsl(var(--card))] text-[hsl(var(--muted-foreground))] hover:border-[hsl(var(--expense))]/50'
                }`}
              >
                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2.5}><path strokeLinecap="round" strokeLinejoin="round" d="M17 13l-5 5m0 0l-5-5m5 5V6" /></svg>
                {t('addTransaction.form.expense')}
              </button>
              <button type="button"
                onClick={() => set('direction', 'income')}
                aria-pressed={!isExpense}
                className={`flex min-h-11 items-center justify-center gap-2 rounded-[7px] border py-2.5 text-sm font-semibold transition-colors ${
                  !isExpense
                    ? 'border-[hsl(var(--income))] bg-[hsl(var(--income))]/10 text-[hsl(var(--income))]'
                    : 'border-[hsl(var(--border))] bg-[hsl(var(--card))] text-[hsl(var(--muted-foreground))] hover:border-[hsl(var(--income))]/50'
                }`}
              >
                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2.5}><path strokeLinecap="round" strokeLinejoin="round" d="M7 11l5-5m0 0l5 5m-5-5v12" /></svg>
                {t('addTransaction.form.income')}
              </button>
            </div>
          </div>

          <div>
            <p id="transaction-source-label" className={labelClass}>{t('addTransaction.form.source')}</p>
            <div role="group" aria-labelledby="transaction-source-label" className="grid grid-cols-2 gap-2">
              <button type="button"
                onClick={() => set('source', 'personal')}
                aria-pressed={isPersonal}
                className={`flex min-h-11 items-center justify-center gap-2 rounded-[7px] border py-2.5 text-sm font-semibold transition-colors ${
                  isPersonal
                    ? 'border-amber-500/55 bg-amber-500/10 text-amber-700 dark:text-amber-300'
                    : 'border-[hsl(var(--border))] bg-[hsl(var(--card))] text-[hsl(var(--muted-foreground))] hover:border-amber-500/40'
                }`}
              >
                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" /></svg>
                {t('addTransaction.form.personalAdvance')}
              </button>
              <button type="button"
                onClick={() => isWorkMode && set('source', 'company')}
                disabled={!isWorkMode}
                aria-pressed={!isPersonal}
                className={`flex min-h-11 items-center justify-center gap-2 rounded-[7px] border py-2.5 text-sm font-semibold transition-colors ${
                  !isPersonal
                    ? 'border-[#2d6687]/55 bg-[#2d6687]/10 text-[#2d6687] dark:text-[#72a9c8]'
                    : 'border-[hsl(var(--border))] bg-[hsl(var(--card))] text-[hsl(var(--muted-foreground))] hover:border-[#2d6687]/40'
                } ${!isWorkMode ? 'cursor-not-allowed opacity-50' : ''}`}
              >
                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4" /></svg>
                {t('addTransaction.form.publicAccount')}
              </button>
            </div>
          </div>

          {/* Account picker */}
          <div>
            <label className={labelClass}>{t('addTransaction.form.account')}</label>
            {accountsLoading ? (
              <div className="rounded-xl border border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-800/50 px-3.5 py-2.5 text-sm text-gray-400 dark:text-gray-500">
                {t('addTransaction.accountLoad.loading')}
              </div>
            ) : accountsError ? (
              <div className="rounded-xl border border-rose-200 dark:border-rose-500/30 bg-rose-50 dark:bg-rose-500/10 px-3.5 py-3 text-sm text-rose-700 dark:text-rose-400 space-y-2">
                <p>{t('addTransaction.accountLoad.error')}</p>
                <button
                  type="button"
                  onClick={() => refetchAccounts()}
                  disabled={accountsFetching}
                  className="text-xs font-semibold underline underline-offset-2 disabled:opacity-50"
                >
                  {accountsFetching ? t('common.loading') : t('common.retry')}
                </button>
              </div>
            ) : sourceAccounts.length > 0 ? (
              <Select
                value={form.account_id}
                onChange={(v) => set('account_id', v)}
                size="lg"
                options={sourceAccounts.map(a => ({
                  value: a.id,
                  label: `${a.name}（${t('common.balance')} ${formatAmount(a.balance_yuan, a.currency)}）`,
                }))}
              />
            ) : (
              <div className="rounded-xl border border-amber-200 dark:border-amber-500/30 bg-amber-50 dark:bg-amber-500/10 px-3.5 py-3 text-sm text-amber-700 dark:text-amber-400 space-y-2">
                <p>{t('addTransaction.accountLoad.empty')}</p>
                <Link to="/settings" className="inline-flex text-xs font-semibold underline underline-offset-2">
                  {t('addTransaction.accountLoad.settingsLink')}
                </Link>
              </div>
            )}
          </div>
        </div>

        {/* Amount */}
        <div className="order-2 flex flex-col justify-between border-b border-[hsl(var(--border))] bg-[hsl(var(--mode-accent-wash))]/35 p-4 md:p-6">
          <label className={labelClass}>{t('addTransaction.form.amount')}</label>
          <div className={`flex items-center gap-2 rounded-[8px] border bg-[hsl(var(--card))] px-3 py-2 transition-colors ${isExpense ? 'border-[hsl(var(--expense))]/45 focus-within:border-[hsl(var(--expense))]' : 'border-[hsl(var(--income))]/45 focus-within:border-[hsl(var(--income))]'}`}>
            <span className={`select-none whitespace-nowrap font-data text-xl font-semibold ${isExpense ? 'text-[hsl(var(--expense))]' : 'text-[hsl(var(--income))]'}`}>
              {isExpense ? '−' : '+'}{CURRENCY_SYMBOLS[form.currency] ?? form.currency}
            </span>
            <input
              type="number"
              required
              min="0.01"
              step="0.01"
              className="min-w-0 flex-1 bg-transparent py-2 font-data text-2xl font-semibold text-[hsl(var(--foreground))] outline-none placeholder:text-[hsl(var(--border))]"
              placeholder="0.00"
              value={form.amount_yuan}
              onChange={(e) => set('amount_yuan', e.target.value)}
            />
            <div className="shrink-0">
              <Select
                value={form.currency}
                onChange={(v) => set('currency', v)}
                size="sm"
                className="!rounded-lg min-w-[72px] !bg-gray-100 !text-gray-700 !border-gray-200 hover:!bg-white hover:!border-gray-300 dark:!bg-gray-800 dark:!text-gray-200 dark:!border-gray-600 dark:hover:!bg-gray-700 dark:hover:!border-gray-500"
                options={SUPPORTED_CURRENCIES.map((currency) => ({
                  value: currency.code,
                  label: currency.code,
                }))}
              />
            </div>
          </div>
          <p className="mt-2 text-xs text-[hsl(var(--muted-foreground))]">
            {isExpense ? t('addTransaction.form.expenseHint') : t('addTransaction.form.incomeHint')}
          </p>
        </div>

        {/* Category */}
        <div className="order-3 border-b border-[hsl(var(--border))] p-4 md:col-span-2 md:p-6">
          <label className={labelClass}>{t('addTransaction.form.category')}</label>
          <div className="grid grid-cols-2 min-[380px]:grid-cols-3 sm:grid-cols-5 md:grid-cols-7 gap-2.5">
            {CATEGORY_KEYS.map((c) => (
              <button
                key={c}
                type="button"
                onClick={() => { set('category', c); setCustomCat('') }}
                aria-pressed={form.category === c}
                className={`flex min-h-11 items-center justify-center rounded-[7px] border px-2 py-2.5 text-xs font-semibold transition-colors ${
                  form.category === c
                    ? 'border-[hsl(var(--mode-accent))] bg-[hsl(var(--mode-accent-wash))] text-[hsl(var(--mode-accent-strong))]'
                    : 'border-[hsl(var(--border))] bg-[hsl(var(--card))] text-[hsl(var(--muted-foreground))] hover:border-[hsl(var(--mode-accent))]/45 hover:text-[hsl(var(--foreground))]'
                }`}
              >
                <span className="leading-tight text-center">{categoryLabel(c)}</span>
              </button>
            ))}
          </div>
          {/* Custom category */}
          <div className="mt-3 flex items-center gap-2">
            <span className="text-xs text-gray-400 dark:text-gray-500 shrink-0">{t('addTransaction.form.custom')}</span>
            <input
              type="text"
              value={customCat}
              onChange={e => {
                const v = e.target.value
                setCustomCat(v)
                set('category', v.trim() !== '' ? v.trim() : CATEGORY_KEYS[0])
              }}
              placeholder={t('addTransaction.form.customPlaceholder')}
              className={`min-w-0 flex-1 rounded-[7px] border px-3 py-2 text-xs transition-colors placeholder:text-[hsl(var(--muted-foreground))]/70 ${
                !CATEGORY_KEYS.includes(form.category as typeof CATEGORY_KEYS[number]) && customCat.trim() !== ''
                  ? 'border-[hsl(var(--mode-accent))] bg-[hsl(var(--mode-accent-wash))] font-semibold text-[hsl(var(--mode-accent-strong))]'
                  : 'border-[hsl(var(--border))] bg-[hsl(var(--control))] text-[hsl(var(--foreground))] hover:border-[hsl(var(--muted-foreground))]/55 focus:border-[hsl(var(--ring))] focus:bg-[hsl(var(--control-hover))]'
              }`}
            />
          </div>
        </div>

        {/* Date/time */}
        <div className="order-4 border-b border-[hsl(var(--border))] p-4 md:col-span-2 md:p-6">
          <label className={labelClass}>{t('addTransaction.form.occurredAt')}</label>
          <input
            type="datetime-local"
            className={inputClass}
            value={occurredAt}
            onChange={(e) => setOccurredAt(e.target.value)}
          />
        </div>

        {/* Attachment + OCR */}
        <div className="order-5 space-y-3 border-b border-[hsl(var(--border))] p-4 md:col-span-2 md:p-6">
          <div>
            <label className={labelClass}>{t('attachments.title')}</label>
            <p className="mb-3 text-xs text-gray-400 dark:text-gray-500">{t('attachments.addHint')}</p>
            <AttachmentUploader
              onUploaded={addPendingAttachment}
              onSuggestion={setOcrSuggestion}
            />
            {pendingAttachments.length > 0 && (
              <div className="mt-2 space-y-1">
                {pendingAttachments.map((attachment) => (
                  <p key={attachment.id} className="text-xs text-emerald-600 dark:text-emerald-300">
                    {t('attachments.pendingLink', { name: attachment.original_filename })}
                  </p>
                ))}
              </div>
            )}
          </div>
        </div>

        {/* Project + Note */}
        <div className="order-6 space-y-4 p-4 md:col-span-2 md:p-6">
          <div>
            <label className={labelClass}>
              {t('addTransaction.form.project')} <span className="text-gray-300 dark:text-gray-600 font-normal normal-case tracking-normal">{t('addTransaction.form.optional')}</span>
            </label>
            <input
              type="text"
              className={inputClass}
              placeholder={t('addTransaction.form.projectPlaceholder')}
              value={form.project_id}
              onChange={(e) => set('project_id', e.target.value)}
            />
          </div>
          <div>
            <label className={labelClass}>
              {t('addTransaction.form.note')} <span className="text-gray-300 dark:text-gray-600 font-normal normal-case tracking-normal">{t('addTransaction.form.optional')}</span>
            </label>
            <input
              type="text"
              className={inputClass}
              placeholder={t('addTransaction.form.notePlaceholder')}
              value={form.note}
              onChange={(e) => set('note', e.target.value)}
            />
          </div>
        </div>
        </fieldset>

        {/* Error + Actions */}
        <div className="order-7 space-y-3 border-t border-[hsl(var(--border))] bg-[hsl(var(--muted))]/45 p-3 md:col-span-2">
          {createdTransactionId && !success && pendingAttachments.length > 0 && (
            <div className="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300">
              <p className="font-semibold">{t('addTransaction.attachmentRecovery.title')}</p>
              <p className="mt-1 text-xs leading-5">{t('addTransaction.attachmentRecovery.description', { count: pendingAttachments.length })}</p>
            </div>
          )}
          {error && (
            <div className="bg-rose-50 dark:bg-rose-500/10 border border-rose-200 dark:border-rose-500/30 text-rose-700 dark:text-rose-400 rounded-xl px-4 py-3 text-sm flex items-start gap-2">
              <svg className="w-4 h-4 shrink-0 mt-0.5" fill="currentColor" viewBox="0 0 20 20"><path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" /></svg>
              {error}
            </div>
          )}
          <div className="flex flex-col-reverse gap-3 sm:flex-row">
            <button
              type="submit"
              disabled={loading || success || (!createdTransactionId && accountsUnavailable)}
              className={`flex-1 rounded-[7px] py-3.5 text-sm font-semibold text-[hsl(var(--primary-foreground))] transition-[filter] disabled:opacity-50 ${
                isExpense
                  ? 'bg-[hsl(var(--expense))] hover:brightness-90'
                  : 'bg-[hsl(var(--income))] hover:brightness-90'
              }`}
            >
              {loading
                ? t('addTransaction.form.submitting')
                : createdTransactionId
                  ? t('addTransaction.form.retryAttachments')
                  : t('addTransaction.form.submit')}
            </button>
            <button
              type="button"
              onClick={() => { void handleCancel() }}
              disabled={loading || success}
              className="fin-control px-5 py-3.5 text-sm font-medium text-[hsl(var(--muted-foreground))] transition-colors hover:text-[hsl(var(--foreground))]"
            >
              {createdTransactionId ? t('addTransaction.form.continueWithoutAttachments') : t('common.cancel')}
            </button>
          </div>
        </div>

      </form>
      <OcrReviewModal suggestion={ocrSuggestion} onApply={applyOcrSuggestion} onClose={() => setOcrSuggestion(null)} />
    </div>
  )
}
