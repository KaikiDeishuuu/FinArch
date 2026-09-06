import { useMemo, useState } from 'react'
import type { FormEvent } from 'react'
import { CalendarDays, Gauge, Pencil, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'
import Select from '../components/Select'
import CompactAmount from '../components/CompactAmount'
import { Badge, type BadgeVariant } from '../components/ui/badge'
import { Button } from '../components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card'
import { EmptyState } from '../components/ui/empty-state'
import { Field, Input } from '../components/ui/input'
import { PageHeader } from '../components/ui/page-header'
import { ProgressBar } from '../components/ui/progress-bar'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '../components/ui/dialog'
import { useBudgetMutations, useBudgets, useBudgetSummary, currentBudgetMonth } from '../hooks/useBudgets'
import { useTransactions } from '../hooks/useTransactions'
import { useMode } from '../hooks/useMode'
import { formatAmountCompact, formatAmountExact } from '../utils/format'
import { categoryLabel } from '../utils/categoryLabel'
import type { Budget, BudgetProgress } from '../api/client'

function progressTone(status: BudgetProgress['status']): 'positive' | 'warning' | 'negative' {
  if (status === 'over') return 'negative'
  if (status === 'warning') return 'warning'
  return 'positive'
}

function progressBadge(status: BudgetProgress['status']): BadgeVariant {
  if (status === 'over') return 'negative'
  if (status === 'warning') return 'warning'
  return 'positive'
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
    <Card className="bg-background p-4 shadow-none transition-[border-color,box-shadow] hover:border-input hover:shadow-xs">
      <CardHeader>
        <div className="min-w-0">
          <CardTitle className="truncate text-foreground">{title}</CardTitle>
          <CardDescription>
            {budget.period_month} · {budget.category ? t('budgets.categoryScoped') : t('budgets.monthScoped')}
          </CardDescription>
        </div>
        <Badge variant={progressBadge(progress.status)} dot>
          {t(`budgets.status.${progress.status}`)}
        </Badge>
      </CardHeader>

      <CardContent className="mt-5">
        <div className="mb-2.5 flex items-end justify-between gap-3">
          <div>
            <p className="text-[11px] font-medium uppercase tracking-[0.08em] text-muted-foreground">{t('budgets.actual')}</p>
            <p className="mt-1 text-xl font-semibold tracking-tight text-foreground tabular-nums">
              <CompactAmount
                compact={formatAmountCompact(progress.actual_yuan, budget.base_currency)}
                exact={formatAmountExact(progress.actual_yuan, budget.base_currency)}
              />
            </p>
          </div>
          <div className="text-right">
            <p className="text-[11px] font-medium uppercase tracking-[0.08em] text-muted-foreground">{t('budgets.planned')}</p>
            <p className="mt-1 text-sm font-semibold text-muted-foreground tabular-nums">
              <CompactAmount
                compact={formatAmountCompact(budget.base_amount_yuan, budget.base_currency)}
                exact={formatAmountExact(budget.base_amount_yuan, budget.base_currency)}
              />
            </p>
          </div>
        </div>
        <ProgressBar
          value={progress.usage_ratio}
          max={1}
          tone={progressTone(progress.status)}
          label={`${title} ${t('budgets.actual')}`}
          aria-valuetext={`${percent}% · ${t(`budgets.status.${progress.status}`)}`}
        />
        <div className="mt-2 flex justify-between gap-3 text-xs text-muted-foreground">
          <span className="tabular-nums">{percent}%</span>
          <span className={remaining < 0 ? 'text-negative' : undefined}>
            {remaining < 0 ? t('budgets.overBy') : t('budgets.remaining')}: {formatAmountCompact(Math.abs(remaining), budget.base_currency)}
          </span>
        </div>
      </CardContent>

      <div className="mt-4 flex justify-end gap-1 border-t border-border pt-3">
        <Button type="button" variant="ghost" size="sm" onClick={() => onEdit(budget)}>
          <Pencil className="size-3.5" />
          {t('common.edit')}
        </Button>
        <Button type="button" variant="danger" size="sm" onClick={() => onDelete(budget)} disabled={deleting}>
          <Trash2 className="size-3.5" />
          {deleting ? t('common.loading') : t('common.delete')}
        </Button>
      </div>
    </Card>
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
  const [deleteTarget, setDeleteTarget] = useState<Budget | null>(null)
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
      setDeleteTarget(null)
      toast.success(t('budgets.toast.deleted'))
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      toast.error(msg || t('budgets.toast.failed'))
    }
  }

  const saving = mutations.create.isPending || mutations.update.isPending

  return (
    <div className="space-y-6">
      <PageHeader
        title={t('budgets.title')}
        description={t('budgets.subtitle')}
        actions={(
          <label className="relative block">
            <span className="sr-only">{t('budgets.overview.title')}</span>
            <CalendarDays aria-hidden="true" className="pointer-events-none absolute left-3 top-1/2 z-10 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              type="month"
              value={period}
              onChange={(e) => setPeriod(e.target.value || currentBudgetMonth())}
              className="w-auto min-w-40 pl-9 font-medium tabular-nums"
            />
          </label>
        )}
      />

      <Card>
        <CardHeader>
          <div>
            <h2 className="text-sm font-semibold tracking-tight">{editing ? t('budgets.form.editTitle') : t('budgets.form.createTitle')}</h2>
            <CardDescription>{t('budgets.form.subtitle')}</CardDescription>
          </div>
          {editing ? <Badge variant="accent">{t('common.edit')}</Badge> : null}
        </CardHeader>
        <CardContent>
          <form onSubmit={submitBudget} className="grid gap-4 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] md:items-end">
            <div className="grid gap-1.5">
              <label id="budget-scope-label" className="text-xs font-medium text-muted-foreground">{t('budgets.form.scope')}</label>
              <Select
                aria-labelledby="budget-scope-label"
                value={category}
                onChange={setCategory}
                size="lg"
                options={[
                  { value: '', label: t('budgets.totalBudget') },
                  ...categories.map(c => ({ value: c, label: categoryLabel(c) })),
                ]}
              />
            </div>
            <Field label={t('budgets.form.amount')}>
              <Input type="number" min="0" step="0.01" value={amount} onChange={(e) => setAmount(e.target.value)} placeholder="3000" />
            </Field>
            <div className="flex gap-2">
              {editing ? (
                <Button type="button" variant="outline" onClick={resetForm}>
                  {t('common.cancel')}
                </Button>
              ) : null}
              <Button type="submit" loading={saving} loadingText={t('common.saving')} className="min-w-24">
                {editing ? t('common.save') : t('common.add')}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <div>
            <h2 className="text-sm font-semibold tracking-tight">{t('budgets.overview.title')}</h2>
            <CardDescription>{t('budgets.overview.subtitle', { period })}</CardDescription>
          </div>
          <Badge variant="neutral" className="font-mono">{period}</Badge>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <div role="status" aria-label={t('common.loading')}>
              <div aria-hidden="true" className="h-28 animate-pulse rounded-lg bg-muted" />
            </div>
          ) : budgets.length === 0 ? (
            <EmptyState
              title={t('budgets.empty.title')}
              description={t('budgets.empty.desc')}
              icon={<Gauge />}
            />
          ) : (
            <div className="grid gap-3 lg:grid-cols-2">
              {progressCards.map(progress => (
                <BudgetProgressCard
                  key={progress.budget.id}
                  progress={progress}
                  onEdit={startEdit}
                  onDelete={setDeleteTarget}
                  deleting={mutations.remove.isPending}
                />
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      <Dialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open && !mutations.remove.isPending) setDeleteTarget(null)
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('common.delete')}</DialogTitle>
            <DialogDescription>
              {deleteTarget
                ? t('budgets.confirmDelete', {
                    name: deleteTarget.category ? categoryLabel(deleteTarget.category) : t('budgets.totalBudget'),
                  })
                : ''}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => setDeleteTarget(null)}
              disabled={mutations.remove.isPending}
            >
              {t('common.cancel')}
            </Button>
            <Button
              type="button"
              variant="danger"
              loading={mutations.remove.isPending}
              loadingText={t('common.loading')}
              onClick={() => {
                if (deleteTarget) void removeBudget(deleteTarget)
              }}
            >
              {t('common.delete')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
