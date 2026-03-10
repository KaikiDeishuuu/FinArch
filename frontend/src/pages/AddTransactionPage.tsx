import { useState, useEffect } from 'react'
import type { FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'
import { createTransaction } from '../api/client'
import { useAccounts } from '../hooks/useAccounts'
import { useHaptic } from '../hooks/useHaptic'
import Select from '../components/Select'
import { CATEGORY_KEYS, categoryLabel } from '../utils/categoryLabel'
import { useMode } from '../contexts/ModeContext'
import { useRefreshFinanceData } from '../hooks/useRefreshFinanceData'


export default function AddTransactionPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const refreshFinanceData = useRefreshFinanceData()
  const haptic = useHaptic()
  const { data: accounts = [] } = useAccounts()
  const [error, setError] = useState('')
  const [success, setSuccess] = useState(false)
  const [loading, setLoading] = useState(false)

  const [customCat, setCustomCat] = useState('')

  const { mode, isWorkMode } = useMode()
  const [form, setForm] = useState({
    direction: 'expense',
    source: isWorkMode ? 'company' : 'personal',
    account_id: '',
    category: CATEGORY_KEYS[0],
    amount_yuan: '',
    currency: 'CNY',
    note: '',
    project_id: '',
  })

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

  useEffect(() => {
    setForm((prev) => ({ ...prev, source: isWorkMode ? "company" : "personal" }))
  }, [isWorkMode])

  function set(key: string, value: string) {
    setForm((prev) => ({ ...prev, [key]: value }))
  }

  const sourceAccounts = accounts.filter(a =>
    a.is_active && a.type === (form.source === 'personal' ? 'personal' : 'public')
  )

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError('')
    const amount = parseFloat(form.amount_yuan)
    if (isNaN(amount) || amount <= 0) {
      haptic.error()
      setError(t('addTransaction.toast.invalidAmount'))
      return
    }
    setLoading(true)
    try {
      const now = new Date()
      const occurredAt = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')} ${String(now.getHours()).padStart(2, '0')}:${String(now.getMinutes()).padStart(2, '0')}:${String(now.getSeconds()).padStart(2, '0')}`
      await createTransaction({
        ...form,
        occurred_at: occurredAt,
        account_id: form.account_id || undefined,
        project_id: form.project_id.trim() || undefined,
        amount_yuan: amount,
        mode,
      })
      haptic.success()
      refreshFinanceData()
      toast.success(t('addTransaction.toast.success'), { description: `${form.currency === 'USD' ? '$' : form.currency === 'EUR' ? '€' : '¥'}${amount.toFixed(2)}` })
      setSuccess(true)
      setTimeout(() => navigate('/transactions'), 1200)
    } catch (err: unknown) {
      haptic.error()
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      setError(msg || t('addTransaction.toast.error'))
      toast.error(msg || t('addTransaction.toast.error'))
    } finally {
      setLoading(false)
    }
  }

  const inputClass = 'w-full border border-gray-200 dark:border-gray-700 rounded-xl px-3.5 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500 focus:border-transparent bg-gray-50 dark:bg-gray-800/50 dark:text-gray-200 dark:placeholder-gray-500 transition-all hover:bg-white dark:hover:bg-gray-800'
  const labelClass = 'block text-xs font-semibold text-gray-500 dark:text-gray-400 mb-2 uppercase tracking-wider'

  const isExpense = form.direction === 'expense'
  const isPersonal = form.source === 'personal'

  return (
    <div className="max-w-3xl pb-8">
      <div className="mb-6">
        <h1 className="text-2xl font-bold text-gray-900 dark:text-gray-100 tracking-tight">{t('addTransaction.title')}</h1>
        <p className="text-sm text-gray-400 dark:text-gray-500 mt-1">{t('addTransaction.subtitle')}</p>
      </div>

      {success && (
        <div className="mb-4 bg-emerald-50 dark:bg-emerald-500/10 border border-green-200 dark:border-emerald-500/30 text-emerald-700 dark:text-emerald-400 rounded-xl px-4 py-3 text-sm flex items-center gap-2">
          <svg className="w-4 h-4 shrink-0" fill="currentColor" viewBox="0 0 20 20"><path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" /></svg>
          {t('addTransaction.toast.successRedirect')}
        </div>
      )}

      <form onSubmit={handleSubmit} className="grid grid-cols-1 md:grid-cols-2 gap-4">

        {/* Direction + Source */}
        <div className="bg-white dark:bg-[hsl(260,15%,11%)] rounded-2xl border border-gray-100/80 dark:border-gray-800/50 p-5 shadow-sm space-y-4">
          <div>
            <label className={labelClass}>{t('addTransaction.form.direction')}</label>
            <div className="grid grid-cols-2 gap-2">
              <button type="button"
                onClick={() => set('direction', 'expense')}
                className={`flex items-center justify-center gap-2 py-2.5 rounded-xl text-sm font-semibold border-2 transition-all ${
                  isExpense
                    ? 'bg-rose-50 dark:bg-rose-500/10 border-rose-200 dark:border-rose-500/30 text-rose-600 dark:text-rose-400'
                    : 'bg-white dark:bg-transparent border-gray-200 dark:border-gray-700 text-gray-400 dark:text-gray-500 hover:border-rose-200 dark:hover:border-rose-500/30 hover:text-rose-400'
                }`}
              >
                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2.5}><path strokeLinecap="round" strokeLinejoin="round" d="M17 13l-5 5m0 0l-5-5m5 5V6" /></svg>
                {t('addTransaction.form.expense')}
              </button>
              <button type="button"
                onClick={() => set('direction', 'income')}
                className={`flex items-center justify-center gap-2 py-2.5 rounded-xl text-sm font-semibold border-2 transition-all ${
                  !isExpense
                    ? 'bg-emerald-50 dark:bg-emerald-500/10 border-emerald-200 dark:border-emerald-500/30 text-emerald-600 dark:text-emerald-400'
                    : 'bg-white dark:bg-transparent border-gray-200 dark:border-gray-700 text-gray-400 dark:text-gray-500 hover:border-emerald-200 dark:hover:border-emerald-500/30 hover:text-emerald-400'
                }`}
              >
                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2.5}><path strokeLinecap="round" strokeLinejoin="round" d="M7 11l5-5m0 0l5 5m-5-5v12" /></svg>
                {t('addTransaction.form.income')}
              </button>
            </div>
          </div>

          <div>
            <label className={labelClass}>{t('addTransaction.form.source')}</label>
            <div className="grid grid-cols-2 gap-2">
              <button type="button"
                onClick={() => isWorkMode ? null : set('source', 'personal')}
                className={`flex items-center justify-center gap-2 py-2.5 rounded-xl text-sm font-semibold border-2 transition-all ${
                  isPersonal
                    ? 'bg-amber-50 dark:bg-amber-500/10 border-amber-200 dark:border-amber-500/30 text-amber-600 dark:text-amber-400'
                    : 'bg-white dark:bg-transparent border-gray-200 dark:border-gray-700 text-gray-400 dark:text-gray-500 hover:border-amber-200 dark:hover:border-amber-500/30 hover:text-amber-400'
                }`}
              >
                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" /></svg>
                {t('addTransaction.form.personalAdvance')}
              </button>
              <button type="button"
                onClick={() => isWorkMode && set('source', 'company')}
                className={`flex items-center justify-center gap-2 py-2.5 rounded-xl text-sm font-semibold border-2 transition-all ${
                  !isPersonal
                    ? 'bg-sky-50 dark:bg-sky-500/10 border-sky-200 dark:border-sky-500/30 text-sky-600 dark:text-sky-400'
                    : 'bg-white dark:bg-transparent border-gray-200 dark:border-gray-700 text-gray-400 dark:text-gray-500 hover:border-sky-200 dark:hover:border-sky-500/30 hover:text-sky-400'
                }`}
              >
                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4" /></svg>
                {t('addTransaction.form.publicAccount')}
              </button>
            </div>
          </div>

          {/* Account picker */}
          {sourceAccounts.length > 0 && (
            <div>
              <label className={labelClass}>{t('addTransaction.form.account')}</label>
              <Select
                value={form.account_id}
                onChange={(v) => set('account_id', v)}
                size="lg"
                options={sourceAccounts.map(a => ({
                  value: a.id,
                  label: `${a.name}（${t('common.balance')} ¥${a.balance_yuan.toFixed(2)}）`,
                }))}
              />
            </div>
          )}
        </div>

        {/* Amount */}
        <div className="bg-white dark:bg-[hsl(260,15%,11%)] rounded-2xl border border-gray-100/80 dark:border-gray-800/50 p-5 shadow-sm flex flex-col justify-between">
          <label className={labelClass}>{t('addTransaction.form.amount')}</label>
          <div className={`flex items-center gap-2 rounded-xl border-2 px-3 py-1 transition-all ${isExpense ? 'border-rose-200 dark:border-rose-500/30 focus-within:border-red-400 dark:focus-within:border-rose-400' : 'border-green-200 dark:border-emerald-500/30 focus-within:border-green-400 dark:focus-within:border-emerald-400'}`}>
            <span className={`text-xl font-bold select-none whitespace-nowrap shrink-0 ${isExpense ? 'text-rose-400' : 'text-emerald-400'}`}>
              {isExpense ? '−' : '+'}{form.currency === 'USD' ? '$' : form.currency === 'EUR' ? '€' : '¥'}
            </span>
            <input
              type="number"
              required
              min="0.01"
              step="0.01"
              className="flex-1 min-w-0 text-xl font-bold text-gray-800 dark:text-gray-200 bg-transparent py-2 focus:outline-none placeholder:text-gray-200 dark:placeholder:text-gray-600"
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
                options={[
                  { value: 'CNY', label: 'CNY' },
                  { value: 'USD', label: 'USD' },
                  { value: 'EUR', label: 'EUR' },
                ]}
              />
            </div>
          </div>
          <p className="text-xs text-gray-400 dark:text-gray-500 mt-2">
            {isExpense ? t('addTransaction.form.expenseHint') : t('addTransaction.form.incomeHint')}
          </p>
        </div>

        {/* Category */}
        <div className="md:col-span-2 bg-white dark:bg-[hsl(260,15%,11%)] rounded-2xl border border-gray-100/80 dark:border-gray-800/50 p-5 shadow-sm">
          <label className={labelClass}>{t('addTransaction.form.category')}</label>
          <div className="grid grid-cols-3 sm:grid-cols-5 md:grid-cols-7 gap-2">
            {CATEGORY_KEYS.map((c) => (
              <button
                key={c}
                type="button"
                onClick={() => { set('category', c); setCustomCat('') }}
                className={`flex items-center justify-center py-2.5 px-1 rounded-xl text-xs font-semibold border-2 transition-all ${
                  form.category === c
                    ? 'bg-violet-50 dark:bg-violet-500/10 border-violet-300 dark:border-violet-500/30 text-violet-700 dark:text-violet-400'
                    : 'bg-white dark:bg-transparent border-gray-200 dark:border-gray-700 text-gray-600 dark:text-gray-400 hover:border-violet-200 dark:hover:border-violet-500/30 hover:bg-violet-50/50 dark:hover:bg-violet-500/5 hover:text-violet-700 dark:hover:text-violet-400'
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
              className={`flex-1 text-xs rounded-xl border-2 py-2 px-3 outline-none transition-all placeholder-gray-300 dark:placeholder-gray-600 ${
                !CATEGORY_KEYS.includes(form.category as typeof CATEGORY_KEYS[number]) && customCat.trim() !== ''
                  ? 'border-violet-500 dark:border-violet-400 bg-violet-50 dark:bg-violet-500/10 text-violet-700 dark:text-violet-400 font-semibold'
                  : 'border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800/50 text-gray-600 dark:text-gray-300 focus:border-violet-300 dark:focus:border-violet-500/30 focus:bg-violet-50 dark:focus:bg-violet-500/5'
              }`}
            />
          </div>
        </div>

        {/* Project + Note */}
        <div className="md:col-span-2 bg-white dark:bg-[hsl(260,15%,11%)] rounded-2xl border border-gray-100/80 dark:border-gray-800/50 p-5 shadow-sm space-y-4">
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

        {/* Error + Actions */}
        <div className="md:col-span-2 space-y-3">
          {error && (
            <div className="bg-rose-50 dark:bg-rose-500/10 border border-rose-200 dark:border-rose-500/30 text-rose-700 dark:text-rose-400 rounded-xl px-4 py-3 text-sm flex items-start gap-2">
              <svg className="w-4 h-4 shrink-0 mt-0.5" fill="currentColor" viewBox="0 0 20 20"><path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" /></svg>
              {error}
            </div>
          )}
          <div className="flex gap-3">
            <button
              type="submit"
              disabled={loading || success}
              className={`flex-1 font-semibold rounded-xl py-3 text-sm transition-all disabled:opacity-50 ${
                isExpense
                  ? 'bg-rose-500 hover:bg-rose-600 text-white'
                  : 'bg-emerald-500 hover:bg-emerald-600 text-white'
              }`}
            >
              {loading ? t('addTransaction.form.submitting') : t('addTransaction.form.submit')}
            </button>
            <button
              type="button"
              onClick={() => navigate(-1)}
              className="px-5 py-3 rounded-xl border-2 border-gray-200 dark:border-gray-700 text-sm text-gray-500 dark:text-gray-400 hover:bg-gray-50 dark:hover:bg-gray-800 hover:border-gray-300 dark:hover:border-gray-600 font-medium transition-all"
            >
              {t('common.cancel')}
            </button>
          </div>
        </div>

      </form>
    </div>
  )
}
