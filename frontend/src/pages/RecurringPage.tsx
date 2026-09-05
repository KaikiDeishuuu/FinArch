import { useMemo, useState } from 'react'
import type { FormEvent } from 'react'
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'
import Select from '../components/Select'
import { EmptyState, FinanceCard, SectionHeader } from '../components/FinancePrimitives'
import { useAccounts } from '../hooks/useAccounts'
import { useMode } from '../hooks/useMode'
import { useRecurringInstances, useRecurringMutations, useRecurringPreview, useRecurringRules } from '../hooks/useRecurringRules'
import type { RecurringRule, RecurringFrequency, MonthEndPolicy, UpsertRecurringRuleRequest } from '../api/client'
import { CATEGORY_KEYS, categoryLabel } from '../utils/categoryLabel'
import { CURRENCY_SYMBOLS, SUPPORTED_CURRENCIES } from '../constants/currencies'
import { formatAmount } from '../utils/format'

const WEEKDAYS = [0, 1, 2, 3, 4, 5, 6]

function todayDate() {
  return new Date().toISOString().slice(0, 10)
}

function localTimezone() {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'Local'
  } catch {
    return 'Local'
  }
}

function timeOfDay(value: string) {
  if (!value) return '09:00:00'
  return value.length === 5 ? `${value}:00` : value
}

interface RecurringFormState {
  id: string
  name: string
  direction: 'income' | 'expense'
  account_id: string
  category: string
  custom_category: string
  amount_yuan: string
  currency: string
  note: string
  project_id: string
  frequency: RecurringFrequency
  interval: string
  start_date: string
  end_date: string
  time_of_day: string
  timezone: string
  day_of_week: string
  day_of_month: string
  month_end_policy: MonthEndPolicy
  catch_up_enabled: boolean
}

function defaultForm(accountId = ''): RecurringFormState {
  const now = new Date()
  return {
    id: '',
    name: '',
    direction: 'expense',
    account_id: accountId,
    category: CATEGORY_KEYS[0],
    custom_category: '',
    amount_yuan: '',
    currency: 'CNY',
    note: '',
    project_id: '',
    frequency: 'monthly',
    interval: '1',
    start_date: todayDate(),
    end_date: '',
    time_of_day: '09:00',
    timezone: localTimezone(),
    day_of_week: String(now.getDay()),
    day_of_month: String(now.getDate()),
    month_end_policy: 'clamp',
    catch_up_enabled: true,
  }
}

function formFromRule(rule: RecurringRule): RecurringFormState {
  const isKnownCategory = CATEGORY_KEYS.includes(rule.category as typeof CATEGORY_KEYS[number])
  return {
    id: rule.id,
    name: rule.name,
    direction: rule.direction || rule.type,
    account_id: rule.account_id,
    category: rule.category,
    custom_category: isKnownCategory ? '' : rule.category,
    amount_yuan: String(rule.amount_yuan),
    currency: rule.currency || 'CNY',
    note: rule.note || '',
    project_id: rule.project_id || '',
    frequency: rule.frequency,
    interval: String(rule.interval || 1),
    start_date: rule.start_date,
    end_date: rule.end_date || '',
    time_of_day: (rule.time_of_day || '09:00:00').slice(0, 5),
    timezone: rule.timezone || localTimezone(),
    day_of_week: rule.day_of_week == null ? String(new Date(`${rule.start_date}T00:00:00`).getDay()) : String(rule.day_of_week),
    day_of_month: rule.day_of_month == null ? String(Number(rule.start_date.slice(8, 10)) || 1) : String(rule.day_of_month),
    month_end_policy: rule.month_end_policy || 'clamp',
    catch_up_enabled: rule.catch_up_enabled,
  }
}

function buildRequest(form: RecurringFormState): UpsertRecurringRuleRequest {
  const amount = Number(form.amount_yuan)
  const interval = Number(form.interval)
  const category = form.custom_category.trim() || form.category
  return {
    name: form.name.trim(),
    account_id: form.account_id,
    type: form.direction,
    direction: form.direction,
    category,
    amount_yuan: amount,
    currency: form.currency,
    note: form.note.trim(),
    project_id: form.project_id.trim(),
    frequency: form.frequency,
    interval: Number.isFinite(interval) && interval > 0 ? Math.floor(interval) : 1,
    start_date: form.start_date,
    end_date: form.end_date || '',
    time_of_day: timeOfDay(form.time_of_day),
    timezone: form.timezone.trim() || 'Local',
    day_of_week: form.frequency === 'weekly' ? Number(form.day_of_week) : null,
    day_of_month: form.frequency === 'monthly' || form.frequency === 'yearly' ? Number(form.day_of_month) : null,
    month_end_policy: form.month_end_policy,
    catch_up_enabled: form.catch_up_enabled,
  }
}

function formatSchedule(rule: RecurringRule, t: (key: string, values?: Record<string, unknown>) => string) {
  const interval = rule.interval || 1
  const freq = t(`recurring.frequency.${rule.frequency}`)
  return interval === 1 ? freq : t('recurring.everyInterval', { interval, frequency: freq })
}

function HistoryPanel({ ruleId }: { ruleId: string }) {
  const { t } = useTranslation()
  const { data: instances = [], isLoading } = useRecurringInstances(ruleId)
  return (
    <div className="mt-3 border-l-2 border-l-[hsl(var(--mode-accent))] bg-[hsl(var(--muted))]/55 p-3">
      <p className="mb-2 font-data text-[10px] font-semibold uppercase tracking-[0.12em] text-[hsl(var(--muted-foreground))]">{t('recurring.history')}</p>
      {isLoading ? (
        <div className="h-10 animate-pulse bg-[hsl(var(--border))]/70" />
      ) : instances.length === 0 ? (
        <p className="text-xs text-[hsl(var(--muted-foreground))]">{t('recurring.noHistory')}</p>
      ) : (
        <div className="divide-y divide-[hsl(var(--border))] border-y border-[hsl(var(--border))]">
          {instances.slice(0, 5).map((item) => (
            <div key={item.id} className="flex items-start justify-between gap-3 bg-[hsl(var(--card))]/70 px-3 py-2 text-xs">
              <div>
                <p className="font-data font-semibold text-[hsl(var(--foreground))]">{item.occurrence_date}</p>
                {item.error && <p className="mt-0.5 text-rose-500 dark:text-rose-300">{item.error}</p>}
              </div>
              <span className={`rounded-full px-2 py-0.5 font-bold ${item.status === 'generated'
                ? 'bg-emerald-50 text-emerald-600 dark:bg-emerald-500/15 dark:text-emerald-300'
                : item.status === 'failed'
                  ? 'bg-rose-50 text-rose-600 dark:bg-rose-500/15 dark:text-rose-300'
                  : 'bg-[hsl(var(--muted))] text-[hsl(var(--muted-foreground))]'
              }`}>{t(`recurring.instanceStatus.${item.status}`)}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

export default function RecurringPage() {
  const { t } = useTranslation()
  const { isWorkMode } = useMode()
  const { data: accounts = [] } = useAccounts()
  const { data: rules = [], isLoading } = useRecurringRules()
  const mutations = useRecurringMutations()
  const activeAccounts = useMemo(() => accounts.filter(a => a.is_active && a.type === (isWorkMode ? 'public' : 'personal')), [accounts, isWorkMode])
  const [form, setForm] = useState<RecurringFormState>(() => defaultForm())
  const [expandedRuleId, setExpandedRuleId] = useState<string | null>(null)

  const previewRequest = useMemo(() => buildRequest({ ...form, account_id: form.account_id || activeAccounts[0]?.id || '' }), [activeAccounts, form])
  const previewEnabled = Boolean((form.account_id || activeAccounts[0]?.id) && form.category && Number(form.amount_yuan) > 0 && form.start_date)
  const { data: preview = [] } = useRecurringPreview({ ...previewRequest, count: 5 }, previewEnabled)

  const failedCount = rules.filter(rule => rule.status === 'active' && rule.next_run_at === 0).length
  const nextRule = rules
    .filter(rule => rule.status === 'active')
    .sort((a, b) => (a.next_run_at || 0) - (b.next_run_at || 0))[0]

  function update<K extends keyof RecurringFormState>(key: K, value: RecurringFormState[K]) {
    setForm(prev => ({ ...prev, [key]: value }))
  }

  function resetForm() {
    setForm(defaultForm(activeAccounts[0]?.id || ''))
  }

  function startEdit(rule: RecurringRule) {
    setForm(formFromRule(rule))
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }

  async function submit(e: FormEvent) {
    e.preventDefault()
    const req = buildRequest({ ...form, account_id: form.account_id || activeAccounts[0]?.id || '' })
    if (!req.account_id) {
      toast.error(t('recurring.errors.accountRequired'))
      return
    }
    if (!req.amount_yuan || req.amount_yuan <= 0) {
      toast.error(t('recurring.errors.invalidAmount'))
      return
    }
    try {
      if (form.id) {
        await mutations.update.mutateAsync({ id: form.id, req })
        toast.success(t('recurring.toast.updated'))
      } else {
        await mutations.create.mutateAsync(req)
        toast.success(t('recurring.toast.created'))
      }
      resetForm()
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      toast.error(msg || t('recurring.toast.failed'))
    }
  }

  async function toggleStatus(rule: RecurringRule) {
    try {
      await mutations.setStatus.mutateAsync({ id: rule.id, status: rule.status === 'active' ? 'paused' : 'active' })
      toast.success(rule.status === 'active' ? t('recurring.toast.paused') : t('recurring.toast.resumed'))
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      toast.error(msg || t('recurring.toast.failed'))
    }
  }

  async function generateNow(rule: RecurringRule) {
    try {
      const result = await mutations.generateNow.mutateAsync(rule.id)
      toast.success(t('recurring.toast.generated', { count: result.generated }))
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      toast.error(msg || t('recurring.toast.failed'))
    }
  }

  async function removeRule(rule: RecurringRule) {
    if (!window.confirm(t('recurring.confirmDelete', { name: rule.name }))) return
    try {
      await mutations.remove.mutateAsync(rule.id)
      if (form.id === rule.id) resetForm()
      toast.success(t('recurring.toast.deleted'))
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      toast.error(msg || t('recurring.toast.failed'))
    }
  }

  const inputClass = 'w-full rounded-[3px] border border-[hsl(var(--border))] bg-[hsl(var(--background))] px-3.5 py-2.5 text-sm text-[hsl(var(--foreground))] outline-none transition-colors placeholder:text-[hsl(var(--muted-foreground))]/70 hover:bg-[hsl(var(--card))] focus:border-[hsl(var(--mode-accent))] focus:ring-2 focus:ring-[hsl(var(--ring))]/20 disabled:cursor-not-allowed disabled:opacity-50'
  const labelClass = 'mb-1.5 block font-data text-[10px] font-semibold uppercase tracking-[0.12em] text-[hsl(var(--muted-foreground))]'

  return (
    <div className="space-y-5 md:space-y-6">
      <div className="ledger-rail flex flex-col gap-4 border-y border-[hsl(var(--border))] py-5 pl-5 pr-2 sm:flex-row sm:items-end sm:justify-between sm:pl-7">
        <div>
          <p className="page-kicker mb-2">FINARCH / SCHEDULE</p>
          <h1 className="font-display text-2xl font-semibold tracking-[-0.025em] text-[hsl(var(--foreground))]">{t('recurring.title')}</h1>
          <p className="mt-1 text-sm text-[hsl(var(--muted-foreground))]">{t('recurring.subtitle')}</p>
        </div>
        <div className="grid grid-cols-2 gap-px overflow-hidden border border-[hsl(var(--border))] bg-[hsl(var(--border))] sm:min-w-[18rem]">
          <div className="bg-[hsl(var(--card))] px-3 py-2.5">
            <p className="font-data text-[10px] font-semibold uppercase tracking-[0.12em] text-[hsl(var(--muted-foreground))]">{t('recurring.summary.active')}</p>
            <p className="font-data mt-1 text-xl font-semibold text-[hsl(var(--mode-accent))]">{rules.filter(r => r.status === 'active').length}</p>
          </div>
          <div className="bg-[hsl(var(--card))] px-3 py-2.5">
            <p className="font-data text-[10px] font-semibold uppercase tracking-[0.12em] text-[hsl(var(--muted-foreground))]">{t('recurring.summary.failed')}</p>
            <p className="font-data mt-1 text-xl font-semibold text-rose-500 dark:text-rose-300">{failedCount}</p>
          </div>
        </div>
      </div>

      <FinanceCard className="rounded-[3px] border-[hsl(var(--border))] border-t-2 border-t-[hsl(var(--mode-accent))] bg-[hsl(var(--card))] shadow-none [&_h2]:font-display [&_h2]:text-base [&_h2]:text-[hsl(var(--foreground))]">
        <SectionHeader title={form.id ? t('recurring.form.editTitle') : t('recurring.form.createTitle')} subtitle={t('recurring.form.subtitle')} />
        <form onSubmit={submit} className="space-y-4">
          <div className="grid gap-3 md:grid-cols-3">
            <div>
              <label className={labelClass}>{t('recurring.form.name')}</label>
              <input className={inputClass} value={form.name} onChange={(e) => update('name', e.target.value)} placeholder={t('recurring.form.namePlaceholder')} />
            </div>
            <div>
              <label className={labelClass}>{t('addTransaction.form.account')}</label>
              <Select
                value={form.account_id || activeAccounts[0]?.id || ''}
                onChange={(v) => update('account_id', v)}
                size="lg"
                options={activeAccounts.map(account => ({ value: account.id, label: `${account.name} · ${account.currency}` }))}
                disabled={activeAccounts.length === 0}
              />
            </div>
            <div>
              <label className={labelClass}>{t('addTransaction.form.direction')}</label>
              <Select
                value={form.direction}
                onChange={(v) => update('direction', v as 'income' | 'expense')}
                size="lg"
                options={[{ value: 'expense', label: t('common.expense') }, { value: 'income', label: t('common.income') }]}
              />
            </div>
          </div>

          <div className="grid gap-3 md:grid-cols-[1fr_1fr_1fr]">
            <div>
              <label className={labelClass}>{t('addTransaction.form.amount')}</label>
              <div className="flex items-center gap-2 rounded-[3px] border border-[hsl(var(--border))] bg-[hsl(var(--background))] px-3 py-1 transition-colors focus-within:border-[hsl(var(--mode-accent))] focus-within:ring-2 focus-within:ring-[hsl(var(--ring))]/20">
                <span className="font-data text-sm font-bold text-[hsl(var(--muted-foreground))]">{CURRENCY_SYMBOLS[form.currency] ?? form.currency}</span>
                <input type="number" min="0.01" step="0.01" className="font-data min-w-0 flex-1 bg-transparent py-2 text-sm font-semibold text-[hsl(var(--foreground))] outline-none" value={form.amount_yuan} onChange={(e) => update('amount_yuan', e.target.value)} placeholder="0.00" />
                <div className="w-20 shrink-0">
                  <Select value={form.currency} onChange={(v) => update('currency', v)} size="sm" options={SUPPORTED_CURRENCIES.map(c => ({ value: c.code, label: c.code }))} />
                </div>
              </div>
            </div>
            <div>
              <label className={labelClass}>{t('addTransaction.form.category')}</label>
              <Select value={CATEGORY_KEYS.includes(form.category as typeof CATEGORY_KEYS[number]) ? form.category : ''} onChange={(v) => { update('category', v); update('custom_category', '') }} size="lg" options={CATEGORY_KEYS.map(c => ({ value: c, label: categoryLabel(c) }))} />
            </div>
            <div>
              <label className={labelClass}>{t('addTransaction.form.custom')}</label>
              <input className={inputClass} value={form.custom_category} onChange={(e) => { update('custom_category', e.target.value); if (e.target.value.trim()) update('category', e.target.value.trim()) }} placeholder={t('addTransaction.form.customPlaceholder')} />
            </div>
          </div>

          <div className="grid gap-3 md:grid-cols-4">
            <div>
              <label className={labelClass}>{t('recurring.form.frequency')}</label>
              <Select value={form.frequency} onChange={(v) => update('frequency', v as RecurringFrequency)} size="lg" options={(['daily', 'weekly', 'monthly', 'yearly'] as RecurringFrequency[]).map(freq => ({ value: freq, label: t(`recurring.frequency.${freq}`) }))} />
            </div>
            <div>
              <label className={labelClass}>{t('recurring.form.interval')}</label>
              <input type="number" min="1" step="1" className={inputClass} value={form.interval} onChange={(e) => update('interval', e.target.value)} />
            </div>
            <div>
              <label className={labelClass}>{t('recurring.form.startDate')}</label>
              <input type="date" className={inputClass} value={form.start_date} onChange={(e) => update('start_date', e.target.value)} />
            </div>
            <div>
              <label className={labelClass}>{t('recurring.form.timeOfDay')}</label>
              <input type="time" className={inputClass} value={form.time_of_day} onChange={(e) => update('time_of_day', e.target.value)} />
            </div>
          </div>

          <div className="grid gap-3 md:grid-cols-4">
            <div>
              <label className={labelClass}>{t('recurring.form.endDate')}</label>
              <input type="date" className={inputClass} value={form.end_date} onChange={(e) => update('end_date', e.target.value)} />
            </div>
            <div>
              <label className={labelClass}>{t('recurring.form.timezone')}</label>
              <input className={inputClass} value={form.timezone} onChange={(e) => update('timezone', e.target.value)} placeholder="Asia/Shanghai" />
            </div>
            <div>
              <label className={labelClass}>{t('recurring.form.weekday')}</label>
              <Select value={form.day_of_week} onChange={(v) => update('day_of_week', v)} size="lg" disabled={form.frequency !== 'weekly'} options={WEEKDAYS.map(day => ({ value: String(day), label: t(`recurring.weekdays.${day}`) }))} />
            </div>
            <div>
              <label className={labelClass}>{t('recurring.form.monthDay')}</label>
              <input type="number" min="1" max="31" className={inputClass} disabled={form.frequency !== 'monthly' && form.frequency !== 'yearly'} value={form.day_of_month} onChange={(e) => update('day_of_month', e.target.value)} />
            </div>
          </div>

          <div className="grid gap-3 md:grid-cols-[1fr_1fr_1fr]">
            <div>
              <label className={labelClass}>{t('recurring.form.monthEndPolicy')}</label>
              <Select value={form.month_end_policy} onChange={(v) => update('month_end_policy', v as MonthEndPolicy)} size="lg" options={[{ value: 'clamp', label: t('recurring.monthEndPolicy.clamp') }, { value: 'skip', label: t('recurring.monthEndPolicy.skip') }]} />
            </div>
            <div>
              <label className={labelClass}>{t('addTransaction.form.project')} <span className="font-normal text-[hsl(var(--muted-foreground))]/60">{t('addTransaction.form.optional')}</span></label>
              <input className={inputClass} value={form.project_id} onChange={(e) => update('project_id', e.target.value)} placeholder={t('addTransaction.form.projectPlaceholder')} />
            </div>
            <div>
              <label className={labelClass}>{t('addTransaction.form.note')} <span className="font-normal text-[hsl(var(--muted-foreground))]/60">{t('addTransaction.form.optional')}</span></label>
              <input className={inputClass} value={form.note} onChange={(e) => update('note', e.target.value)} placeholder={t('addTransaction.form.notePlaceholder')} />
            </div>
          </div>

          <label className="flex items-center gap-2 text-xs font-medium text-[hsl(var(--muted-foreground))]">
            <input type="checkbox" checked={form.catch_up_enabled} onChange={(e) => update('catch_up_enabled', e.target.checked)} className="rounded-[2px] border-[hsl(var(--border))] accent-[hsl(var(--mode-accent))] focus:ring-[hsl(var(--ring))]" />
            {t('recurring.form.catchUp')}
          </label>

          <div className="border border-[hsl(var(--border))] border-l-2 border-l-[hsl(var(--mode-accent))] bg-[hsl(var(--muted))]/45 p-3">
            <p className="mb-2 font-data text-[10px] font-semibold uppercase tracking-[0.12em] text-[hsl(var(--mode-accent))]">{t('recurring.previewTitle')}</p>
            {preview.length > 0 ? (
              <div className="flex flex-wrap gap-2">
                {preview.map((item) => <span key={item.occurrence_date} className="border border-[hsl(var(--border))] bg-[hsl(var(--card))] px-2.5 py-1 font-data text-xs font-semibold text-[hsl(var(--mode-accent))]">{item.occurred_at}</span>)}
              </div>
            ) : (
              <p className="text-xs text-[hsl(var(--muted-foreground))]">{t('recurring.previewEmpty')}</p>
            )}
          </div>

          <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
            {form.id && <button type="button" onClick={resetForm} className="rounded-[3px] border border-[hsl(var(--border))] px-4 py-2 text-sm font-semibold text-[hsl(var(--muted-foreground))] transition-colors hover:bg-[hsl(var(--muted))]">{t('common.cancel')}</button>}
            <button type="submit" disabled={mutations.create.isPending || mutations.update.isPending || activeAccounts.length === 0} className="rounded-[3px] bg-[hsl(var(--mode-accent))] px-5 py-2 text-sm font-semibold text-[hsl(var(--primary-foreground))] transition-colors hover:brightness-95 disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[hsl(var(--ring))] focus-visible:ring-offset-2">
              {mutations.create.isPending || mutations.update.isPending ? t('common.saving') : form.id ? t('common.save') : t('common.add')}
            </button>
          </div>
        </form>
      </FinanceCard>

      <FinanceCard className="rounded-[3px] border-[hsl(var(--border))] bg-[hsl(var(--card))] shadow-none [&_h2]:font-display [&_h2]:text-base [&_h2]:text-[hsl(var(--foreground))]">
        <SectionHeader
          title={t('recurring.listTitle')}
          subtitle={nextRule ? t('recurring.nextDue', { name: nextRule.name, time: nextRule.next_occurred_at }) : t('recurring.noNextDue')}
        />
        {isLoading ? (
          <div className="h-24 animate-pulse rounded-[2px] bg-[hsl(var(--muted))]" />
        ) : rules.length === 0 ? (
          <EmptyState title={t('recurring.empty.title')} description={t('recurring.empty.desc')} />
        ) : (
          <div className="grid gap-px overflow-hidden border border-[hsl(var(--border))] bg-[hsl(var(--border))] lg:grid-cols-2">
            {rules.map(rule => (
              <div key={rule.id} className="bg-[hsl(var(--card))] p-4">
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <h3 className="truncate text-sm font-semibold text-[hsl(var(--foreground))]">{rule.name}</h3>
                      <span className={`border px-2 py-0.5 font-data text-[10px] font-bold ${rule.status === 'active' ? 'border-emerald-300/60 bg-emerald-50 text-emerald-700 dark:border-emerald-500/30 dark:bg-emerald-500/10 dark:text-emerald-300' : 'border-[hsl(var(--border))] bg-[hsl(var(--muted))] text-[hsl(var(--muted-foreground))]'}`}>{t(`recurring.status.${rule.status}`)}</span>
                    </div>
                    <p className="font-data mt-1 text-xs text-[hsl(var(--muted-foreground))]">{formatSchedule(rule, t)} · {rule.next_occurred_at}</p>
                  </div>
                  <p className={`font-data shrink-0 text-base font-semibold tabular-nums ${rule.direction === 'income' ? 'text-emerald-600 dark:text-emerald-300' : 'text-rose-500 dark:text-rose-300'}`}>{rule.direction === 'income' ? '+' : '−'}{formatAmount(rule.amount_yuan, rule.currency)}</p>
                </div>
                <div className="mt-3 flex flex-wrap items-center gap-1.5 text-xs">
                  <span className="border border-[hsl(var(--border))] bg-[hsl(var(--muted))]/60 px-2 py-1 font-semibold text-[hsl(var(--foreground))]">{categoryLabel(rule.category)}</span>
                  {rule.project_id && <span className="border border-[hsl(var(--border))] bg-[hsl(var(--mode-accent-wash))] px-2 py-1 font-data font-semibold text-[hsl(var(--mode-accent))]">{rule.project_id}</span>}
                  {rule.note && <span className="min-w-0 truncate border border-[hsl(var(--border))] px-2 py-1 text-[hsl(var(--muted-foreground))]">{rule.note}</span>}
                </div>
                <div className="mt-3 flex flex-wrap justify-end gap-2 border-t border-[hsl(var(--border))] pt-3">
                  <button type="button" aria-expanded={expandedRuleId === rule.id} onClick={() => setExpandedRuleId(expandedRuleId === rule.id ? null : rule.id)} className="rounded-[2px] px-3 py-1.5 text-xs font-semibold text-[hsl(var(--muted-foreground))] transition-colors hover:bg-[hsl(var(--muted))] hover:text-[hsl(var(--foreground))]">{expandedRuleId === rule.id ? t('common.collapse') : t('recurring.history')}</button>
                  <button type="button" onClick={() => generateNow(rule)} disabled={mutations.generateNow.isPending} className="rounded-[2px] px-3 py-1.5 text-xs font-semibold text-[hsl(var(--mode-accent))] transition-colors hover:bg-[hsl(var(--mode-accent-wash))] disabled:opacity-50">{t('recurring.generateNow')}</button>
                  <button type="button" onClick={() => toggleStatus(rule)} disabled={mutations.setStatus.isPending || rule.status === 'ended'} className="rounded-[2px] px-3 py-1.5 text-xs font-semibold text-amber-600 transition-colors hover:bg-amber-50 disabled:opacity-50 dark:text-amber-300 dark:hover:bg-amber-500/10">{rule.status === 'active' ? t('recurring.pause') : t('recurring.resume')}</button>
                  <button type="button" onClick={() => startEdit(rule)} className="rounded-[2px] px-3 py-1.5 text-xs font-semibold text-[hsl(var(--mode-accent))] transition-colors hover:bg-[hsl(var(--mode-accent-wash))]">{t('common.edit')}</button>
                  <button type="button" onClick={() => removeRule(rule)} disabled={mutations.remove.isPending} className="rounded-[2px] px-3 py-1.5 text-xs font-semibold text-rose-500 transition-colors hover:bg-rose-50 disabled:opacity-50 dark:text-rose-300 dark:hover:bg-rose-500/10">{t('common.delete')}</button>
                </div>
                {expandedRuleId === rule.id && <HistoryPanel ruleId={rule.id} />}
              </div>
            ))}
          </div>
        )}
      </FinanceCard>
    </div>
  )
}
