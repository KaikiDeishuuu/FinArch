import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../contexts/AuthContext'
import { useExchangeRates } from '../contexts/ExchangeRateContext'
import { toCNY, formatAmountCompact, formatAmountExact } from '../utils/format'
import CompactAmount from '../components/CompactAmount'
import { BrandWatermark } from '../components/Brand'
import { useTransactions } from '../hooks/useTransactions'
import { useAccounts } from '../hooks/useAccounts'
import { useHeartbeat } from '../hooks/useHeartbeat'
import { useOnlineDevices } from '../hooks/useOnlineDevices'
import { StaggerContainer, StaggerItem, AnimatedCard, CardSkeleton } from '../motion'
import { useMode } from '../contexts/ModeContext'

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
    color: 'bg-violet-50 dark:bg-violet-500/10 border-violet-100 dark:border-violet-500/20',
    iconBg: 'bg-violet-100 dark:bg-violet-500/20 text-violet-600 dark:text-violet-400',
  },
  {
    to: '/add',
    Icon: IconPlus,
    titleKey: 'dashboard.features.reimbursement.title',
    descKey: 'dashboard.features.reimbursement.desc',
    color: 'bg-emerald-50 dark:bg-emerald-500/10 border-emerald-100 dark:border-emerald-500/20',
    iconBg: 'bg-emerald-100 dark:bg-emerald-500/20 text-emerald-500 dark:text-emerald-400',
  },
  {
    to: '/match',
    Icon: IconSearch,
    titleKey: 'dashboard.features.smartMatch.title',
    descKey: 'dashboard.features.smartMatch.desc',
    color: 'bg-purple-50 dark:bg-purple-500/10 border-purple-100 dark:border-purple-500/20',
    iconBg: 'bg-purple-100 dark:bg-purple-500/20 text-purple-600 dark:text-purple-400',
  },
  {
    to: '/stats',
    Icon: IconChart,
    titleKey: 'dashboard.features.dataVisualization.title',
    descKey: 'dashboard.features.dataVisualization.desc',
    color: 'bg-orange-50 dark:bg-orange-500/10 border-orange-100 dark:border-orange-500/20',
    iconBg: 'bg-orange-100 dark:bg-orange-500/20 text-orange-600 dark:text-orange-400',
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
  const [workflowTab, setWorkflowTab] = useState<'personal' | 'company'>(isWorkMode ? 'company' : 'personal')

  useEffect(() => {
    setWorkflowTab(isWorkMode ? 'company' : 'personal')
  }, [isWorkMode])

  // Device heartbeat — keeps this device marked as online
  useHeartbeat()
  const { data: onlineDeviceCount } = useOnlineDevices()

  // Greeting — generated once per mount, pick random from i18n array
  const [greetingKey] = useState(getGreetingKey)
  const greetingMessages = t(`dashboard.greeting.${greetingKey}`, { returnObjects: true }) as string[]
  const [greetingIdx] = useState(() => Math.floor(Math.random() * greetingMessages.length))
  const greetingText = greetingMessages[greetingIdx] || greetingMessages[0]

  const { data: transactions = [], isLoading: loading, error: txError } = useTransactions()
  const { data: accounts = [] } = useAccounts()
  const error = txError
    ? ((txError as { response?: { data?: { message?: string } } })?.response?.data?.message ?? t('common.error'))
    : ''

  // Use server-side cached account balances
  const companyBalance = useMemo(() =>
    accounts
      .filter(a => a.type === 'public' && a.is_active)
      .reduce((s, a) => s + a.balance_yuan, 0),
    [accounts]
  )

  const personalBalance = useMemo(() =>
    accounts
      .filter(a => a.type === 'personal' && a.is_active)
      .reduce((s, a) => s + a.balance_yuan, 0),
    [accounts]
  )

  const personalTotalExpense = useMemo(() =>
    transactions
      .filter(t => t.source === 'personal' && t.direction === 'expense')
      .reduce((s, t) => s + toCNY(t.amount_yuan, t.currency || 'CNY', rates), 0),
    [transactions, rates]
  )


  const companyOutstanding = useMemo(() =>
    transactions
      .filter(t => t.source === 'company' && t.direction === 'expense' && !t.reimbursed)
      .reduce((s, t) => s + toCNY(t.amount_yuan, t.currency || 'CNY', rates), 0),
    [transactions, rates]
  )

  const fmtExact = (n: number) => formatAmountExact(n, 'CNY')
  const fmtCompact = (n: number) => formatAmountCompact(n, 'CNY')

  const pendingTxs = isWorkMode ? [] : transactions.filter(t => t.source === 'personal' && t.direction === 'expense' && !t.reimbursed)
  const notUploaded = pendingTxs.filter((t) => !t.uploaded)
  const uploadedNotReimbursed = pendingTxs.filter((t) => t.uploaded && !t.reimbursed)
  const companyNotUploaded = isWorkMode ? transactions.filter(t => t.source === 'company' && t.direction === 'expense' && !t.uploaded) : []
  const companyUploadedNotReimbursed = isWorkMode ? transactions.filter(t => t.source === 'company' && t.direction === 'expense' && t.uploaded && !t.reimbursed) : []

  // ─── Smart pending item analysis ───────────────────────────────────────────
  const hasPending = isWorkMode && (companyNotUploaded.length > 0 || companyUploadedNotReimbursed.length > 0)
  const allClear = !loading && !hasPending && transactions.length > 0

  const pendingAnalysis = useMemo(() => {
    const now = Date.now()
    const DAY = 86400000

    const notUploadedAmount = notUploaded.reduce((s, t) => s + toCNY(t.amount_yuan, t.currency || 'CNY', rates), 0)
    const uploadedNotReimbursedAmount = uploadedNotReimbursed.reduce((s, t) => s + toCNY(t.amount_yuan, t.currency || 'CNY', rates), 0)
    const companyNotUploadedAmount = companyNotUploaded.reduce((s, t) => s + toCNY(t.amount_yuan, t.currency || 'CNY', rates), 0)
    const companyUploadedAmount = companyUploadedNotReimbursed.reduce((s, t) => s + toCNY(t.amount_yuan, t.currency || 'CNY', rates), 0)

    const oldestDate = (txs: typeof transactions) => {
      if (txs.length === 0) return null
      const dates = txs.map(t => new Date(t.occurred_at).getTime()).filter(d => !isNaN(d))
      return dates.length > 0 ? Math.min(...dates) : null
    }

    const oldestNotUploaded = oldestDate(notUploaded)
    const oldestUploaded = oldestDate(uploadedNotReimbursed)
    const oldestCompanyNotUploaded = oldestDate(companyNotUploaded)
    const oldestCompanyUploaded = oldestDate(companyUploadedNotReimbursed)

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

    // Smart sub-messages: companyNotUploaded
    const companyNotUploadedSub = (() => {
      if (companyNotUploaded.length === 0) return ''
      const days = daysSince(oldestCompanyNotUploaded)
      const amt = fmtExact(companyNotUploadedAmount)
      if (days > 30) return t('dashboard.pending.companyNotUploaded.over30d', { amt, days })
      if (days > 7) return t('dashboard.pending.companyNotUploaded.over7d', { amt })
      if (companyNotUploaded.length >= 5) return t('dashboard.pending.companyNotUploaded.manyItems', { amt, count: companyNotUploaded.length })
      return t('dashboard.pending.companyNotUploaded.default', { amt })
    })()

    // Smart sub-messages: companyUploadedNotReimbursed
    const companyUploadedSub = (() => {
      if (companyUploadedNotReimbursed.length === 0) return ''
      const days = daysSince(oldestCompanyUploaded)
      const amt = fmtExact(companyUploadedAmount)
      if (days > 30) return t('dashboard.pending.companyUploaded.over30d', { amt })
      if (days > 14) return t('dashboard.pending.companyUploaded.over14d', { amt })
      return t('dashboard.pending.companyUploaded.default', { amt })
    })()

    // Urgency header
    const maxDays = Math.max(daysSince(oldestNotUploaded), daysSince(oldestUploaded), daysSince(oldestCompanyNotUploaded), daysSince(oldestCompanyUploaded))
    const totalPending = notUploaded.length + uploadedNotReimbursed.length + companyNotUploaded.length + companyUploadedNotReimbursed.length
    const totalAmount = notUploadedAmount + uploadedNotReimbursedAmount + companyNotUploadedAmount + companyUploadedAmount

    let headerHint = ''
    if (maxDays > 30) headerHint = t('dashboard.pending.header.overdue', { days: maxDays })
    else if (totalAmount >= 10000) headerHint = t('dashboard.pending.header.highAmount', { count: totalPending, amt: fmtExact(totalAmount) })
    else if (totalPending >= 8) headerHint = t('dashboard.pending.header.manyItems', { count: totalPending })
    else if (totalPending > 0) headerHint = t('dashboard.pending.header.default', { count: totalPending })

    return { notUploadedSub, uploadedNotReimbursedSub, companyNotUploadedSub, companyUploadedSub, headerHint }
  }, [notUploaded, uploadedNotReimbursed, companyNotUploaded, companyUploadedNotReimbursed, rates, t])

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
  const featureCards = FEATURES.filter((f) => isWorkMode || f.to !== '/match')

  return (
    <div className="space-y-6">
      {/* Hero Header */}
      <div className="relative overflow-hidden bg-gradient-to-br from-violet-600 via-purple-600 to-fuchsia-500 rounded-2xl p-6 md:p-7 text-white shadow-lg shadow-violet-500/20 dark:shadow-violet-900/30">
        <div className="absolute -top-8 -right-8 w-32 h-32 bg-white/10 rounded-full blur-2xl" />
        <div className="absolute -bottom-6 -left-6 w-24 h-24 bg-fuchsia-400/20 rounded-full blur-2xl" />
        <BrandWatermark className="absolute -bottom-2 right-4 opacity-[0.08]" opacity={0.12} />
        <div className="relative flex items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="flex items-center gap-2 mb-1">
              <p className="text-white/70 text-xs font-medium">{new Date().toLocaleDateString(dateLocale, { year: 'numeric', month: 'long', day: 'numeric' })}</p>
              {onlineDeviceCount != null && (
                <span className="text-[10px] px-1.5 py-0.5 rounded-full bg-white/10 text-white/60 backdrop-blur-sm inline-flex items-center gap-1">
                  <span className="relative flex h-1.5 w-1.5"><span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span><span className="relative inline-flex rounded-full h-1.5 w-1.5 bg-emerald-300"></span></span>
                  {t('common.devices', { count: onlineDeviceCount })}
                </span>
              )}
            </div>
            <h1 className="text-2xl md:text-3xl font-bold tracking-tight truncate">{greetingText}{i18n.language === 'zh' ? '，' : ', '}{user?.nickname || user?.username || user?.email?.split('@')[0]}</h1>
            <div className="flex items-center gap-2 mt-2 flex-wrap">
              {ratesLoading
                ? <span className="text-[10px] px-2 py-0.5 rounded-full bg-white/15 text-white/70 backdrop-blur-sm">{t('dashboard.hero.exRateLoading')}</span>
                : rateDate
                  ? <span className="text-[10px] px-2 py-0.5 rounded-full bg-white/15 text-white/90 backdrop-blur-sm font-medium inline-flex items-center gap-1"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} className="w-3 h-3"><path d="M4 7h16M4 17h16M10 4c-2 2-2 14 0 16M14 4c2 2 2 14 0 16" /></svg> $ {rates.USD?.toFixed(2)} · € {rates.EUR?.toFixed(2)} · {rateDate}</span>
                  : <span className="text-[10px] px-2 py-0.5 rounded-full bg-amber-400/20 text-amber-200 backdrop-blur-sm font-medium">{t('dashboard.hero.exRateFallback')} · $ {rates.USD?.toFixed(2)} · € {rates.EUR?.toFixed(2)}</span>
              }
            </div>
          </div>
          <Link
            to="/add"
            className="shrink-0 bg-white/20 hover:bg-white/30 active:scale-95 backdrop-blur-sm text-white text-sm font-semibold px-4 py-2.5 rounded-xl transition-all border border-white/20"
          >
            {t('dashboard.addButton')}
          </Link>
        </div>
      </div>

      {/* Balance cards */}
      {isWorkMode ? (
        <StaggerContainer className="grid grid-cols-2 gap-2 sm:gap-3">
          <StaggerItem>
            <div className="relative overflow-hidden rounded-2xl border border-emerald-100/80 dark:border-emerald-500/20 bg-gradient-to-br from-emerald-50 via-white to-emerald-100/70 dark:from-emerald-500/12 dark:via-[hsl(260,15%,11%)] dark:to-emerald-500/5 p-3 sm:p-5 shadow-sm hover:shadow-md dark:hover:shadow-lg dark:hover:shadow-black/20 transition-shadow">
              <div className="flex items-center gap-2 mb-2 sm:mb-3">
                <div className="w-7 h-7 sm:w-8 sm:h-8 rounded-xl bg-emerald-50 dark:bg-emerald-500/15 flex items-center justify-center shrink-0">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" className="w-4.5 h-4.5 text-emerald-600 dark:text-emerald-400"><rect x="2" y="7" width="20" height="14" rx="2" /><path d="M16 7V5a2 2 0 00-4 0v2" /><line x1="12" y1="12" x2="12" y2="16" /><line x1="10" y1="14" x2="14" y2="14" /></svg>
                </div>
                <p className="text-xs font-semibold text-gray-400 dark:text-gray-500 tracking-wide truncate">{t('dashboard.balance.public')}</p>
              </div>
              <p className="text-lg sm:text-xl md:text-2xl font-bold text-gray-800 dark:text-gray-100 leading-tight tabular-nums whitespace-nowrap truncate">
                <CompactAmount compact={fmtCompact(companyBalance)} exact={fmtExact(companyBalance)} />
              </p>
              <p className="text-[10px] sm:text-[11px] text-gray-400 dark:text-gray-500 mt-1 sm:mt-1.5">{t('dashboard.balance.balanceLabel')}</p>
            </div>
          </StaggerItem>
          <StaggerItem>
            <div className="relative overflow-hidden rounded-2xl border border-rose-100/80 dark:border-rose-500/20 bg-gradient-to-br from-rose-50 via-white to-orange-100/70 dark:from-rose-500/12 dark:via-[hsl(260,15%,11%)] dark:to-orange-500/10 p-3 sm:p-5 shadow-sm hover:shadow-md dark:hover:shadow-lg dark:hover:shadow-black/20 transition-shadow">
              <div className="flex items-center gap-2 mb-2 sm:mb-3">
                <div className="w-7 h-7 sm:w-8 sm:h-8 rounded-xl bg-rose-50 dark:bg-rose-500/15 flex items-center justify-center shrink-0">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" className="w-4.5 h-4.5 text-rose-500 dark:text-rose-400"><path d="M12 2v20M17 5H9.5a3.5 3.5 0 100 7h5a3.5 3.5 0 110 7H6" /></svg>
                </div>
                <p className="text-xs font-semibold text-gray-400 dark:text-gray-500 tracking-wide truncate">{t('dashboard.balance.publicPending')}</p>
              </div>
              <p className="text-lg sm:text-xl md:text-2xl font-bold text-gray-800 dark:text-gray-100 leading-tight tabular-nums whitespace-nowrap truncate">
                <CompactAmount compact={fmtCompact(companyOutstanding)} exact={fmtExact(companyOutstanding)} />
              </p>
              <p className="text-[10px] sm:text-[11px] text-gray-400 dark:text-gray-500 mt-1 sm:mt-1.5">{t('dashboard.balance.pendingLabel')}</p>
            </div>
          </StaggerItem>
        </StaggerContainer>
      ) : (
        <StaggerContainer className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <StaggerItem>
            <div className="relative overflow-hidden rounded-2xl border border-emerald-100/80 dark:border-emerald-500/20 bg-gradient-to-br from-emerald-50 via-white to-emerald-100/70 dark:from-emerald-500/12 dark:via-[hsl(260,15%,11%)] dark:to-emerald-500/5 p-4 sm:p-5 shadow-sm hover:shadow-md dark:hover:shadow-lg dark:hover:shadow-black/20 transition-shadow">
              <div className="absolute -top-6 -right-6 w-20 h-20 bg-emerald-300/20 rounded-full blur-2xl" />
              <div className="relative">
                <div className="flex items-center gap-2 mb-2">
                  <span className="w-8 h-8 rounded-xl bg-emerald-100 dark:bg-emerald-500/20 text-emerald-600 dark:text-emerald-300 inline-flex items-center justify-center"><IconSparkles /></span>
                  <p className="text-xs font-semibold text-emerald-700/80 dark:text-emerald-300/80 tracking-wide">{t('dashboard.balance.personalAdvance')}</p>
                </div>
                <p className="mt-1 text-xl md:text-2xl font-bold text-emerald-700 dark:text-emerald-200 tabular-nums">
                  <CompactAmount compact={fmtCompact(personalBalance)} exact={fmtExact(personalBalance)} />
                </p>
                <p className="text-[11px] text-emerald-700/60 dark:text-emerald-300/70 mt-1.5">{t('dashboard.balance.balanceLabel')}</p>
              </div>
            </div>
          </StaggerItem>
          <StaggerItem>
            <div className="relative overflow-hidden rounded-2xl border border-rose-100/80 dark:border-rose-500/20 bg-gradient-to-br from-rose-50 via-white to-orange-100/70 dark:from-rose-500/12 dark:via-[hsl(260,15%,11%)] dark:to-orange-500/10 p-4 sm:p-5 shadow-sm hover:shadow-md dark:hover:shadow-lg dark:hover:shadow-black/20 transition-shadow">
              <div className="absolute -bottom-7 -left-6 w-24 h-24 bg-rose-300/20 rounded-full blur-2xl" />
              <div className="relative">
                <div className="flex items-center gap-2 mb-2">
                  <span className="w-8 h-8 rounded-xl bg-rose-100 dark:bg-rose-500/20 text-rose-600 dark:text-rose-300 inline-flex items-center justify-center"><IconChart /></span>
                  <p className="text-xs font-semibold text-rose-700/80 dark:text-rose-300/80 tracking-wide">{t('transactions.summary.expense')}</p>
                </div>
                <p className="mt-1 text-xl md:text-2xl font-bold text-rose-600 dark:text-rose-300 tabular-nums">
                  <CompactAmount compact={fmtCompact(personalTotalExpense)} exact={fmtExact(personalTotalExpense)} />
                </p>
                <p className="text-[11px] text-rose-700/60 dark:text-rose-300/70 mt-1.5">{t('dashboard.balance.advanceLabel')}</p>
              </div>
            </div>
          </StaggerItem>
        </StaggerContainer>
      )}

      {/* Pending action hints */}
      {hasPending && (
        <div className="bg-white dark:bg-[hsl(260,15%,11%)] rounded-2xl border border-gray-100/80 dark:border-gray-800/50 p-5 shadow-sm hover:shadow-md transition-shadow">
          <div className="flex items-center justify-between mb-3">
            <h2 className="text-xs font-semibold text-gray-400 dark:text-gray-500 tracking-wide">{t('dashboard.pending.title')}</h2>
            {pendingAnalysis.headerHint && (
              <span className="text-[10px] text-gray-400 dark:text-gray-500 font-medium">{pendingAnalysis.headerHint}</span>
            )}
          </div>
          <div className="space-y-2">
            {notUploaded.length > 0 && (
              <Link to="/transactions" className="flex items-center gap-3 p-3 rounded-xl bg-amber-50 dark:bg-amber-500/10 border border-amber-100 dark:border-amber-500/20 hover:border-amber-300 dark:hover:border-amber-400/40 transition-colors">
                <span className="w-8 h-8 rounded-lg bg-amber-100 dark:bg-amber-500/20 text-amber-600 dark:text-amber-400 flex items-center justify-center shrink-0"><IconUpload /></span>
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium text-amber-800 dark:text-amber-300">{notUploaded.length} {t('transactions.badges.notUploaded')}</p>
                  <p className="text-xs text-amber-600 dark:text-amber-400/70 mt-0.5">{pendingAnalysis.notUploadedSub}</p>
                </div>
                <svg className="w-4 h-4 text-amber-400 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" /></svg>
              </Link>
            )}
            {uploadedNotReimbursed.length > 0 && (
              <Link to="/match" className="flex items-center gap-3 p-3 rounded-xl bg-violet-50 dark:bg-violet-500/10 border border-violet-100 dark:border-violet-500/20 hover:border-violet-300 dark:hover:border-violet-400/40 transition-colors">
                <span className="w-8 h-8 rounded-lg bg-violet-100 dark:bg-violet-500/20 text-violet-600 dark:text-violet-400 flex items-center justify-center shrink-0"><IconSearch /></span>
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium text-violet-800 dark:text-violet-300">{uploadedNotReimbursed.length} {t('transactions.badges.pending')}</p>
                  <p className="text-xs text-violet-600 dark:text-violet-400/70 mt-0.5">{pendingAnalysis.uploadedNotReimbursedSub}</p>
                </div>
                <svg className="w-4 h-4 text-violet-400 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" /></svg>
              </Link>
            )}
            {companyNotUploaded.length > 0 && (
              <Link to="/transactions" className="flex items-center gap-3 p-3 rounded-xl bg-slate-50 dark:bg-slate-500/10 border border-slate-200 dark:border-slate-500/20 hover:border-violet-300 dark:hover:border-violet-400/40 transition-colors">
                <span className="w-8 h-8 rounded-lg bg-slate-100 dark:bg-slate-500/20 text-slate-600 dark:text-slate-400 flex items-center justify-center shrink-0"><IconUpload /></span>
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium text-slate-700 dark:text-slate-300">{companyNotUploaded.length} {t('common.company')} {t('transactions.badges.notUploaded')}</p>
                  <p className="text-xs text-slate-500 dark:text-slate-400/70 mt-0.5">{pendingAnalysis.companyNotUploadedSub}</p>
                </div>
                <svg className="w-4 h-4 text-slate-400 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" /></svg>
              </Link>
            )}
            {companyUploadedNotReimbursed.length > 0 && (
              <Link to="/match" className="flex items-center gap-3 p-3 rounded-xl bg-emerald-50 dark:bg-emerald-500/10 border border-emerald-100 dark:border-emerald-500/20 hover:border-emerald-300 dark:hover:border-emerald-400/40 transition-colors">
                <span className="w-8 h-8 rounded-lg bg-emerald-100 dark:bg-emerald-500/20 text-emerald-600 dark:text-emerald-400 flex items-center justify-center shrink-0"><IconSearch /></span>
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium text-emerald-800 dark:text-emerald-300">{companyUploadedNotReimbursed.length} {t('common.company')} {t('transactions.badges.pending')}</p>
                  <p className="text-xs text-emerald-600 dark:text-emerald-400/70 mt-0.5">{pendingAnalysis.companyUploadedSub}</p>
                </div>
                <svg className="w-4 h-4 text-emerald-400 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" /></svg>
              </Link>
            )}
          </div>
        </div>
      )}
      {allClear && (
        <div className="bg-white dark:bg-[hsl(260,15%,11%)] rounded-2xl border border-gray-100/80 dark:border-gray-800/50 p-5 shadow-sm">
          <div className="flex items-center gap-3">
            <span className="w-8 h-8 rounded-lg bg-emerald-50 dark:bg-emerald-500/15 text-emerald-500 dark:text-emerald-400 flex items-center justify-center shrink-0"><IconCheck /></span>
            <div>
              <p className="text-sm font-medium text-gray-700 dark:text-gray-200">
                {isWorkMode ? t('dashboard.pending.noPending') : t('dashboard.life.noPending')}
              </p>
              <p className="text-xs text-gray-400 dark:text-gray-500 mt-0.5">
                {isWorkMode ? t('dashboard.pending.tip') : t('dashboard.life.tip')}
              </p>
              {!isWorkMode && (
                <p className="text-xs text-emerald-500 dark:text-emerald-400 mt-1 font-medium">
                  {t('dashboard.life.allClear')}
                </p>
              )}
            </div>
          </div>
        </div>
      )}

      {/* Feature guide */}
      <div className="bg-white dark:bg-[hsl(260,15%,11%)] rounded-2xl border border-gray-100/80 dark:border-gray-800/50 p-5 shadow-sm hover:shadow-md transition-shadow">
        <h2 className="text-xs font-semibold text-gray-400 dark:text-gray-500 mb-4 tracking-wide">{t('dashboard.featureNavTitle')}</h2>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          {featureCards.map((f) => (
            <AnimatedCard
              key={f.to}
              className="rounded-2xl"
            >
              <Link
                to={f.to}
                className="flex items-start gap-3 p-4 rounded-2xl border border-gray-100/80 dark:border-gray-800/50 transition-all hover:border-violet-200 dark:hover:border-violet-500/40 hover:shadow-md hover:shadow-violet-100/30 dark:hover:shadow-violet-900/20 group bg-white dark:bg-transparent"
              >
                <span className={`w-9 h-9 rounded-xl flex items-center justify-center shrink-0 ${f.iconBg} transition-transform group-hover:scale-105`}>
                  <f.Icon />
                </span>
                <div className="min-w-0">
                  <p className="text-sm font-semibold text-gray-700 dark:text-gray-200">{t(f.titleKey)}</p>
                  <p className="text-xs text-gray-400 dark:text-gray-500 mt-0.5 leading-relaxed">{t(f.descKey)}</p>
                </div>
              </Link>
            </AnimatedCard>
          ))}
        </div>
      </div>

      {/* Workflow guide — timeline style with tabs */}
      <div className="bg-white dark:bg-[hsl(260,15%,11%)] rounded-2xl border border-gray-100/80 dark:border-gray-800/50 p-5 shadow-sm hover:shadow-md transition-shadow">
        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2 mb-5">
          <h2 className="text-xs font-semibold text-gray-400 dark:text-gray-500 tracking-wide">{t('dashboard.workflowTitle')}</h2>
          {isWorkMode && (
            <div className="inline-flex bg-gray-100 dark:bg-gray-800/60 rounded-lg p-0.5">
              <span className="px-2.5 py-1 rounded-md text-[10px] font-bold bg-white dark:bg-gray-700 text-sky-600 dark:text-sky-400 shadow-sm inline-flex items-center gap-1">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} className="w-3 h-3"><path d="M3 10h18" /><path d="M5 10v8M9 10v8M15 10v8M19 10v8" /><path d="M2 18h20" /><path d="m12 4 10 4H2z" /></svg>{t('dashboard.workflow.companyTitle')}
              </span>
            </div>
          )}
        </div>

        {/* Personal flow */}
        {workflowTab === 'personal' && !isWorkMode && (
          <div className="space-y-0">
            {([
              { step: '1', Icon: IconPen, titleKey: 'dashboard.workflow.personalStep1', descKey: 'dashboard.workflow.personalDesc1', color: 'amber' },
              { step: '2', Icon: IconUpload, titleKey: 'dashboard.workflow.personalStep2', descKey: 'dashboard.workflow.personalDesc2', color: 'amber' },
              { step: '3', Icon: IconSearch, titleKey: isWorkMode ? 'dashboard.workflow.personalStep3' : 'dashboard.workflow.personalLifeStep3', descKey: isWorkMode ? 'dashboard.workflow.personalDesc3' : 'dashboard.workflow.personalLifeDesc3', color: 'amber' },
              { step: '4', Icon: IconCheck, titleKey: isWorkMode ? 'dashboard.workflow.personalStep4' : 'dashboard.workflow.personalLifeStep4', descKey: isWorkMode ? 'dashboard.workflow.personalDesc4' : 'dashboard.workflow.personalLifeDesc4', color: 'amber' },
            ] as const).map((s, i, arr) => (
              <div key={s.step} className="flex gap-3">
                {/* Timeline spine */}
                <div className="flex flex-col items-center">
                  <div className="w-8 h-8 rounded-xl bg-amber-100 dark:bg-amber-500/15 text-amber-600 dark:text-amber-400 flex items-center justify-center shrink-0">
                    <s.Icon />
                  </div>
                  {i < arr.length - 1 && <div className="w-px flex-1 bg-amber-200/60 dark:bg-amber-500/20 my-1" />}
                </div>
                {/* Content */}
                <div className={`pb-4 ${i === arr.length - 1 ? 'pb-0' : ''}`}>
                  <div className="flex items-center gap-2">
                    <span className="text-[10px] font-bold text-amber-500/60 dark:text-amber-400/50">STEP {s.step}</span>
                    <p className="text-sm font-semibold text-gray-800 dark:text-gray-200">{t(s.titleKey)}</p>
                  </div>
                  <p className="text-xs text-gray-400 dark:text-gray-500 mt-0.5 leading-relaxed">{t(s.descKey)}</p>
                </div>
              </div>
            ))}
          </div>
        )}

        {/* Company flow */}
        {isWorkMode && workflowTab === 'company' && (
          <div className="space-y-0">
            {([
              { step: '1', Icon: IconPen, titleKey: 'dashboard.workflow.companyStep1', descKey: 'dashboard.workflow.companyDesc1', color: 'sky' },
              { step: '2', Icon: IconUpload, titleKey: 'dashboard.workflow.companyStep2', descKey: 'dashboard.workflow.companyDesc2', color: 'sky' },
              { step: '3', Icon: IconCheck, titleKey: 'dashboard.workflow.companyStep3', descKey: 'dashboard.workflow.companyDesc3', color: 'sky' },
            ] as const).map((s, i, arr) => (
              <div key={s.step} className="flex gap-3">
                {/* Timeline spine */}
                <div className="flex flex-col items-center">
                  <div className="w-8 h-8 rounded-xl bg-sky-100 dark:bg-sky-500/15 text-sky-600 dark:text-sky-400 flex items-center justify-center shrink-0">
                    <s.Icon />
                  </div>
                  {i < arr.length - 1 && <div className="w-px flex-1 bg-sky-200/60 dark:bg-sky-500/20 my-1" />}
                </div>
                {/* Content */}
                <div className={`pb-4 ${i === arr.length - 1 ? 'pb-0' : ''}`}>
                  <div className="flex items-center gap-2">
                    <span className="text-[10px] font-bold text-sky-500/60 dark:text-sky-400/50">STEP {s.step}</span>
                    <p className="text-sm font-semibold text-gray-800 dark:text-gray-200">{t(s.titleKey)}</p>
                  </div>
                  <p className="text-xs text-gray-400 dark:text-gray-500 mt-0.5 leading-relaxed">{t(s.descKey)}</p>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
