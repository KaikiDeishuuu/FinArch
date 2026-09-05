import { useMemo, useState } from 'react'
import type { FormEvent } from 'react'
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'
import Select from '../components/Select'
import CompactAmount from '../components/CompactAmount'
import { EmptyState, FinanceCard, SectionHeader } from '../components/FinancePrimitives'
import { useBudgetMutations, useBudgets, useBudgetSummary, currentBudgetMonth } from '../hooks/useBudgets'
import { useTransactions } from '../hooks/useTransactions'
import { useMode } from '../hooks/useMode'
import { formatAmountCompact, formatAmountExact } from '../utils/format'
import { categoryLabel } from '../utils/categoryLabel'
import type { Budget, BudgetProgress } from '../api/client'

function progressBarClass(status: BudgetProgress['status']) {
  if (status === 'over') return 'bg-rose-500'
  if (status === 'warning') return 'bg-amber-500'
  return 'bg-emerald-500'
}

function BudgetProgressCard({ progress, onEdit, onDelete, deleting }: {
  progress: BudgetProgress
  onEdit: (budget: Budget) => void
  onDelete: (budget: Budget) => void
  deleting: boolean
}) {
  const { t } = useTranslation()
  const budget = progress.budget
  const title = budget.category ? categoryLabel(budget.category) : t('budgets.totalBudget')
  const percent = Math.round(progress.usage_ratio * 100)
  const remaining = progress.remaining_yuan
  return (
    <FinanceCard className="space-y-3 rounded-[3px] border-[hsl(var(--border))] bg-[hsl(var(--card))] shadow-none">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="truncate text-sm font-semibold text-[hsl(var(--foreground))]">{title}</p>
          <p className="font-data mt-1 text-xs text-[hsl(var(--muted-foreground))]">{budget.period_month} · {budget.category ? t('budgets.categoryScoped') : t('budgets.monthScoped')}</p>
        </div>
        <span className={`rounded-full px-2 py-1 text-[10px] font-bold ${progress.status === 'over'
          ? 'bg-rose-50 text-rose-600 dark:bg-rose-500/15 dark:text-rose-300'
          : progress.status === 'warning'
            ? 'bg-amber-50 text-amber-600 dark:bg-amber-500/15 dark:text-amber-300'
            : 'bg-emerald-50 text-emerald-600 dark:bg-emerald-500/15 dark:text-emerald-300'
        }`}>{t(`budgets.status.${progress.status}`)}</span>
      </div>
      <div>
        <div className="mb-2 flex items-end justify-between gap-3">
          <div>
            <p className="font-data text-[10px] font-semibold uppercase tracking-[0.12em] text-[hsl(var(--muted-foreground))]">{t('budgets.actual')}</p>
            <p className="font-data mt-1 text-lg font-semibold tabular-nums text-[hsl(var(--foreground))]">
              <CompactAmount compact={formatAmountCompact(progress.actual_yuan, budget.base_currency)} exact={formatAmountExact(progress.actual_yuan, budget.base_currency)} />
            </p>
          </div>
          <div className="text-right">
            <p className="font-data text-[10px] font-semibold uppercase tracking-[0.12em] text-[hsl(var(--muted-foreground))]">{t('budgets.planned')}</p>
            <p className="font-data mt-1 text-sm font-semibold tabular-nums text-[hsl(var(--muted-foreground))]">
              <CompactAmount compact={formatAmountCompact(budget.base_amount_yuan, budget.base_currency)} exact={formatAmountExact(budget.base_amount_yuan, budget.base_currency)} />
            </p>
          </div>
        </div>
        <div
          role="progressbar"
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={Math.max(0, Math.min(100, percent))}
          className="h-1.5 overflow-hidden bg-[hsl(var(--muted))]"
        >
          <div className={`h-full ${progressBarClass(progress.status)}`} style={{ width: `${Math.max(0, Math.min(100, percent))}%` }} />
        </div>
        <div className="font-data mt-2 flex justify-between text-xs text-[hsl(var(--muted-foreground))]">
          <span>{percent}%</span>
          <span className={remaining < 0 ? 'text-rose-500 dark:text-rose-300' : ''}>{remaining < 0 ? t('budgets.overBy') : t('budgets.remaining')}: {formatAmountCompact(Math.abs(remaining), budget.base_currency)}</span>
        </div>
      </div>
      <div className="flex justify-end gap-2 border-t border-[hsl(var(--border))] pt-3">
        <button type="button" onClick={() => onEdit(budget)} className="rounded-[2px] px-3 py-1.5 text-xs font-semibold text-[hsl(var(--mode-accent))] transition-colors hover:bg-[hsl(var(--mode-accent-wash))] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[hsl(var(--ring))]">
          {t('common.edit')}
        </button>
        <button type="button" onClick={() => onDelete(budget)} disabled={deleting} className="rounded-lg px-3 py-1.5 text-xs font-semibold text-rose-500 transition-colors hover:bg-rose-50 disabled:opacity-50 dark:text-rose-300 dark:hover:bg-rose-500/10">
          {deleting ? t('common.loading') : t('common.delete')}
        </button>
      </div>
    </FinanceCard>
  )
}

export default function BudgetsPage() {
  const { t } = useTranslation()
  const { isWorkMode } = useMode()
  const [period, setPeriod] = useState(currentBudgetMonth())
  const { data: budgets = [], isLoading } = useBudgets(period)
  const { data: summary } = useBudgetSummary(period)
  const { data: transactions = [] } = useTransactions()
  const mutations = useBudgetMutations(period)
  const [editing, setEditing] = useState<Budget | null>(null)
  const [category, setCategory] = useState('')
  const [amount, setAmount] = useState('')

  const categories = useMemo(() => Array.from(new Set(
    transactions
      .filter(tx => tx.source === (isWorkMode ? 'company' : 'personal'))
      .filter(tx => tx.direction === 'expense')
      .map(tx => tx.category)
      .filter((category): category is string => !!category),
  )).sort(), [transactions, isWorkMode])

  const progressCards = useMemo(() => {
    const items: BudgetProgress[] = []
    if (summary?.total_budget) items.push(summary.total_budget)
    items.push(...(summary?.category_budgets ?? []))
    return items
  }, [summary])

  function resetForm() {
    setEditing(null)
    setCategory('')
    setAmount('')
  }

  function startEdit(budget: Budget) {
    setEditing(budget)
    setCategory(budget.category)
    setAmount(String(budget.amount_yuan))
  }

  async function submitBudget(e: FormEvent) {
    e.preventDefault()
    const value = Number(amount)
    if (!Number.isFinite(value) || value <= 0) {
      toast.error(t('budgets.errors.invalidAmount'))
      return
    }
    try {
      if (editing) {
        await mutations.update.mutateAsync({ id: editing.id, req: { period_month: period, category, amount_yuan: value, currency: editing.currency || 'CNY', base_currency: editing.base_currency || 'CNY', base_amount_cents: Math.round(value * 100) } })
        toast.success(t('budgets.toast.updated'))
      } else {
        await mutations.create.mutateAsync({ period_month: period, category, amount_yuan: value, currency: 'CNY', base_currency: 'CNY', base_amount_cents: Math.round(value * 100) })
        toast.success(t('budgets.toast.created'))
      }
      resetForm()
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      toast.error(msg || t('budgets.toast.failed'))
    }
  }

  async function removeBudget(budget: Budget) {
    try {
      await mutations.remove.mutateAsync(budget.id)
      if (editing?.id === budget.id) resetForm()
      toast.success(t('budgets.toast.deleted'))
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      toast.error(msg || t('budgets.toast.failed'))
    }
  }

  const inputClass = 'w-full rounded-[3px] border border-[hsl(var(--border))] bg-[hsl(var(--background))] px-3.5 py-2.5 font-data text-sm text-[hsl(var(--foreground))] outline-none transition-colors placeholder:text-[hsl(var(--muted-foreground))]/70 hover:bg-[hsl(var(--card))] focus:border-[hsl(var(--mode-accent))] focus:ring-2 focus:ring-[hsl(var(--ring))]/20'

  return (
    <div className="space-y-5 md:space-y-6">
      <div className="ledger-rail flex flex-col gap-3 border-y border-[hsl(var(--border))] py-5 pl-5 pr-2 sm:flex-row sm:items-start sm:justify-between sm:pl-7">
        <div>
          <p className="page-kicker mb-2">FINARCH / PLAN</p>
          <h1 className="font-display text-2xl font-semibold tracking-[-0.025em] text-[hsl(var(--foreground))]">{t('budgets.title')}</h1>
          <p className="mt-1 text-sm text-[hsl(var(--muted-foreground))]">{t('budgets.subtitle')}</p>
        </div>
        <input type="month" value={period} onChange={(e) => setPeriod(e.target.value || currentBudgetMonth())} className="h-10 rounded-[3px] border border-[hsl(var(--border))] bg-[hsl(var(--card))] px-3 font-data text-sm font-semibold text-[hsl(var(--foreground))] outline-none focus:border-[hsl(var(--mode-accent))] focus:ring-2 focus:ring-[hsl(var(--ring))]/20" />
      </div>

      <FinanceCard className="rounded-[3px] border-[hsl(var(--border))] border-t-2 border-t-[hsl(var(--mode-accent))] bg-[hsl(var(--card))] shadow-none [&_h2]:font-display [&_h2]:text-base [&_h2]:text-[hsl(var(--foreground))]">
        <SectionHeader title={editing ? t('budgets.form.editTitle') : t('budgets.form.createTitle')} subtitle={t('budgets.form.subtitle')} />
        <form onSubmit={submitBudget} className="grid gap-3 md:grid-cols-[1fr_1fr_auto] md:items-end">
          <div>
            <label className="mb-1.5 block font-data text-[10px] font-semibold uppercase tracking-[0.12em] text-[hsl(var(--muted-foreground))]">{t('budgets.form.scope')}</label>
            <Select
              value={category}
              onChange={setCategory}
              size="lg"
              options={[
                { value: '', label: t('budgets.totalBudget') },
                ...categories.map(c => ({ value: c, label: categoryLabel(c) })),
              ]}
            />
          </div>
          <div>
            <label className="mb-1.5 block font-data text-[10px] font-semibold uppercase tracking-[0.12em] text-[hsl(var(--muted-foreground))]">{t('budgets.form.amount')}</label>
            <input type="number" min="0" step="0.01" value={amount} onChange={(e) => setAmount(e.target.value)} className={inputClass} placeholder="3000" />
          </div>
          <div className="flex gap-2">
            {editing && <button type="button" onClick={resetForm} className="h-10 rounded-[3px] border border-[hsl(var(--border))] px-4 text-sm font-semibold text-[hsl(var(--muted-foreground))] transition-colors hover:bg-[hsl(var(--muted))]">{t('common.cancel')}</button>}
            <button type="submit" disabled={mutations.create.isPending || mutations.update.isPending} className="h-10 rounded-[3px] bg-[hsl(var(--mode-accent))] px-5 text-sm font-semibold text-[hsl(var(--primary-foreground))] transition-colors hover:brightness-95 disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[hsl(var(--ring))] focus-visible:ring-offset-2">
              {mutations.create.isPending || mutations.update.isPending ? t('common.saving') : editing ? t('common.save') : t('common.add')}
            </button>
          </div>
        </form>
      </FinanceCard>

      <FinanceCard className="rounded-[3px] border-[hsl(var(--border))] bg-[hsl(var(--card))] shadow-none [&_h2]:font-display [&_h2]:text-base [&_h2]:text-[hsl(var(--foreground))]">
        <SectionHeader title={t('budgets.overview.title')} subtitle={t('budgets.overview.subtitle', { period })} />
        {isLoading ? (
          <div className="h-24 animate-pulse rounded-[2px] bg-[hsl(var(--muted))]" />
        ) : budgets.length === 0 ? (
          <EmptyState title={t('budgets.empty.title')} description={t('budgets.empty.desc')} />
        ) : (
          <div className="grid gap-3 lg:grid-cols-2">
            {progressCards.map(progress => (
              <BudgetProgressCard key={progress.budget.id} progress={progress} onEdit={startEdit} onDelete={removeBudget} deleting={mutations.remove.isPending} />
            ))}
          </div>
        )}
      </FinanceCard>
    </div>
  )
}
