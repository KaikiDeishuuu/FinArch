import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
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
import { StaggerContainer, StaggerItem, AnimatedCard, CardSkeleton } from '../motion'
import { useMode } from '../hooks/useMode'
import { useBudgetSummary, currentBudgetMonth } from '../hooks/useBudgets'
import { useRecurringRules } from '../hooks/useRecurringRules'
import { EmptyState, FinanceCard, ProgressBar, SectionHeader } from '../components/FinancePrimitives'
import { categoryLabel } from '../utils/categoryLabel'

const IconList = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-5 h-5">
    <path d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01" />
  </svg>
)
const IconPlus = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" className="w-5 h-5">
    <path d="M12 5v14M5 12h14" />
  </svg>
)
const IconSearch = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-5 h-5">
    <circle cx="11" cy="11" r="8" /><path d="M21 21l-4.35-4.35" />
  </svg>
)
const IconChart = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-5 h-5">
    <path d="M18 20V10M12 20V4M6 20v-6" />
  </svg>
)
const IconUpload = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-5 h-5">
    <path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4" />
    <polyline points="17 8 12 3 7 8" />
    <line x1="12" y1="3" x2="12" y2="15" />
  </svg>
)
const IconPen = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-5 h-5">
    <path d="M11 4H4a2 2 0 00-2 2v14a2 2 0 002 2h14a2 2 0 002-2v-7" />
    <path d="M18.5 2.5a2.121 2.121 0 013 3L12 15l-4 1 1-4 9.5-9.5z" />
  </svg>
)
const IconCheck = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.5} strokeLinecap="round" strokeLinejoin="round" className="w-5 h-5">
    <polyline points="20 6 9 17 4 12" />
  </svg>
)

const IconSparkles = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-5 h-5">
    <path d="M12 3l1.8 4.2L18 9l-4.2 1.8L12 15l-1.8-4.2L6 9l4.2-1.8L12 3z" />
    <path d="M5 16l.9 2.1L8 19l-2.1.9L5 22l-.9-2.1L2 19l2.1-.9L5 16z" />
    <path d="M19 14l.9 2.1L22 17l-2.1.9L19 20l-.9-2.1L16 17l2.1-.9L19 14z" />
  </svg>
)

const FEATURES = [
  {
    to: '/transactions',
    Icon: IconList,
    titleKey: 'dashboard.features.smartAccounting.title',
    descKey: 'dashboard.features.smartAccounting.desc',
    lifeTitleKey: 'dashboard.features.smartAccounting.title',
    lifeDescKey: 'dashboard.features.smartAccounting.desc',
  },
  {
    to: '/add',
    Icon: IconPlus,
    titleKey: 'dashboard.features.reimbursement.title',
    descKey: 'dashboard.features.reimbursement.desc',
    lifeTitleKey: 'dashboard.features.lifeEntry.title',
    lifeDescKey: 'dashboard.features.lifeEntry.desc',
  },
  {
    to: '/match',
    Icon: IconSearch,
    titleKey: 'dashboard.features.smartMatch.title',
    descKey: 'dashboard.features.smartMatch.desc',
    lifeTitleKey: 'dashboard.features.lifeMatch.title',
    lifeDescKey: 'dashboard.features.lifeMatch.desc',
  },
  {
    to: '/stats',
    Icon: IconChart,
    titleKey: 'dashboard.features.dataVisualization.title',
    descKey: 'dashboard.features.dataVisualization.desc',
    lifeTitleKey: 'dashboard.features.dataVisualization.title',
    lifeDescKey: 'dashboard.features.dataVisualization.desc',
  },
]

// ─── Time-based greeting key selector ─────────────────────────────────────
function getGreetingKey() {
  const hour = new Date().getHours()
  if (hour >= 1 && hour < 5) return 'dawn'
  if (hour >= 5 && hour < 8) return 'earlyMorning'
  if (hour >= 8 && hour < 11) return 'morning'
  if (hour >= 11 && hour < 12) return 'beforeNoon'
  if (hour >= 12 && hour < 14) return 'lunch'
  if (hour >= 14 && hour < 18) return 'afternoon'
  if (hour >= 18 && hour < 21) return 'evening'
  return 'night' // 21-0, 0-1
}

export default function DashboardPage() {
  const { user } = useAuth()
  const { rates, rateDate, loading: ratesLoading } = useExchangeRates()
  const { t, i18n } = useTranslation()
  const { isWorkMode } = useMode()
  const [workWorkflowTab, setWorkWorkflowTab] = useState<'company' | 'personal'>('company')
  const workflowTab = isWorkMode ? workWorkflowTab : 'personal'
  const [analysisNow] = useState(Date.now)

  // Device heartbeat — keeps this device marked as online
  useHeartbeat()
  const { data: onlineDeviceCount } = useOnlineDevices()

  // Greeting — generated once per mount, pick random from i18n array
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

  // Use server-side cached account balances
  const companyBalance = useMemo(() =>
    accounts
      .filter(a => a.type === 'public' && a.is_active)
      .reduce((sum, account) => sum + accountBalanceToCNY(account, rates), 0),
    [accounts, rates]
  )

  const personalBalance = useMemo(() =>
    accounts
      .filter(a => a.type === 'personal' && a.is_active)
      .reduce((sum, account) => sum + accountBalanceToCNY(account, rates), 0),
    [accounts, rates]
  )

  const personalTotalExpense = useMemo(() =>
    transactions
      .filter(t => t.source === 'personal' && t.direction === 'expense')
      .reduce((sum, transaction) => sum + transactionAmountToCNY(transaction, rates), 0),
    [transactions, rates]
  )


  const personalOutstanding = useMemo(() =>
    transactions
      .filter(t => t.source === 'personal' && t.direction === 'expense' && !t.reimbursed)
      .reduce((sum, transaction) => sum + transactionAmountToCNY(transaction, rates), 0),
    [transactions, rates]
  )

  const fmtExact = (n: number) => formatAmountExact(n, 'CNY')
  const fmtCompact = (n: number) => formatAmountCompact(n, 'CNY')

  const monthInsights = useMemo(() => {
    const monthPrefix = budgetMonth
    const monthly = transactions.filter(t => t.source === sourceFilter && t.occurred_at.startsWith(monthPrefix))
    const byCategory = new Map<string, number>()
    let income = 0
    let expense = 0
    for (const tx of monthly) {
      const amount = transactionAmountToCNY(tx, rates)
      if (tx.direction === 'income') {
        income += amount
      } else {
        expense += amount
        const key = tx.category || t('categories.other')
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
    [isWorkMode, transactions]
  )
  const notUploaded = useMemo(() => pendingTxs.filter((t) => !t.uploaded), [pendingTxs])
  const uploadedNotReimbursed = useMemo(() => pendingTxs.filter((t) => t.uploaded && !t.reimbursed), [pendingTxs])

  // ─── Smart pending item analysis ───────────────────────────────────────────
  const hasPending = isWorkMode && pendingTxs.length > 0
  const allClear = isWorkMode && !loading && !hasPending && transactions.length > 0

  const pendingAnalysis = useMemo(() => {
    const now = analysisNow
    const DAY = 86400000

    const notUploadedAmount = notUploaded.reduce((sum, transaction) => sum + transactionAmountToCNY(transaction, rates), 0)
    const uploadedNotReimbursedAmount = uploadedNotReimbursed.reduce((sum, transaction) => sum + transactionAmountToCNY(transaction, rates), 0)
    const oldestDate = (txs: typeof transactions) => {
      if (txs.length === 0) return null
      const dates = txs.map(t => new Date(t.occurred_at).getTime()).filter(d => !isNaN(d))
      return dates.length > 0 ? Math.min(...dates) : null
    }

    const oldestNotUploaded = oldestDate(notUploaded)
    const oldestUploaded = oldestDate(uploadedNotReimbursed)
    const daysSince = (ts: number | null) => ts ? Math.floor((now - ts) / DAY) : 0

    // Smart sub-messages: notUploaded
    const notUploadedSub = (() => {
      if (notUploaded.length === 0) return ''
      const days = daysSince(oldestNotUploaded)
      const amt = fmtExact(notUploadedAmount)
      if (days > 30) return t('dashboard.pending.notUploaded.over30d', { amt, days })
      if (days > 14) return t('dashboard.pending.notUploaded.over14d', { amt })
      if (days > 7) return t('dashboard.pending.notUploaded.over7d', { amt })
      if (notUploaded.length >= 10) return t('dashboard.pending.notUploaded.manyItems', { amt })
      if (notUploadedAmount >= 5000) return t('dashboard.pending.notUploaded.highAmount', { amt })
      return t('dashboard.pending.notUploaded.default', { amt })
    })()

    // Smart sub-messages: uploadedNotReimbursed
    const uploadedNotReimbursedSub = (() => {
      if (uploadedNotReimbursed.length === 0) return ''
      const days = daysSince(oldestUploaded)
      const amt = fmtExact(uploadedNotReimbursedAmount)
      if (days > 60) return t('dashboard.pending.uploadedPending.over60d', { amt, days })
      if (days > 30) return t('dashboard.pending.uploadedPending.over30d', { amt })
      if (days > 14) return t('dashboard.pending.uploadedPending.over14d', { amt })
      if (uploadedNotReimbursed.length >= 5) return t('dashboard.pending.uploadedPending.manyItems', { amt, count: uploadedNotReimbursed.length })
      return t('dashboard.pending.uploadedPending.default', { amt })
    })()

    // Urgency header
    const maxDays = Math.max(daysSince(oldestNotUploaded), daysSince(oldestUploaded))
    const totalPending = notUploaded.length + uploadedNotReimbursed.length
    const totalAmount = notUploadedAmount + uploadedNotReimbursedAmount

    let headerHint = ''
    if (maxDays > 30) headerHint = t('dashboard.pending.header.overdue', { days: maxDays })
    else if (totalAmount >= 10000) headerHint = t('dashboard.pending.header.highAmount', { count: totalPending, amt: fmtExact(totalAmount) })
    else if (totalPending >= 8) headerHint = t('dashboard.pending.header.manyItems', { count: totalPending })
    else if (totalPending > 0) headerHint = t('dashboard.pending.header.default', { count: totalPending })

    return { notUploadedSub, uploadedNotReimbursedSub, headerHint }
  }, [analysisNow, notUploaded, uploadedNotReimbursed, rates, t])

  if (loading) {
    return (
      <div className="space-y-6 animate-fade-in">
        <CardSkeleton className="h-28" />
        <div className="grid grid-cols-2 gap-3">
          <CardSkeleton /><CardSkeleton /><CardSkeleton /><CardSkeleton />
        </div>
        <CardSkeleton className="h-24" />
        <CardSkeleton className="h-40" />
      </div>
    )
  }

  if (error) {
    return (
      <div className="bg-rose-50 dark:bg-rose-500/10 border border-rose-200 dark:border-rose-500/30 text-rose-700 dark:text-rose-300 rounded-xl p-6 text-sm">
        <p className="font-semibold mb-1">{t('common.error')}</p>
        <p>{error}</p>
      </div>
    )
  }

  const dateLocale = i18n.language === 'zh' ? 'zh-CN' : 'en-US'
  const featureCards = FEATURES.map((feature) => ({
    ...feature,
    titleKey: isWorkMode ? feature.titleKey : feature.lifeTitleKey,
    descKey: isWorkMode ? feature.descKey : feature.lifeDescKey,
  }))
  const nextRecurringRule = recurringRules
    .filter(rule => rule.status === 'active')
    .sort((a, b) => (a.next_run_at || 0) - (b.next_run_at || 0))[0]

  return (
    <div className="space-y-7 md:space-y-8">
      {/* Ledger header */}
      <div className="ledger-rail relative border-y border-[hsl(var(--border))] bg-[hsl(var(--card))]/45 py-5 pl-5 pr-3 sm:py-6 sm:pl-7 sm:pr-5">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <p className="page-kicker mb-2">FINARCH / OVERVIEW</p>
            <div className="mb-1 flex flex-wrap items-center gap-2">
              <p className="font-data text-xs font-medium text-[hsl(var(--muted-foreground))]">{new Date().toLocaleDateString(dateLocale, { year: 'numeric', month: 'long', day: 'numeric' })}</p>
              {onlineDeviceCount != null && (
                <span className="inline-flex items-center gap-1.5 border-l border-[hsl(var(--border))] pl-2 font-data text-[10px] font-medium text-[hsl(var(--muted-foreground))]">
                  <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" />
                  {t('common.devices', { count: onlineDeviceCount })}
                </span>
              )}
            </div>
            <h1 className="font-display text-2xl font-semibold tracking-[-0.025em] text-[hsl(var(--foreground))] md:text-3xl">{greetingText}</h1>
            <div className="mt-3 flex flex-wrap items-center gap-2">
              {ratesLoading
                ? <span className="border border-[hsl(var(--border))] bg-[hsl(var(--muted))]/70 px-2 py-1 font-data text-[10px] text-[hsl(var(--muted-foreground))]">{t('dashboard.hero.exRateLoading')}</span>
                : rateDate
                  ? <span className="inline-flex items-center gap-1 border border-[hsl(var(--border))] bg-[hsl(var(--muted))]/70 px-2 py-1 font-data text-[10px] font-medium text-[hsl(var(--muted-foreground))]"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} className="h-3 w-3 text-[hsl(var(--mode-accent))]"><path d="M4 7h16M4 17h16M10 4c-2 2-2 14 0 16M14 4c2 2 2 14 0 16" /></svg> $ {rates.USD?.toFixed(2)} · € {rates.EUR?.toFixed(2)} · {rateDate}</span>
                  : <span className="border border-amber-300/60 bg-amber-50 px-2 py-1 font-data text-[10px] font-medium text-amber-700 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300">{t('dashboard.hero.exRateFallback')} · $ {rates.USD?.toFixed(2)} · € {rates.EUR?.toFixed(2)}</span>
              }
            </div>
          </div>
          <Link
            to="/add"
            className="shrink-0 rounded-[3px] bg-[hsl(var(--mode-accent))] px-3.5 py-2.5 text-sm font-semibold text-[hsl(var(--primary-foreground))] transition-colors hover:brightness-95 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[hsl(var(--ring))] focus-visible:ring-offset-2 sm:px-4"
          >
            {t('dashboard.addButton')}
          </Link>
        </div>
      </div>

      {/* Balance cards */}
      {isWorkMode ? (
        <StaggerContainer className="grid grid-cols-2 gap-px overflow-hidden border border-[hsl(var(--border))] bg-[hsl(var(--border))]">
          <StaggerItem>
            <div className="h-full bg-[hsl(var(--card))] p-3 sm:p-5">
              <div className="mb-2 flex items-center gap-2 sm:mb-3">
                <div className="flex h-7 w-7 shrink-0 items-center justify-center border border-[hsl(var(--border))] text-[hsl(var(--mode-accent))] sm:h-8 sm:w-8">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" className="h-4 w-4"><rect x="2" y="7" width="20" height="14" rx="2" /><path d="M16 7V5a2 2 0 00-4 0v2" /><line x1="12" y1="12" x2="12" y2="16" /><line x1="10" y1="14" x2="14" y2="14" /></svg>
                </div>
                <p className="truncate text-xs font-semibold tracking-wide text-[hsl(var(--muted-foreground))]">{t('dashboard.balance.public')}</p>
              </div>
              <p className="font-data truncate whitespace-nowrap text-lg font-semibold leading-tight tabular-nums text-[hsl(var(--foreground))] sm:text-xl md:text-2xl">
                <CompactAmount compact={fmtCompact(companyBalance)} exact={fmtExact(companyBalance)} />
              </p>
              <p className="mt-1 text-[10px] text-[hsl(var(--muted-foreground))] sm:mt-1.5 sm:text-[11px]">{t('dashboard.balance.balanceLabel')}</p>
            </div>
          </StaggerItem>
          <StaggerItem>
            <div className="h-full bg-[hsl(var(--card))] p-3 sm:p-5">
              <div className="mb-2 flex items-center gap-2 sm:mb-3">
                <div className="flex h-7 w-7 shrink-0 items-center justify-center border border-[hsl(var(--border))] text-amber-600 dark:text-amber-400 sm:h-8 sm:w-8">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" className="h-4 w-4"><path d="M12 2v20M17 5H9.5a3.5 3.5 0 100 7h5a3.5 3.5 0 110 7H6" /></svg>
                </div>
                <p className="truncate text-xs font-semibold tracking-wide text-[hsl(var(--muted-foreground))]">{t('dashboard.balance.personalPending')}</p>
              </div>
              <p className="font-data truncate whitespace-nowrap text-lg font-semibold leading-tight tabular-nums text-[hsl(var(--foreground))] sm:text-xl md:text-2xl">
                <CompactAmount compact={fmtCompact(personalOutstanding)} exact={fmtExact(personalOutstanding)} />
              </p>
              <p className="mt-1 text-[10px] text-[hsl(var(--muted-foreground))] sm:mt-1.5 sm:text-[11px]">{t('dashboard.balance.pendingLabel')}</p>
            </div>
          </StaggerItem>
        </StaggerContainer>
      ) : (
        <StaggerContainer className="grid grid-cols-1 gap-px overflow-hidden border border-[hsl(var(--border))] bg-[hsl(var(--border))] sm:grid-cols-2">
          <StaggerItem>
            <div className="h-full bg-[hsl(var(--card))] p-4 sm:p-5">
              <div>
                <div className="flex items-center gap-2 mb-2">
                  <span className="inline-flex h-8 w-8 items-center justify-center border border-[hsl(var(--border))] text-[hsl(var(--mode-accent))]"><IconSparkles /></span>
                  <p className="text-xs font-semibold tracking-wide text-[hsl(var(--muted-foreground))]">{t('dashboard.balance.personalAdvance')}</p>
                </div>
                <p className="font-data mt-1 text-xl font-semibold tabular-nums text-[hsl(var(--foreground))] md:text-2xl">
                  <CompactAmount compact={fmtCompact(personalBalance)} exact={fmtExact(personalBalance)} />
                </p>
                <p className="mt-1.5 text-[11px] text-[hsl(var(--muted-foreground))]">{t('dashboard.balance.balanceLabel')}</p>
              </div>
            </div>
          </StaggerItem>
          <StaggerItem>
            <div className="h-full bg-[hsl(var(--card))] p-4 sm:p-5">
              <div>
                <div className="flex items-center gap-2 mb-2">
                  <span className="inline-flex h-8 w-8 items-center justify-center border border-[hsl(var(--border))] text-rose-600 dark:text-rose-400"><IconChart /></span>
                  <p className="text-xs font-semibold tracking-wide text-[hsl(var(--muted-foreground))]">{t('transactions.summary.expense')}</p>
                </div>
                <p className="font-data mt-1 text-xl font-semibold tabular-nums text-rose-600 dark:text-rose-400 md:text-2xl">
                  <CompactAmount compact={fmtCompact(personalTotalExpense)} exact={fmtExact(personalTotalExpense)} />
                </p>
                <p className="mt-1.5 text-[11px] text-[hsl(var(--muted-foreground))]">{t('dashboard.balance.lifeExpenseLabel')}</p>
              </div>
            </div>
          </StaggerItem>
        </StaggerContainer>
      )}

      {/* Monthly insights */}
      <FinanceCard className="rounded-[3px] border-[hsl(var(--border))] bg-[hsl(var(--card))] shadow-none [&_h2]:font-display [&_h2]:text-base [&_h2]:text-[hsl(var(--foreground))]">
        <SectionHeader title={t('dashboard.insights.title')} subtitle={t('dashboard.insights.subtitle')} />
        <div className="grid grid-cols-2 gap-px overflow-hidden border border-[hsl(var(--border))] bg-[hsl(var(--border))] lg:grid-cols-4">
          {[
            { label: t('dashboard.insights.income'), value: monthInsights.income, tone: 'text-emerald-600 dark:text-emerald-300' },
            { label: t('dashboard.insights.expense'), value: monthInsights.expense, tone: 'text-rose-500 dark:text-rose-300' },
            { label: t('dashboard.insights.net'), value: monthInsights.net, tone: monthInsights.net >= 0 ? 'text-[hsl(var(--mode-accent))]' : 'text-orange-500 dark:text-orange-300' },
          ].map(item => (
            <div key={item.label} className="bg-[hsl(var(--card))] p-3 sm:p-4">
              <p className="font-data text-[10px] font-semibold uppercase tracking-[0.12em] text-[hsl(var(--muted-foreground))]">{item.label}</p>
              <p className={`font-data mt-1.5 text-lg font-semibold tabular-nums ${item.tone}`}>
                <CompactAmount compact={fmtCompact(item.value)} exact={fmtExact(item.value)} prefix={item.label === t('dashboard.insights.net') && item.value >= 0 ? '+' : ''} />
              </p>
            </div>
          ))}
          <div className="bg-[hsl(var(--card))] p-3 sm:p-4">
            <p className="font-data text-[10px] font-semibold uppercase tracking-[0.12em] text-[hsl(var(--muted-foreground))]">{t('dashboard.insights.topCategory')}</p>
            {monthInsights.topCategory ? (
              <>
                <p className="mt-1.5 truncate text-sm font-semibold text-[hsl(var(--foreground))]">{categoryLabel(monthInsights.topCategory.category)}</p>
                <p className="font-data text-xs font-semibold tabular-nums text-[hsl(var(--muted-foreground))]">{fmtCompact(monthInsights.topCategory.amount)}</p>
              </>
            ) : (
              <p className="mt-2 text-xs text-[hsl(var(--muted-foreground))]">{t('dashboard.insights.noCategory')}</p>
            )}
          </div>
        </div>
      </FinanceCard>

      {/* Budget and recurring preview */}
      <div className="grid gap-4 lg:grid-cols-2">
        <FinanceCard className="rounded-[3px] border-[hsl(var(--border))] border-t-2 border-t-[hsl(var(--mode-accent))] bg-[hsl(var(--card))] shadow-none [&_h2]:font-display [&_h2]:text-base [&_h2]:text-[hsl(var(--foreground))]">
          <SectionHeader
            title={t('dashboard.budgetCard.title')}
            subtitle={t('dashboard.budgetCard.subtitle')}
            action={<Link to="/budgets" className="border-b border-[hsl(var(--mode-accent))] px-1 py-1 text-xs font-semibold text-[hsl(var(--mode-accent))] transition-opacity hover:opacity-70">{t('dashboard.budgetCard.action')}</Link>}
          />
          {budgetSummary?.total_budget ? (
            <div className="space-y-3">
              <div className="flex items-end justify-between gap-3">
                <div>
                  <p className="font-data text-[10px] font-semibold uppercase tracking-[0.12em] text-[hsl(var(--muted-foreground))]">{t('budgets.actual')}</p>
                  <p className="font-data mt-1 text-xl font-semibold tabular-nums text-[hsl(var(--foreground))]">{fmtCompact(budgetSummary.total_budget.actual_yuan)}</p>
                </div>
                <div className="text-right">
                  <p className="font-data text-[10px] font-semibold uppercase tracking-[0.12em] text-[hsl(var(--muted-foreground))]">{t('budgets.planned')}</p>
                  <p className="font-data mt-1 text-sm font-semibold tabular-nums text-[hsl(var(--muted-foreground))]">{fmtCompact(budgetSummary.total_budget.budget.base_amount_yuan)}</p>
                </div>
              </div>
              <ProgressBar value={budgetSummary.total_budget.usage_ratio} tone={budgetSummary.total_budget.status === 'over' ? 'danger' : budgetSummary.total_budget.status === 'warning' ? 'warning' : 'success'} />
              <div className="font-data flex justify-between text-xs text-[hsl(var(--muted-foreground))]">
                <span>{Math.round(budgetSummary.total_budget.usage_ratio * 100)}%</span>
                <span>{budgetSummary.total_budget.remaining_yuan < 0 ? t('budgets.overBy') : t('budgets.remaining')}: {fmtCompact(Math.abs(budgetSummary.total_budget.remaining_yuan))}</span>
              </div>
            </div>
          ) : (
            <EmptyState title={t('dashboard.budgetCard.emptyTitle')} description={t('dashboard.budgetCard.emptyDesc')} />
          )}
        </FinanceCard>
        <FinanceCard className="rounded-[3px] border-[hsl(var(--border))] border-t-2 border-t-[hsl(var(--mode-accent))] bg-[hsl(var(--card))] shadow-none [&_h2]:font-display [&_h2]:text-base [&_h2]:text-[hsl(var(--foreground))]">
          <SectionHeader
            title={t('dashboard.recurringCard.title')}
            subtitle={t('dashboard.recurringCard.subtitle')}
            action={<Link to="/recurring" className="border-b border-[hsl(var(--mode-accent))] px-1 py-1 text-xs font-semibold text-[hsl(var(--mode-accent))] transition-opacity hover:opacity-70">{t('dashboard.recurringCard.action')}</Link>}
          />
          {recurringRules.length > 0 ? (
            <div className="space-y-3">
              <div className="border-l-2 border-l-[hsl(var(--mode-accent))] bg-[hsl(var(--muted))]/60 p-4 text-sm leading-relaxed text-[hsl(var(--foreground))]">
                {nextRecurringRule
                  ? t('dashboard.recurringCard.next', { name: nextRecurringRule.name, time: nextRecurringRule.next_occurred_at })
                  : t('dashboard.recurringCard.desc')}
              </div>
              <p className="font-data text-xs font-semibold text-[hsl(var(--muted-foreground))]">{t('dashboard.recurringCard.activeCount', { count: recurringRules.filter(rule => rule.status === 'active').length })}</p>
            </div>
          ) : (
            <EmptyState title={t('dashboard.recurringCard.emptyTitle')} description={t('dashboard.recurringCard.emptyDesc')} action={<Link to="/recurring" className="border-b border-[hsl(var(--mode-accent))] px-1 py-1 text-xs font-semibold text-[hsl(var(--mode-accent))]">{t('dashboard.recurringCard.action')}</Link>} />
          )}
        </FinanceCard>
      </div>

      {/* Pending action hints */}
      {hasPending && (
        <div className="border border-[hsl(var(--border))] border-l-2 border-l-amber-500 bg-[hsl(var(--card))] p-5">
          <div className="flex items-center justify-between mb-3">
            <h2 className="page-kicker">{t('dashboard.pending.title')}</h2>
            {pendingAnalysis.headerHint && (
              <span className="font-data text-[10px] font-medium text-[hsl(var(--muted-foreground))]">{pendingAnalysis.headerHint}</span>
            )}
          </div>
          <div className="divide-y divide-[hsl(var(--border))] border-y border-[hsl(var(--border))]">
            {notUploaded.length > 0 && (
              <Link to="/transactions?source=personal" className="flex items-center gap-3 px-1 py-3 transition-colors hover:bg-amber-50/60 dark:hover:bg-amber-500/5">
                <span className="flex h-8 w-8 shrink-0 items-center justify-center border border-amber-300/60 text-amber-600 dark:border-amber-500/30 dark:text-amber-400"><IconUpload /></span>
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium text-[hsl(var(--foreground))]">{notUploaded.length} {t('transactions.badges.notUploaded')}</p>
                  <p className="mt-0.5 text-xs text-[hsl(var(--muted-foreground))]">{pendingAnalysis.notUploadedSub}</p>
                </div>
                <svg className="w-4 h-4 text-amber-400 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" /></svg>
              </Link>
            )}
            {uploadedNotReimbursed.length > 0 && (
              <Link to="/match" className="flex items-center gap-3 px-1 py-3 transition-colors hover:bg-[hsl(var(--mode-accent-wash))]">
                <span className="flex h-8 w-8 shrink-0 items-center justify-center border border-[hsl(var(--mode-accent))]/40 text-[hsl(var(--mode-accent))]"><IconSearch /></span>
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium text-[hsl(var(--foreground))]">{uploadedNotReimbursed.length} {t('transactions.badges.pending')}</p>
                  <p className="mt-0.5 text-xs text-[hsl(var(--muted-foreground))]">{pendingAnalysis.uploadedNotReimbursedSub}</p>
                </div>
                <svg className="h-4 w-4 shrink-0 text-[hsl(var(--mode-accent))]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" /></svg>
              </Link>
            )}
          </div>
        </div>
      )}
      {allClear && (
        <div className="border border-[hsl(var(--border))] border-l-2 border-l-emerald-500 bg-[hsl(var(--card))] p-5">
          <div className="flex items-center gap-3">
            <span className="flex h-8 w-8 shrink-0 items-center justify-center border border-emerald-300/60 text-emerald-600 dark:border-emerald-500/30 dark:text-emerald-400"><IconCheck /></span>
            <div>
              <p className="text-sm font-medium text-[hsl(var(--foreground))]">
                {t('dashboard.pending.noPending')}
              </p>
              <p className="mt-0.5 text-xs text-[hsl(var(--muted-foreground))]">
                {t('dashboard.pending.tip')}
              </p>
            </div>
          </div>
        </div>
      )}

      {/* Feature guide */}
      <div className="border-y border-[hsl(var(--border))] bg-[hsl(var(--card))]/40 py-5">
        <h2 className="page-kicker mb-4 px-1">{t('dashboard.featureNavTitle')}</h2>
        <div className="grid grid-cols-1 gap-px overflow-hidden border border-[hsl(var(--border))] bg-[hsl(var(--border))] sm:grid-cols-2">
          {featureCards.map((f) => (
            <AnimatedCard
              key={f.to}
              className="bg-[hsl(var(--card))]"
            >
              <Link
                to={f.to}
                className="group flex min-h-full items-start gap-3 bg-[hsl(var(--card))] p-4 transition-colors hover:bg-[hsl(var(--muted))]/60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-[hsl(var(--ring))]"
              >
                <span className="flex h-9 w-9 shrink-0 items-center justify-center border border-[hsl(var(--border))] text-[hsl(var(--mode-accent))] transition-transform group-hover:-translate-y-0.5">
                  <f.Icon />
                </span>
                <div className="min-w-0">
                  <p className="text-sm font-semibold text-[hsl(var(--foreground))]">{t(f.titleKey)}</p>
                  <p className="mt-0.5 text-xs leading-relaxed text-[hsl(var(--muted-foreground))]">{t(f.descKey)}</p>
                </div>
              </Link>
            </AnimatedCard>
          ))}
        </div>
      </div>

      {/* Workflow guide — timeline style with tabs */}
      <div className="border border-[hsl(var(--border))] bg-[hsl(var(--card))] p-5">
        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2 mb-5">
          <h2 className="page-kicker">{t('dashboard.workflowTitle')}</h2>
          {isWorkMode && (
            <div role="group" aria-label={t('dashboard.workflowTitle')} className="inline-flex border border-[hsl(var(--border))] bg-[hsl(var(--muted))]/60 p-0.5">
              {(['company', 'personal'] as const).map((source) => (
                <button
                  key={source}
                  type="button"
                  onClick={() => setWorkWorkflowTab(source)}
                  aria-pressed={workflowTab === source}
                  className={`px-2.5 py-1 font-data text-[10px] font-bold transition-colors ${workflowTab === source
                    ? 'bg-[hsl(var(--card))] text-[hsl(var(--mode-accent))] outline outline-1 outline-[hsl(var(--border))]'
                    : 'text-[hsl(var(--muted-foreground))] hover:text-[hsl(var(--foreground))]'
                  }`}
                >
                  {source === 'company' ? t('dashboard.workflow.companyTitle') : t('dashboard.workflow.personalTitle')}
                </button>
              ))}
            </div>
          )}
        </div>

        {/* Personal flow */}
        {workflowTab === 'personal' && (
          <div className="space-y-0">
            {([
              { step: '1', Icon: IconPen, titleKey: isWorkMode ? 'dashboard.workflow.personalStep1' : 'dashboard.workflow.personalLifeStep1', descKey: isWorkMode ? 'dashboard.workflow.personalDesc1' : 'dashboard.workflow.personalLifeDesc1' },
              { step: '2', Icon: IconUpload, titleKey: isWorkMode ? 'dashboard.workflow.personalStep2' : 'dashboard.workflow.personalLifeStep2', descKey: isWorkMode ? 'dashboard.workflow.personalDesc2' : 'dashboard.workflow.personalLifeDesc2' },
              { step: '3', Icon: IconSearch, titleKey: isWorkMode ? 'dashboard.workflow.personalStep3' : 'dashboard.workflow.personalLifeStep3', descKey: isWorkMode ? 'dashboard.workflow.personalDesc3' : 'dashboard.workflow.personalLifeDesc3' },
              { step: '4', Icon: IconCheck, titleKey: isWorkMode ? 'dashboard.workflow.personalStep4' : 'dashboard.workflow.personalLifeStep4', descKey: isWorkMode ? 'dashboard.workflow.personalDesc4' : 'dashboard.workflow.personalLifeDesc4' },
            ] as const).map((s, i, arr) => (
              <div key={s.step} className="flex gap-3">
                {/* Timeline spine */}
                <div className="flex flex-col items-center">
                  <div className="flex h-8 w-8 shrink-0 items-center justify-center border border-[hsl(var(--mode-accent))]/40 text-[hsl(var(--mode-accent))]">
                    <s.Icon />
                  </div>
                  {i < arr.length - 1 && <div className="my-1 w-px flex-1 bg-[hsl(var(--border))]" />}
                </div>
                {/* Content */}
                <div className={`pb-4 ${i === arr.length - 1 ? 'pb-0' : ''}`}>
                  <div className="flex items-center gap-2">
                    <span className="font-data text-[10px] font-bold text-[hsl(var(--mode-accent))]">STEP {s.step}</span>
                    <p className="text-sm font-semibold text-[hsl(var(--foreground))]">{t(s.titleKey)}</p>
                  </div>
                  <p className="mt-0.5 text-xs leading-relaxed text-[hsl(var(--muted-foreground))]">{t(s.descKey)}</p>
                </div>
              </div>
            ))}
          </div>
        )}

        {/* Company flow */}
        {isWorkMode && workflowTab === 'company' && (
          <div className="space-y-0">
            {([
              { step: '1', Icon: IconPen, titleKey: 'dashboard.workflow.companyStep1', descKey: 'dashboard.workflow.companyDesc1' },
              { step: '2', Icon: IconUpload, titleKey: 'dashboard.workflow.companyStep2', descKey: 'dashboard.workflow.companyDesc2' },
              { step: '3', Icon: IconCheck, titleKey: 'dashboard.workflow.companyStep3', descKey: 'dashboard.workflow.companyDesc3' },
            ] as const).map((s, i, arr) => (
              <div key={s.step} className="flex gap-3">
                {/* Timeline spine */}
                <div className="flex flex-col items-center">
                  <div className="flex h-8 w-8 shrink-0 items-center justify-center border border-[hsl(var(--mode-accent))]/40 text-[hsl(var(--mode-accent))]">
                    <s.Icon />
                  </div>
                  {i < arr.length - 1 && <div className="my-1 w-px flex-1 bg-[hsl(var(--border))]" />}
                </div>
                {/* Content */}
                <div className={`pb-4 ${i === arr.length - 1 ? 'pb-0' : ''}`}>
                  <div className="flex items-center gap-2">
                    <span className="font-data text-[10px] font-bold text-[hsl(var(--mode-accent))]">STEP {s.step}</span>
                    <p className="text-sm font-semibold text-[hsl(var(--foreground))]">{t(s.titleKey)}</p>
                  </div>
                  <p className="mt-0.5 text-xs leading-relaxed text-[hsl(var(--muted-foreground))]">{t(s.descKey)}</p>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
