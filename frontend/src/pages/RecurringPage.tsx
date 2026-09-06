import { useMemo, useState } from 'react'
import type { FormEvent } from 'react'
import { CalendarClock, History, Pencil, Play, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'
import Select from '../components/Select'
import { Badge } from '../components/ui/badge'
import { Button } from '../components/ui/button'
import { Card, CardContent, CardDescription, CardHeader } from '../components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '../components/ui/dialog'
import { EmptyState } from '../components/ui/empty-state'
import { Field, Input } from '../components/ui/input'
import { PageHeader } from '../components/ui/page-header'
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
    <div className="mt-3 rounded-lg border border-border bg-card p-3">
      <p className="mb-2 flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
        <History className="size-3.5" />
        {t('recurring.history')}
      </p>
      {isLoading ? (
        <div className="h-10 animate-pulse rounded-lg bg-muted" />
      ) : instances.length === 0 ? (
        <p className="text-xs text-muted-foreground">{t('recurring.noHistory')}</p>
      ) : (
        <div className="space-y-2">
          {instances.slice(0, 5).map((item) => (
            <div key={item.id} className="flex items-start justify-between gap-3 rounded-lg border border-border bg-background px-3 py-2 text-xs">
              <div>
                <p className="font-medium text-foreground tabular-nums">{item.occurrence_date}</p>
                {item.error ? <p className="mt-0.5 text-negative">{item.error}</p> : null}
              </div>
              <Badge variant={item.status === 'generated' ? 'positive' : item.status === 'failed' ? 'negative' : 'neutral'} dot>
                {t(`recurring.instanceStatus.${item.status}`)}
              </Badge>
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
  const [deleteTarget, setDeleteTarget] = useState<RecurringRule | null>(null)

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
    try {
      await mutations.remove.mutateAsync(rule.id)
      if (form.id === rule.id) resetForm()
      setDeleteTarget(null)
      toast.success(t('recurring.toast.deleted'))
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      toast.error(msg || t('recurring.toast.failed'))
    }
  }

  const labelClass = 'mb-1.5 block text-xs font-medium text-muted-foreground'

  return (
    <div className="space-y-6">
      <PageHeader
        title={t('recurring.title')}
        description={t('recurring.subtitle')}
        actions={(
          <div className="grid min-w-64 grid-cols-2 gap-2">
            <Card className="p-3 shadow-none">
              <p className="text-[11px] font-medium text-muted-foreground">{t('recurring.summary.active')}</p>
              <p className="mt-1 text-xl font-semibold text-positive tabular-nums">{rules.filter(r => r.status === 'active').length}</p>
            </Card>
            <Card className="p-3 shadow-none">
              <p className="text-[11px] font-medium text-muted-foreground">{t('recurring.summary.failed')}</p>
              <p className="mt-1 text-xl font-semibold text-negative tabular-nums">{failedCount}</p>
            </Card>
          </div>
        )}
      />

      <Card>
        <CardHeader>
          <div>
            <h2 className="text-sm font-semibold tracking-tight">{form.id ? t('recurring.form.editTitle') : t('recurring.form.createTitle')}</h2>
            <CardDescription>{t('recurring.form.subtitle')}</CardDescription>
          </div>
          {form.id ? <Badge variant="accent">{t('common.edit')}</Badge> : null}
        </CardHeader>
        <CardContent>
          <form onSubmit={submit} className="space-y-5">
            <div className="grid gap-4 md:grid-cols-3">
              <Field label={t('recurring.form.name')}>
                <Input value={form.name} onChange={(e) => update('name', e.target.value)} placeholder={t('recurring.form.namePlaceholder')} />
              </Field>
              <div>
                <label className={labelClass}>{t('addTransaction.form.account')}</label>
                <Select aria-label={t('addTransaction.form.account')} value={form.account_id || activeAccounts[0]?.id || ''} onChange={(v) => update('account_id', v)} size="lg" options={activeAccounts.map(account => ({ value: account.id, label: `${account.name} · ${account.currency}` }))} disabled={activeAccounts.length === 0} />
              </div>
              <div>
                <label className={labelClass}>{t('addTransaction.form.direction')}</label>
                <Select aria-label={t('addTransaction.form.direction')} value={form.direction} onChange={(v) => update('direction', v as 'income' | 'expense')} size="lg" options={[{ value: 'expense', label: t('common.expense') }, { value: 'income', label: t('common.income') }]} />
              </div>
            </div>

            <div className="grid gap-4 md:grid-cols-3">
              <div>
                <label htmlFor="recurring-amount" className={labelClass}>{t('addTransaction.form.amount')}</label>
                <div className="flex h-10 items-center gap-2 rounded-lg border border-input bg-card px-3 shadow-xs focus-within:border-ring focus-within:ring-2 focus-within:ring-ring/20">
                  <span className="text-sm font-semibold text-muted-foreground">{CURRENCY_SYMBOLS[form.currency] ?? form.currency}</span>
                  <input id="recurring-amount" type="number" min="0.01" step="0.01" className="min-w-0 flex-1 bg-transparent text-sm font-semibold text-foreground outline-none placeholder:text-subtle" value={form.amount_yuan} onChange={(e) => update('amount_yuan', e.target.value)} placeholder="0.00" />
                  <div className="w-20 shrink-0"><Select aria-label={t('addTransaction.form.currency')} value={form.currency} onChange={(v) => update('currency', v)} size="sm" options={SUPPORTED_CURRENCIES.map(c => ({ value: c.code, label: c.code }))} /></div>
                </div>
              </div>
              <div>
                <label className={labelClass}>{t('addTransaction.form.category')}</label>
                <Select aria-label={t('addTransaction.form.category')} value={CATEGORY_KEYS.includes(form.category as typeof CATEGORY_KEYS[number]) ? form.category : ''} onChange={(v) => { update('category', v); update('custom_category', '') }} size="lg" options={CATEGORY_KEYS.map(c => ({ value: c, label: categoryLabel(c) }))} />
              </div>
              <Field label={t('addTransaction.form.custom')}>
                <Input value={form.custom_category} onChange={(e) => { update('custom_category', e.target.value); if (e.target.value.trim()) update('category', e.target.value.trim()) }} placeholder={t('addTransaction.form.customPlaceholder')} />
              </Field>
            </div>

            <div className="grid gap-4 md:grid-cols-4">
              <div>
                <label className={labelClass}>{t('recurring.form.frequency')}</label>
                <Select aria-label={t('recurring.form.frequency')} value={form.frequency} onChange={(v) => update('frequency', v as RecurringFrequency)} size="lg" options={(['daily', 'weekly', 'monthly', 'yearly'] as RecurringFrequency[]).map(freq => ({ value: freq, label: t(`recurring.frequency.${freq}`) }))} />
              </div>
              <Field label={t('recurring.form.interval')}><Input type="number" min="1" step="1" value={form.interval} onChange={(e) => update('interval', e.target.value)} /></Field>
              <Field label={t('recurring.form.startDate')}><Input type="date" value={form.start_date} onChange={(e) => update('start_date', e.target.value)} /></Field>
              <Field label={t('recurring.form.timeOfDay')}><Input type="time" value={form.time_of_day} onChange={(e) => update('time_of_day', e.target.value)} /></Field>
            </div>

            <div className="grid gap-4 md:grid-cols-4">
              <Field label={t('recurring.form.endDate')}><Input type="date" value={form.end_date} onChange={(e) => update('end_date', e.target.value)} /></Field>
              <Field label={t('recurring.form.timezone')}><Input value={form.timezone} onChange={(e) => update('timezone', e.target.value)} placeholder="Asia/Shanghai" /></Field>
              <div>
                <label className={labelClass}>{t('recurring.form.weekday')}</label>
                <Select aria-label={t('recurring.form.weekday')} value={form.day_of_week} onChange={(v) => update('day_of_week', v)} size="lg" disabled={form.frequency !== 'weekly'} options={WEEKDAYS.map(day => ({ value: String(day), label: t(`recurring.weekdays.${day}`) }))} />
              </div>
              <Field label={t('recurring.form.monthDay')}><Input type="number" min="1" max="31" disabled={form.frequency !== 'monthly' && form.frequency !== 'yearly'} value={form.day_of_month} onChange={(e) => update('day_of_month', e.target.value)} /></Field>
            </div>

            <div className="grid gap-4 md:grid-cols-3">
              <div>
                <label className={labelClass}>{t('recurring.form.monthEndPolicy')}</label>
                <Select aria-label={t('recurring.form.monthEndPolicy')} value={form.month_end_policy} onChange={(v) => update('month_end_policy', v as MonthEndPolicy)} size="lg" options={[{ value: 'clamp', label: t('recurring.monthEndPolicy.clamp') }, { value: 'skip', label: t('recurring.monthEndPolicy.skip') }]} />
              </div>
              <Field label={`${t('addTransaction.form.project')} · ${t('addTransaction.form.optional')}`}><Input value={form.project_id} onChange={(e) => update('project_id', e.target.value)} placeholder={t('addTransaction.form.projectPlaceholder')} /></Field>
              <Field label={`${t('addTransaction.form.note')} · ${t('addTransaction.form.optional')}`}><Input value={form.note} onChange={(e) => update('note', e.target.value)} placeholder={t('addTransaction.form.notePlaceholder')} /></Field>
            </div>

            <label className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
              <input type="checkbox" checked={form.catch_up_enabled} onChange={(e) => update('catch_up_enabled', e.target.checked)} className="size-4 rounded border-input accent-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/30" />
              {t('recurring.form.catchUp')}
            </label>

            <div className="rounded-lg border border-dashed border-accent/35 bg-accent-soft p-3">
              <p className="mb-2 flex items-center gap-1.5 text-xs font-medium text-accent"><CalendarClock className="size-3.5" />{t('recurring.previewTitle')}</p>
              {preview.length > 0 ? (
                <div className="flex flex-wrap gap-2">
                  {preview.map((item) => <Badge key={item.occurrence_date} variant="accent" className="font-mono">{item.occurred_at}</Badge>)}
                </div>
              ) : <p className="text-xs text-muted-foreground">{t('recurring.previewEmpty')}</p>}
            </div>

            <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
              {form.id ? <Button type="button" variant="outline" onClick={resetForm}>{t('common.cancel')}</Button> : null}
              <Button type="submit" loading={mutations.create.isPending || mutations.update.isPending} loadingText={t('common.saving')} disabled={activeAccounts.length === 0} className="min-w-24">
                {form.id ? t('common.save') : t('common.add')}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <div>
            <h2 className="text-sm font-semibold tracking-tight">{t('recurring.listTitle')}</h2>
            <CardDescription>{nextRule ? t('recurring.nextDue', { name: nextRule.name, time: nextRule.next_occurred_at }) : t('recurring.noNextDue')}</CardDescription>
          </div>
          <Badge variant="neutral">{rules.length}</Badge>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <div role="status" aria-label={t('common.loading')}>
              <div aria-hidden="true" className="h-28 animate-pulse rounded-lg bg-muted" />
            </div>
          ) : rules.length === 0 ? (
            <EmptyState title={t('recurring.empty.title')} description={t('recurring.empty.desc')} icon={<CalendarClock />} />
          ) : (
            <div className="grid gap-3 lg:grid-cols-2">
              {rules.map(rule => (
                <section key={rule.id} className="rounded-xl border border-border bg-background p-4">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <h3 className="truncate text-sm font-semibold text-foreground">{rule.name}</h3>
                        <Badge variant={rule.status === 'active' ? 'positive' : 'neutral'} dot>{t(`recurring.status.${rule.status}`)}</Badge>
                      </div>
                      <p className="mt-1 text-xs text-muted-foreground">{formatSchedule(rule, t)} · {rule.next_occurred_at}</p>
                    </div>
                    <p className={`shrink-0 text-base font-semibold tabular-nums ${rule.direction === 'income' ? 'text-positive' : 'text-negative'}`}>{rule.direction === 'income' ? '+' : '−'}{formatAmount(rule.amount_yuan, rule.currency)}</p>
                  </div>
                  <div className="mt-3 flex flex-wrap items-center gap-1.5 text-xs">
                    <Badge variant="neutral">{categoryLabel(rule.category)}</Badge>
                    {rule.project_id ? <Badge variant="accent">{rule.project_id}</Badge> : null}
                    {rule.note ? <span className="min-w-0 truncate text-muted-foreground">{rule.note}</span> : null}
                  </div>
                  <div className="mt-4 flex flex-wrap justify-end gap-1 border-t border-border pt-3">
                    <Button type="button" variant="ghost" size="sm" onClick={() => setExpandedRuleId(expandedRuleId === rule.id ? null : rule.id)}><History className="size-3.5" />{expandedRuleId === rule.id ? t('common.collapse') : t('recurring.history')}</Button>
                    <Button type="button" variant="ghost" size="sm" onClick={() => generateNow(rule)} disabled={mutations.generateNow.isPending}><Play className="size-3.5" />{t('recurring.generateNow')}</Button>
                    <Button type="button" variant="ghost" size="sm" onClick={() => toggleStatus(rule)} disabled={mutations.setStatus.isPending || rule.status === 'ended'}>{rule.status === 'active' ? t('recurring.pause') : t('recurring.resume')}</Button>
                    <Button type="button" variant="ghost" size="sm" onClick={() => startEdit(rule)}><Pencil className="size-3.5" />{t('common.edit')}</Button>
                    <Button type="button" variant="danger" size="sm" onClick={() => setDeleteTarget(rule)} disabled={mutations.remove.isPending}><Trash2 className="size-3.5" />{t('common.delete')}</Button>
                  </div>
                  {expandedRuleId === rule.id ? <HistoryPanel ruleId={rule.id} /> : null}
                </section>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      <Dialog open={deleteTarget !== null} onOpenChange={(open) => { if (!open && !mutations.remove.isPending) setDeleteTarget(null) }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('common.delete')}</DialogTitle>
            <DialogDescription>{deleteTarget ? t('recurring.confirmDelete', { name: deleteTarget.name }) : ''}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setDeleteTarget(null)} disabled={mutations.remove.isPending}>{t('common.cancel')}</Button>
            <Button type="button" variant="danger" loading={mutations.remove.isPending} loadingText={t('common.loading')} onClick={() => { if (deleteTarget) void removeRule(deleteTarget) }}>{t('common.delete')}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
