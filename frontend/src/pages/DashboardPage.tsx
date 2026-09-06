import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import {
  ArrowRight,
  BarChart3,
  Check,
  CircleCheck,
  Globe2,
  List,
  PenLine,
  Plus,
  Repeat2,
  SearchCheck,
  Sparkles,
  Upload,
  WalletCards,
  type LucideIcon,
} from 'lucide-react'
import { useAuth } from '../hooks/useAuth'
import { useExchangeRates } from '../hooks/useExchangeRates'
import { formatAmountCompact, formatAmountExact } from '../utils/format'
import { accountBalanceToCNY, transactionAmountToCNY } from '../utils/financeAmounts'
import { formatGreeting, normalizeGreetingLocale } from '../utils/greeting'
import { secureRandomInt } from '../utils/secureRandom'
import CompactAmount from '../components/CompactAmount'
import { useTransactions } from '../hooks/useTransactions'
import { useAccounts } from '../hooks/useAccounts'
import { useHeartbeat } from '../hooks/useHeartbeat'
import { useOnlineDevices } from '../hooks/useOnlineDevices'
import { useMode } from '../hooks/useMode'
import { useBudgetSummary, currentBudgetMonth } from '../hooks/useBudgets'
import { useRecurringRules } from '../hooks/useRecurringRules'
import { categoryLabel } from '../utils/categoryLabel'
import AnnouncementBoard from '../components/AnnouncementBoard'
import { Alert } from '../components/ui/alert'
import { Badge } from '../components/ui/badge'
import { ButtonLink } from '../components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../components/ui/card'
import { EmptyState } from '../components/ui/empty-state'
import { PageHeader } from '../components/ui/page-header'
import { ProgressBar } from '../components/ui/progress-bar'
import { Segmented, SegmentedButton } from '../components/ui/segmented'
import { CardSkeleton } from '../components/ui/skeleton'
import { StatTile } from '../components/ui/stat-tile'
import { cn } from '../lib/utils'

const FEATURES = [
  {
    to: '/transactions',
    Icon: List,
    titleKey: 'dashboard.features.smartAccounting.title',
    descKey: 'dashboard.features.smartAccounting.desc',
    lifeTitleKey: 'dashboard.features.smartAccounting.title',
    lifeDescKey: 'dashboard.features.smartAccounting.desc',
  },
  {
    to: '/add',
    Icon: Plus,
    titleKey: 'dashboard.features.reimbursement.title',
    descKey: 'dashboard.features.reimbursement.desc',
    lifeTitleKey: 'dashboard.features.lifeEntry.title',
    lifeDescKey: 'dashboard.features.lifeEntry.desc',
  },
  {
    to: '/match',
    Icon: SearchCheck,
    titleKey: 'dashboard.features.smartMatch.title',
    descKey: 'dashboard.features.smartMatch.desc',
    lifeTitleKey: 'dashboard.features.lifeMatch.title',
    lifeDescKey: 'dashboard.features.lifeMatch.desc',
  },
  {
    to: '/stats',
    Icon: BarChart3,
    titleKey: 'dashboard.features.dataVisualization.title',
    descKey: 'dashboard.features.dataVisualization.desc',
    lifeTitleKey: 'dashboard.features.dataVisualization.title',
    lifeDescKey: 'dashboard.features.dataVisualization.desc',
  },
] satisfies Array<{
  to: string
  Icon: LucideIcon
  titleKey: string
  descKey: string
  lifeTitleKey: string
  lifeDescKey: string
}>

function getGreetingKey() {
  const hour = new Date().getHours()
  if (hour >= 1 && hour < 5) return 'dawn'
  if (hour >= 5 && hour < 8) return 'earlyMorning'
  if (hour >= 8 && hour < 11) return 'morning'
  if (hour >= 11 && hour < 12) return 'beforeNoon'
  if (hour >= 12 && hour < 14) return 'lunch'
  if (hour >= 14 && hour < 18) return 'afternoon'
  if (hour >= 18 && hour < 21) return 'evening'
  return 'night'
}

export default function DashboardPage() {
  const { user } = useAuth()
  const { rates, rateDate, loading: ratesLoading } = useExchangeRates()
  const { t, i18n } = useTranslation()
  const { isWorkMode } = useMode()
  const [workWorkflowTab, setWorkWorkflowTab] = useState<'company' | 'personal'>('company')
  const workflowTab = isWorkMode ? workWorkflowTab : 'personal'
  const [analysisNow] = useState(Date.now)

  useHeartbeat()
  const { data: onlineDeviceCount } = useOnlineDevices()

  const [greetingKey] = useState(getGreetingKey)
  const greetingMessages = t(`dashboard.greeting.${greetingKey}`, { returnObjects: true }) as string[]
  const [greetingIdx] = useState(() => {
    if (greetingMessages.length <= 1) return 0
    return secureRandomInt(greetingMessages.length)
  })
  const rawGreetingText = greetingMessages[greetingIdx] || greetingMessages[0] || ''
  const username = user?.nickname || user?.username || user?.email?.split('@')[0] || ''
  const greetingText = formatGreeting({
    locale: normalizeGreetingLocale(i18n.language),
    greeting: rawGreetingText,
    message: t('dashboard.greetingPrompt'),
    username,
  })

  const { data: transactions = [], isLoading: loading, error: txError } = useTransactions()
  const { data: accounts = [] } = useAccounts()
  const budgetMonth = currentBudgetMonth()
  const { data: budgetSummary } = useBudgetSummary(budgetMonth)
  const { data: recurringRules = [] } = useRecurringRules()
  const sourceFilter: 'personal' | 'company' = isWorkMode ? 'company' : 'personal'
  const error = txError
    ? ((txError as { response?: { data?: { message?: string } } })?.response?.data?.message ?? t('common.error'))
    : ''

  const companyBalance = useMemo(() =>
    accounts
      .filter((account) => account.type === 'public' && account.is_active)
      .reduce((sum, account) => sum + accountBalanceToCNY(account, rates), 0),
  [accounts, rates])

  const personalBalance = useMemo(() =>
    accounts
      .filter((account) => account.type === 'personal' && account.is_active)
      .reduce((sum, account) => sum + accountBalanceToCNY(account, rates), 0),
  [accounts, rates])

  const personalTotalExpense = useMemo(() =>
    transactions
      .filter((transaction) => transaction.source === 'personal' && transaction.direction === 'expense')
      .reduce((sum, transaction) => sum + transactionAmountToCNY(transaction, rates), 0),
  [transactions, rates])

  const personalOutstanding = useMemo(() =>
    transactions
      .filter(t => t.source === 'personal' && t.direction === 'expense' && !t.reimbursed)
      .reduce((sum, transaction) => sum + transactionAmountToCNY(transaction, rates), 0),
  [transactions, rates])

  const fmtExact = (value: number) => formatAmountExact(value, 'CNY')
  const fmtCompact = (value: number) => formatAmountCompact(value, 'CNY')

  const monthInsights = useMemo(() => {
    const monthly = transactions.filter(
      (transaction) => transaction.source === sourceFilter && transaction.occurred_at.startsWith(budgetMonth),
    )
    const byCategory = new Map<string, number>()
    let income = 0
    let expense = 0

    for (const transaction of monthly) {
      const amount = transactionAmountToCNY(transaction, rates)
      if (transaction.direction === 'income') {
        income += amount
      } else {
        expense += amount
        const key = transaction.category || t('categories.other')
        byCategory.set(key, (byCategory.get(key) ?? 0) + amount)
      }
    }

    const topCategory = Array.from(byCategory.entries()).sort((a, b) => b[1] - a[1])[0]
    return {
      income,
      expense,
      net: income - expense,
      topCategory: topCategory ? { category: topCategory[0], amount: topCategory[1] } : null,
    }
  }, [budgetMonth, rates, sourceFilter, t, transactions])

  const pendingTxs = useMemo(
    () => isWorkMode
      ? transactions.filter(t => t.source === 'personal' && t.direction === 'expense' && !t.reimbursed)
      : [],
    [isWorkMode, transactions],
  )
  const notUploaded = useMemo(() => pendingTxs.filter((transaction) => !transaction.uploaded), [pendingTxs])
  const uploadedNotReimbursed = useMemo(
    () => pendingTxs.filter((transaction) => transaction.uploaded && !transaction.reimbursed),
    [pendingTxs],
  )
  const hasPending = isWorkMode && pendingTxs.length > 0
  const allClear = isWorkMode && !loading && !hasPending && transactions.length > 0

  const pendingAnalysis = useMemo(() => {
    const now = analysisNow
    const day = 86_400_000
    const notUploadedAmount = notUploaded.reduce(
      (sum, transaction) => sum + transactionAmountToCNY(transaction, rates),
      0,
    )
    const uploadedNotReimbursedAmount = uploadedNotReimbursed.reduce(
      (sum, transaction) => sum + transactionAmountToCNY(transaction, rates),
      0,
    )
    const oldestDate = (items: typeof transactions) => {
      if (items.length === 0) return null
      const dates = items
        .map((transaction) => new Date(transaction.occurred_at).getTime())
        .filter((date) => !Number.isNaN(date))
      return dates.length > 0 ? Math.min(...dates) : null
    }
    const oldestNotUploaded = oldestDate(notUploaded)
    const oldestUploaded = oldestDate(uploadedNotReimbursed)
    const daysSince = (timestamp: number | null) => timestamp ? Math.floor((now - timestamp) / day) : 0

    const notUploadedSub = (() => {
      if (notUploaded.length === 0) return ''
      const days = daysSince(oldestNotUploaded)
      const amount = fmtExact(notUploadedAmount)
      if (days > 30) return t('dashboard.pending.notUploaded.over30d', { amt: amount, days })
      if (days > 14) return t('dashboard.pending.notUploaded.over14d', { amt: amount })
      if (days > 7) return t('dashboard.pending.notUploaded.over7d', { amt: amount })
      if (notUploaded.length >= 10) return t('dashboard.pending.notUploaded.manyItems', { amt: amount })
      if (notUploadedAmount >= 5000) return t('dashboard.pending.notUploaded.highAmount', { amt: amount })
      return t('dashboard.pending.notUploaded.default', { amt: amount })
    })()

    const uploadedNotReimbursedSub = (() => {
      if (uploadedNotReimbursed.length === 0) return ''
      const days = daysSince(oldestUploaded)
      const amount = fmtExact(uploadedNotReimbursedAmount)
      if (days > 60) return t('dashboard.pending.uploadedPending.over60d', { amt: amount, days })
      if (days > 30) return t('dashboard.pending.uploadedPending.over30d', { amt: amount })
      if (days > 14) return t('dashboard.pending.uploadedPending.over14d', { amt: amount })
      if (uploadedNotReimbursed.length >= 5) {
        return t('dashboard.pending.uploadedPending.manyItems', {
          amt: amount,
          count: uploadedNotReimbursed.length,
        })
      }
      return t('dashboard.pending.uploadedPending.default', { amt: amount })
    })()

    const maxDays = Math.max(daysSince(oldestNotUploaded), daysSince(oldestUploaded))
    const totalPending = notUploaded.length + uploadedNotReimbursed.length
    const totalAmount = notUploadedAmount + uploadedNotReimbursedAmount
    let headerHint = ''
    if (maxDays > 30) headerHint = t('dashboard.pending.header.overdue', { days: maxDays })
    else if (totalAmount >= 10_000) {
      headerHint = t('dashboard.pending.header.highAmount', {
        count: totalPending,
        amt: fmtExact(totalAmount),
      })
    } else if (totalPending >= 8) headerHint = t('dashboard.pending.header.manyItems', { count: totalPending })
    else if (totalPending > 0) headerHint = t('dashboard.pending.header.default', { count: totalPending })

    return { notUploadedSub, uploadedNotReimbursedSub, headerHint }
  }, [analysisNow, notUploaded, uploadedNotReimbursed, rates, t])

  if (loading) {
    return (
      <div className="space-y-6 animate-fade-in" aria-busy="true">
        <CardSkeleton className="h-24" />
        <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
          <CardSkeleton />
          <CardSkeleton />
          <CardSkeleton />
          <CardSkeleton />
        </div>
        <div className="grid gap-4 lg:grid-cols-2">
          <CardSkeleton className="h-40" />
          <CardSkeleton className="h-40" />
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <Alert variant="negative" className="block p-5">
        <p className="font-semibold">{t('common.error')}</p>
        <p className="mt-1 text-sm opacity-85">{error}</p>
      </Alert>
    )
  }

  const dateLocale = (i18n.resolvedLanguage ?? i18n.language).toLowerCase().startsWith('zh') ? 'zh-CN' : 'en-US'
  const featureCards = FEATURES.map((feature) => ({
    ...feature,
    titleKey: isWorkMode ? feature.titleKey : feature.lifeTitleKey,
    descKey: isWorkMode ? feature.descKey : feature.lifeDescKey,
  }))
  const nextRecurringRule = recurringRules
    .filter((rule) => rule.status === 'active')
    .sort((first, second) => (first.next_run_at || 0) - (second.next_run_at || 0))[0]
  const activeRecurringCount = recurringRules.filter((rule) => rule.status === 'active').length
  const budgetTone = budgetSummary?.total_budget?.status === 'over'
    ? 'negative'
    : budgetSummary?.total_budget?.status === 'warning'
      ? 'warning'
      : 'positive'

  const personalWorkflowSteps = [
    {
      step: '1',
      Icon: PenLine,
      titleKey: isWorkMode ? 'dashboard.workflow.personalStep1' : 'dashboard.workflow.personalLifeStep1',
      descKey: isWorkMode ? 'dashboard.workflow.personalDesc1' : 'dashboard.workflow.personalLifeDesc1',
    },
    {
      step: '2',
      Icon: Upload,
      titleKey: isWorkMode ? 'dashboard.workflow.personalStep2' : 'dashboard.workflow.personalLifeStep2',
      descKey: isWorkMode ? 'dashboard.workflow.personalDesc2' : 'dashboard.workflow.personalLifeDesc2',
    },
    {
      step: '3',
      Icon: SearchCheck,
      titleKey: isWorkMode ? 'dashboard.workflow.personalStep3' : 'dashboard.workflow.personalLifeStep3',
      descKey: isWorkMode ? 'dashboard.workflow.personalDesc3' : 'dashboard.workflow.personalLifeDesc3',
    },
    {
      step: '4',
      Icon: Check,
      titleKey: isWorkMode ? 'dashboard.workflow.personalStep4' : 'dashboard.workflow.personalLifeStep4',
      descKey: isWorkMode ? 'dashboard.workflow.personalDesc4' : 'dashboard.workflow.personalLifeDesc4',
    },
  ]
  const companyWorkflowSteps = [
    { step: '1', Icon: PenLine, titleKey: 'dashboard.workflow.companyStep1', descKey: 'dashboard.workflow.companyDesc1' },
    { step: '2', Icon: Upload, titleKey: 'dashboard.workflow.companyStep2', descKey: 'dashboard.workflow.companyDesc2' },
    { step: '3', Icon: Check, titleKey: 'dashboard.workflow.companyStep3', descKey: 'dashboard.workflow.companyDesc3' },
  ]
  const workflowSteps = workflowTab === 'company' ? companyWorkflowSteps : personalWorkflowSteps

  return (
    <div className="space-y-6">
      <PageHeader
        title={greetingText}
        description={new Date().toLocaleDateString(dateLocale, {
          year: 'numeric',
          month: 'long',
          day: 'numeric',
        })}
        actions={(
          <ButtonLink to="/add">
            <Plus className="size-4" />
            {t('dashboard.addButton')}
          </ButtonLink>
        )}
        meta={(
          <>
            {onlineDeviceCount != null ? (
              <Badge variant="positive" dot>
                {t('common.devices', { count: onlineDeviceCount })}
              </Badge>
            ) : null}
            {ratesLoading ? (
              <Badge>{t('dashboard.hero.exRateLoading')}</Badge>
            ) : rateDate ? (
              <Badge variant="accent">
                <Globe2 className="size-3" />
                $ {rates.USD?.toFixed(2)} · € {rates.EUR?.toFixed(2)} · {rateDate}
              </Badge>
            ) : (
              <Badge variant="warning">
                {t('dashboard.hero.exRateFallback')} · $ {rates.USD?.toFixed(2)} · € {rates.EUR?.toFixed(2)}
              </Badge>
            )}
          </>
        )}
      />

      <AnnouncementBoard />

      <section aria-label={t('dashboard.balance.balanceLabel')} className="grid grid-cols-2 gap-3">
        {isWorkMode ? (
          <>
            <StatTile
              label={t('dashboard.balance.public')}
              value={<CompactAmount compact={fmtCompact(companyBalance)} exact={fmtExact(companyBalance)} />}
              hint={t('dashboard.balance.balanceLabel')}
              icon={<WalletCards />}
            />
            <StatTile
              label={t('dashboard.balance.personalPending')}
              value={<CompactAmount compact={fmtCompact(personalOutstanding)} exact={fmtExact(personalOutstanding)} />}
              hint={t('dashboard.balance.pendingLabel')}
              icon={<Upload />}
              tone="warning"
            />
          </>
        ) : (
          <>
            <StatTile
              label={t('dashboard.balance.personalAdvance')}
              value={<CompactAmount compact={fmtCompact(personalBalance)} exact={fmtExact(personalBalance)} />}
              hint={t('dashboard.balance.balanceLabel')}
              icon={<Sparkles />}
              tone="positive"
            />
            <StatTile
              label={t('transactions.summary.expense')}
              value={<CompactAmount compact={fmtCompact(personalTotalExpense)} exact={fmtExact(personalTotalExpense)} />}
              hint={t('dashboard.balance.lifeExpenseLabel')}
              icon={<BarChart3 />}
              tone="negative"
            />
          </>
        )}
      </section>

      <section aria-labelledby="dashboard-insights-title" className="space-y-3">
        <div>
          <h2 id="dashboard-insights-title" className="text-sm font-semibold text-foreground">
            {t('dashboard.insights.title')}
          </h2>
          <p className="mt-0.5 text-xs text-muted-foreground">{t('dashboard.insights.subtitle')}</p>
        </div>
        <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
          <StatTile
            label={t('dashboard.insights.income')}
            value={<CompactAmount compact={fmtCompact(monthInsights.income)} exact={fmtExact(monthInsights.income)} />}
            tone="positive"
          />
          <StatTile
            label={t('dashboard.insights.expense')}
            value={<CompactAmount compact={fmtCompact(monthInsights.expense)} exact={fmtExact(monthInsights.expense)} />}
            tone="negative"
          />
          <StatTile
            label={t('dashboard.insights.net')}
            value={(
              <CompactAmount
                compact={fmtCompact(monthInsights.net)}
                exact={fmtExact(monthInsights.net)}
                prefix={monthInsights.net >= 0 ? '+' : ''}
              />
            )}
            tone={monthInsights.net >= 0 ? 'positive' : 'negative'}
          />
          <StatTile
            label={t('dashboard.insights.topCategory')}
            value={monthInsights.topCategory
              ? categoryLabel(monthInsights.topCategory.category)
              : t('dashboard.insights.noCategory')}
            hint={monthInsights.topCategory ? fmtCompact(monthInsights.topCategory.amount) : undefined}
          />
        </div>
      </section>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <div>
              <CardTitle>{t('dashboard.budgetCard.title')}</CardTitle>
              <CardDescription>{t('dashboard.budgetCard.subtitle')}</CardDescription>
            </div>
            <ButtonLink to="/budgets" variant="ghost" size="sm">
              {t('dashboard.budgetCard.action')}
              <ArrowRight className="size-3.5" />
            </ButtonLink>
          </CardHeader>
          <CardContent>
            {budgetSummary?.total_budget ? (
              <div className="space-y-3">
                <div className="flex items-end justify-between gap-4">
                  <div>
                    <p className="text-xs font-medium text-muted-foreground">{t('budgets.actual')}</p>
                    <p className="mt-1 text-xl font-semibold tabular-nums text-foreground">
                      {fmtCompact(budgetSummary.total_budget.actual_yuan)}
                    </p>
                  </div>
                  <div className="text-right">
                    <p className="text-xs font-medium text-muted-foreground">{t('budgets.planned')}</p>
                    <p className="mt-1 text-sm font-semibold tabular-nums text-foreground">
                      {fmtCompact(budgetSummary.total_budget.budget.base_amount_yuan)}
                    </p>
                  </div>
                </div>
                <ProgressBar
                  label={t('dashboard.budgetCard.title')}
                  value={budgetSummary.total_budget.usage_ratio * 100}
                  tone={budgetTone}
                />
                <div className="flex justify-between gap-3 text-xs text-muted-foreground">
                  <span>{Math.round(budgetSummary.total_budget.usage_ratio * 100)}%</span>
                  <span className="text-right">
                    {budgetSummary.total_budget.remaining_yuan < 0 ? t('budgets.overBy') : t('budgets.remaining')}:{' '}
                    {fmtCompact(Math.abs(budgetSummary.total_budget.remaining_yuan))}
                  </span>
                </div>
              </div>
            ) : (
              <EmptyState
                className="bg-background py-7"
                title={t('dashboard.budgetCard.emptyTitle')}
                description={t('dashboard.budgetCard.emptyDesc')}
              />
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <div>
              <CardTitle>{t('dashboard.recurringCard.title')}</CardTitle>
              <CardDescription>{t('dashboard.recurringCard.subtitle')}</CardDescription>
            </div>
            <ButtonLink to="/recurring" variant="ghost" size="sm">
              {t('dashboard.recurringCard.action')}
              <ArrowRight className="size-3.5" />
            </ButtonLink>
          </CardHeader>
          <CardContent>
            {recurringRules.length > 0 ? (
              <div className="rounded-lg border border-border bg-background p-4">
                <div className="flex items-start gap-3">
                  <span className="grid size-8 shrink-0 place-items-center rounded-md bg-mode-soft text-mode">
                    <Repeat2 className="size-4" />
                  </span>
                  <div className="min-w-0">
                    <p className="text-sm leading-relaxed text-foreground">
                      {nextRecurringRule
                        ? t('dashboard.recurringCard.next', {
                          name: nextRecurringRule.name,
                          time: nextRecurringRule.next_occurred_at,
                        })
                        : t('dashboard.recurringCard.desc')}
                    </p>
                    <p className="mt-2 text-xs font-medium text-muted-foreground">
                      {t('dashboard.recurringCard.activeCount', { count: activeRecurringCount })}
                    </p>
                  </div>
                </div>
              </div>
            ) : (
              <EmptyState
                className="bg-background py-7"
                title={t('dashboard.recurringCard.emptyTitle')}
                description={t('dashboard.recurringCard.emptyDesc')}
                action={(
                  <ButtonLink to="/recurring" variant="outline" size="sm">
                    {t('dashboard.recurringCard.action')}
                  </ButtonLink>
                )}
              />
            )}
          </CardContent>
        </Card>
      </div>

      {hasPending ? (
        <Card>
          <CardHeader>
            <div>
              <CardTitle>{t('dashboard.pending.title')}</CardTitle>
              {pendingAnalysis.headerHint ? (
                <CardDescription>{pendingAnalysis.headerHint}</CardDescription>
              ) : null}
            </div>
            <Badge variant="warning">{pendingTxs.length}</Badge>
          </CardHeader>
          <CardContent className="space-y-2">
            {notUploaded.length > 0 ? (
              <Link
                to="/transactions?source=personal"
                className="group flex items-center gap-3 rounded-lg border border-warning/25 bg-warning-soft p-3 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <span className="grid size-8 shrink-0 place-items-center rounded-md bg-card text-warning ring-1 ring-warning/20">
                  <Upload className="size-4" />
                </span>
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-medium text-foreground">
                    {notUploaded.length} {t('transactions.badges.notUploaded')}
                  </p>
                  <p className="mt-0.5 text-xs text-muted-foreground">{pendingAnalysis.notUploadedSub}</p>
                </div>
                <ArrowRight className="size-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5" />
              </Link>
            ) : null}
            {uploadedNotReimbursed.length > 0 ? (
              <Link
                to="/match"
                className="group flex items-center gap-3 rounded-lg border border-accent/25 bg-accent-soft p-3 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <span className="grid size-8 shrink-0 place-items-center rounded-md bg-card text-accent ring-1 ring-accent/20">
                  <SearchCheck className="size-4" />
                </span>
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-medium text-foreground">
                    {uploadedNotReimbursed.length} {t('transactions.badges.pending')}
                  </p>
                  <p className="mt-0.5 text-xs text-muted-foreground">
                    {pendingAnalysis.uploadedNotReimbursedSub}
                  </p>
                </div>
                <ArrowRight className="size-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5" />
              </Link>
            ) : null}
          </CardContent>
        </Card>
      ) : null}

      {allClear ? (
        <Alert variant="positive" className="items-start p-4">
          <CircleCheck className="mt-0.5 size-4 shrink-0" />
          <div>
            <p className="font-medium">{t('dashboard.pending.noPending')}</p>
            <p className="mt-0.5 text-xs opacity-80">{t('dashboard.pending.tip')}</p>
          </div>
        </Alert>
      ) : null}

      <Card>
        <CardHeader>
          <div>
            <CardTitle>{t('dashboard.featureNavTitle')}</CardTitle>
            <CardDescription>{t('dashboard.workflowTitle')}</CardDescription>
          </div>
          {isWorkMode ? (
            <Segmented aria-label={t('dashboard.workflowTitle')}>
              {(['company', 'personal'] as const).map((source) => (
                <SegmentedButton
                  key={source}
                  onClick={() => setWorkWorkflowTab(source)}
                  aria-pressed={workflowTab === source}
                  className="max-w-36 truncate"
                >
                  {source === 'company'
                    ? t('dashboard.workflow.companyTitle')
                    : t('dashboard.workflow.personalTitle')}
                </SegmentedButton>
              ))}
            </Segmented>
          ) : null}
        </CardHeader>

        <CardContent className="grid gap-6 xl:grid-cols-[minmax(0,1.2fr)_minmax(18rem,0.8fr)]">
          <nav className="grid gap-2 sm:grid-cols-2" aria-label={t('dashboard.featureNavTitle')}>
            {featureCards.map((feature) => (
              <Link
                key={feature.to}
                to={feature.to}
                className="group flex min-h-24 items-start gap-3 rounded-lg border border-border bg-background p-3 transition-colors hover:border-input hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <span className="grid size-8 shrink-0 place-items-center rounded-md bg-card text-mode ring-1 ring-border">
                  <feature.Icon className="size-4" />
                </span>
                <div className="min-w-0 flex-1">
                  <div className="flex items-center justify-between gap-2">
                    <p className="text-sm font-semibold text-foreground">{t(feature.titleKey)}</p>
                    <ArrowRight className="size-3.5 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5" />
                  </div>
                  <p className="mt-1 text-xs leading-relaxed text-muted-foreground">{t(feature.descKey)}</p>
                </div>
              </Link>
            ))}
          </nav>

          <ol className="space-y-0" aria-label={t('dashboard.workflowTitle')}>
            {workflowSteps.map((step, index) => (
              <li key={step.step} className="flex gap-3">
                <div className="flex flex-col items-center">
                  <span className="grid size-8 shrink-0 place-items-center rounded-md bg-mode-soft text-mode">
                    <step.Icon className="size-4" />
                  </span>
                  {index < workflowSteps.length - 1 ? (
                    <span className="my-1 min-h-4 w-px flex-1 bg-border" aria-hidden="true" />
                  ) : null}
                </div>
                <div className={cn('min-w-0 pb-4', index === workflowSteps.length - 1 && 'pb-0')}>
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-[10px] font-semibold text-muted-foreground">{step.step.padStart(2, '0')}</span>
                    <p className="text-sm font-semibold text-foreground">{t(step.titleKey)}</p>
                  </div>
                  <p className="mt-0.5 text-xs leading-relaxed text-muted-foreground">{t(step.descKey)}</p>
                </div>
              </li>
            ))}
          </ol>
        </CardContent>
      </Card>
    </div>
  )
}
