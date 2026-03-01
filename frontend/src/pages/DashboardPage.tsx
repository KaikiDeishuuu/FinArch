import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useAuth } from '../contexts/AuthContext'
import { useExchangeRates } from '../contexts/ExchangeRateContext'
import { toCNY, formatAmountCompact, formatAmountExact } from '../utils/format'
import CompactAmount from '../components/CompactAmount'
import { BrandWatermark } from '../components/Brand'
import { useTransactions } from '../hooks/useTransactions'
import { useAccounts } from '../hooks/useAccounts'
import { StaggerContainer, StaggerItem, AnimatedCard, CardSkeleton } from '../motion'

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

const FEATURES = [
  {
    to: '/transactions',
    Icon: IconList,
    title: '交易明细',
    desc: '查看、筛选所有收支记录，标记上传与报销状态',
    color: 'bg-violet-50 border-violet-100',
    iconBg: 'bg-violet-100 text-violet-600',
  },
  {
    to: '/add',
    Icon: IconPlus,
    title: '添加交易',
    desc: '新增一笔收入或支出，支持多类别与项目归属',
    color: 'bg-emerald-50 border-emerald-100',
    iconBg: 'bg-emerald-100 text-emerald-500',
  },
  {
    to: '/match',
    Icon: IconSearch,
    title: '子集匹配',
    desc: '输入报销总额，自动寻找与之精确匹配的交易组合',
    color: 'bg-purple-50 border-purple-100',
    iconBg: 'bg-purple-100 text-purple-600',
  },
  {
    to: '/stats',
    Icon: IconChart,
    title: '统计分析',
    desc: '月度趋势、分类支出和项目汇总可视化报告',
    color: 'bg-orange-50 border-orange-100',
    iconBg: 'bg-orange-100 text-orange-600',
  },
]

// ─── Time-based greeting generator ─────────────────────────────────────────
function generateGreeting() {
  const hour = new Date().getHours()
  const pick = <T,>(arr: T[]): T => arr[Math.floor(Math.random() * arr.length)]

  let timeGreeting: string
  let subMessage: string

  if (hour >= 5 && hour < 9) {
    timeGreeting = pick(['早安', '早上好', '清晨好'])
    subMessage = pick([
      '新的一天，充满可能 ✨',
      '美好的早晨，精神满满！',
      '今天也是元气满满的一天',
      '一日之计在于晨，加油！',
      '阳光正好，微风不燥',
    ])
  } else if (hour >= 9 && hour < 11) {
    timeGreeting = pick(['上午好', '早上好'])
    subMessage = pick([
      '高效工作，专注当下',
      '愿一切顺顺利利',
      '上午效率最高，好好把握',
      '保持专注，你很棒',
    ])
  } else if (hour >= 11 && hour < 13) {
    timeGreeting = pick(['中午好', '午安'])
    subMessage = pick([
      '别忘了吃午饭哦 🍱',
      '适当休息，下午更有活力',
      '午间充电，养足精神',
      '记得补充能量，劳逸结合',
    ])
  } else if (hour >= 13 && hour < 17) {
    timeGreeting = pick(['下午好'])
    subMessage = pick([
      '来杯下午茶提提神 ☕',
      '下午也要保持好状态',
      '坚持就是胜利，加油！',
      '下午过半了，继续加油',
      '困了就活动活动筋骨',
    ])
  } else if (hour >= 17 && hour < 19) {
    timeGreeting = pick(['傍晚好', '下午好'])
    subMessage = pick([
      '忙碌一天，辛苦了',
      '快到下班时间啦 🌇',
      '整理一下今天的收支吧',
      '日落时分，放慢脚步',
    ])
  } else if (hour >= 19 && hour < 22) {
    timeGreeting = pick(['晚上好'])
    subMessage = pick([
      '忙碌一天，放松一下',
      '吃晚饭了吗？别饿着肚子',
      '今天辛苦了，好好休息',
      '夜间时光，属于自己 🌙',
    ])
  } else if (hour >= 22 || hour === 0) {
    timeGreeting = pick(['夜深了', '晚安'])
    subMessage = pick([
      '早点休息，明天会更好',
      '别熬夜哦，身体最重要 🌙',
      '注意休息，晚安',
      '夜已深，早些歇息吧',
      '忙完了就早点睡吧',
    ])
  } else {
    // 1:00–4:59 凌晨
    timeGreeting = pick(['凌晨了', '夜很深了'])
    subMessage = pick([
      '这么晚还在忙？注意身体 💤',
      '熬夜伤身哦，快去睡吧',
      '凌晨了，记得早点休息',
      '身体是革命的本钱，别太拼了',
      '夜猫子也要按时休息呀 🦉',
    ])
  }

  return { timeGreeting, subMessage }
}

export default function DashboardPage() {
  const { user } = useAuth()
  const { rates, rateDate, loading: ratesLoading } = useExchangeRates()

  // Greeting — generated once per mount, stable across re-renders
  const [greeting] = useState(generateGreeting)
  const { data: transactions = [], isLoading: loading, error: txError } = useTransactions()
  const { data: accounts = [] } = useAccounts()
  const error = txError
    ? ((txError as { response?: { data?: { message?: string } } })?.response?.data?.message ?? '数据加载失败，请刷新页面重试')
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

  const personalOutstanding = useMemo(() =>
    transactions
      .filter(t => t.source === 'personal' && t.direction === 'expense' && !t.reimbursed)
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

  const pendingTxs = transactions.filter(t => t.source === 'personal' && !t.reimbursed)
  const notUploaded = pendingTxs.filter((t) => !t.uploaded)
  const uploadedNotReimbursed = pendingTxs.filter((t) => t.uploaded && !t.reimbursed)
  const companyNotUploaded = transactions.filter(t => t.source === 'company' && t.direction === 'expense' && !t.uploaded)
  const companyUploadedNotReimbursed = transactions.filter(t => t.source === 'company' && t.direction === 'expense' && t.uploaded && !t.reimbursed)

  if (loading) {
    return (
      <div className="space-y-6">
        <CardSkeleton className="h-28" />
        <div className="grid grid-cols-2 gap-3">
          <CardSkeleton /><CardSkeleton /><CardSkeleton /><CardSkeleton />
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="bg-rose-50 border border-rose-200 text-rose-700 rounded-xl p-6 text-sm">
        <p className="font-semibold mb-1">加载失败</p>
        <p>{error}</p>
      </div>
    )
  }

  return (
    <div className="space-y-6">
            {/* Hero Header — Premium gradient */}
      <div className="relative overflow-hidden bg-gradient-to-br from-violet-600 via-purple-600 to-fuchsia-500 rounded-2xl p-6 md:p-7 text-white shadow-lg shadow-violet-500/20">
        <div className="absolute -top-8 -right-8 w-32 h-32 bg-white/10 rounded-full blur-2xl" />
        <div className="absolute -bottom-6 -left-6 w-24 h-24 bg-fuchsia-400/20 rounded-full blur-2xl" />
        <BrandWatermark className="absolute -bottom-2 right-4 opacity-[0.08]" opacity={0.12} />
        <div className="relative flex items-start justify-between gap-3">
          <div className="min-w-0">
            <p className="text-white/70 text-xs font-medium mb-1">{new Date().toLocaleDateString('zh-CN', { year: 'numeric', month: 'long', day: 'numeric' })}</p>
            <h1 className="text-2xl md:text-3xl font-bold tracking-tight truncate">{greeting.timeGreeting}，{user?.nickname || user?.username || user?.email?.split('@')[0]}</h1>
            <p className="text-white/60 text-xs mt-1">{greeting.subMessage}</p>
            <div className="flex items-center gap-2 mt-2 flex-wrap">
              {ratesLoading
                ? <span className="text-[10px] px-2 py-0.5 rounded-full bg-white/15 text-white/70 backdrop-blur-sm">汇率加载中…</span>
                : rateDate
                  ? <span className="text-[10px] px-2 py-0.5 rounded-full bg-white/15 text-white/90 backdrop-blur-sm font-medium">💱 $ {rates.USD?.toFixed(2)} · € {rates.EUR?.toFixed(2)} · {rateDate}</span>
                  : <span className="text-[10px] px-2 py-0.5 rounded-full bg-amber-400/20 text-amber-200 backdrop-blur-sm font-medium">备用汇率 · $ {rates.USD?.toFixed(2)} · € {rates.EUR?.toFixed(2)}</span>
              }
            </div>
          </div>
          <Link
            to="/add"
            className="shrink-0 bg-white/20 hover:bg-white/30 active:scale-95 backdrop-blur-sm text-white text-sm font-semibold px-4 py-2.5 rounded-xl transition-all border border-white/20"
          >
            + 添加
          </Link>
        </div>
      </div>

      {/* Balance cards — 2×2 grid — Premium */}
      <StaggerContainer className="grid grid-cols-2 gap-3">
        {/* 公共账户余额 */}
        <StaggerItem>
        <div className="bg-white rounded-2xl border border-gray-100/80 p-5 shadow-sm hover:shadow-md transition-shadow">
          <div className="flex items-center gap-2 mb-3">
            <div className="w-8 h-8 rounded-xl bg-emerald-50 flex items-center justify-center shrink-0">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" className="w-4.5 h-4.5 text-emerald-600"><rect x="2" y="7" width="20" height="14" rx="2"/><path d="M16 7V5a2 2 0 00-4 0v2"/><line x1="12" y1="12" x2="12" y2="16"/><line x1="10" y1="14" x2="14" y2="14"/></svg>
            </div>
            <p className="text-xs font-semibold text-gray-400 uppercase tracking-wider">公共账户</p>
          </div>
          <p className="text-xl md:text-2xl font-bold text-gray-800 leading-tight tabular-nums whitespace-nowrap truncate">
            <CompactAmount compact={fmtCompact(companyBalance)} exact={fmtExact(companyBalance)} />
          </p>
          <p className="text-[11px] text-gray-400 mt-1.5">当前可用资金</p>
        </div>
        </StaggerItem>
        {/* 个人账户余额 */}
        <StaggerItem>
        <div className="bg-white rounded-2xl border border-gray-100/80 p-5 shadow-sm hover:shadow-md transition-shadow">
          <div className="flex items-center gap-2 mb-3">
            <div className="w-8 h-8 rounded-xl bg-violet-50 flex items-center justify-center shrink-0">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" className="w-4.5 h-4.5 text-violet-600"><path d="M20 21v-2a4 4 0 00-4-4H8a4 4 0 00-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>
            </div>
            <p className="text-xs font-semibold text-gray-400 uppercase tracking-wider">个人账户</p>
          </div>
          <p className="text-xl md:text-2xl font-bold text-gray-800 leading-tight tabular-nums whitespace-nowrap truncate">
            <CompactAmount compact={fmtCompact(personalBalance)} exact={fmtExact(personalBalance)} />
          </p>
          <p className="text-[11px] text-gray-400 mt-1.5">个人账户余额</p>
        </div>
        </StaggerItem>
        {/* 个人待报销 */}
        <StaggerItem>
        <div className="bg-white rounded-2xl border border-gray-100/80 p-5 shadow-sm hover:shadow-md transition-shadow">
          <div className="flex items-center gap-2 mb-3">
            <div className="w-8 h-8 rounded-xl bg-amber-50 flex items-center justify-center shrink-0">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" className="w-4.5 h-4.5 text-amber-600"><circle cx="12" cy="12" r="10"/><path d="M9 12l2 2 4-4"/></svg>
            </div>
            <p className="text-xs font-semibold text-gray-400 uppercase tracking-wider">个人待报销</p>
          </div>
          <p className="text-xl md:text-2xl font-bold text-gray-800 leading-tight tabular-nums whitespace-nowrap truncate">
            <CompactAmount compact={fmtCompact(personalOutstanding)} exact={fmtExact(personalOutstanding)} />
          </p>
          <p className="text-[11px] text-gray-400 mt-1.5">个人垫付未报销</p>
        </div>
        </StaggerItem>
        {/* 公共待报销 */}
        <StaggerItem>
        <div className="bg-white rounded-2xl border border-gray-100/80 p-5 shadow-sm hover:shadow-md transition-shadow">
          <div className="flex items-center gap-2 mb-3">
            <div className="w-8 h-8 rounded-xl bg-rose-50 flex items-center justify-center shrink-0">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" className="w-4.5 h-4.5 text-rose-500"><path d="M12 2v20M17 5H9.5a3.5 3.5 0 100 7h5a3.5 3.5 0 110 7H6"/></svg>
            </div>
            <p className="text-xs font-semibold text-gray-400 uppercase tracking-wider">公共待报销</p>
          </div>
          <p className="text-xl md:text-2xl font-bold text-gray-800 leading-tight tabular-nums whitespace-nowrap truncate">
            <CompactAmount compact={fmtCompact(companyOutstanding)} exact={fmtExact(companyOutstanding)} />
          </p>
          <p className="text-[11px] text-gray-400 mt-1.5">公共支出未结算</p>
        </div>
        </StaggerItem>
      </StaggerContainer>

      {/* Pending action hints — Premium */}
      {(notUploaded.length > 0 || uploadedNotReimbursed.length > 0 || companyNotUploaded.length > 0 || companyUploadedNotReimbursed.length > 0) && (
        <div className="bg-white rounded-2xl border border-gray-100/80 p-5 shadow-sm hover:shadow-md transition-shadow">
          <h2 className="text-xs font-semibold text-gray-400 mb-3 uppercase tracking-wider">待处理事项</h2>
          <div className="space-y-2">
            {notUploaded.length > 0 && (
              <Link to="/transactions" className="flex items-center gap-3 p-3 rounded-xl bg-amber-50 border border-amber-100 hover:border-amber-300 transition-colors">
                <span className="w-8 h-8 rounded-lg bg-amber-100 text-amber-600 flex items-center justify-center shrink-0"><IconUpload /></span>
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium text-amber-800">有 {notUploaded.length} 笔垫付尚未上传系统</p>
                  <p className="text-xs text-amber-600 mt-0.5">上传后才可标记报销 → 点击前往交易明细</p>
                </div>
                <svg className="w-4 h-4 text-amber-400 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" /></svg>
              </Link>
            )}
            {uploadedNotReimbursed.length > 0 && (
              <Link to="/match" className="flex items-center gap-3 p-3 rounded-xl bg-violet-50 border border-violet-100 hover:border-violet-300 transition-colors">
                <span className="w-8 h-8 rounded-lg bg-violet-100 text-violet-600 flex items-center justify-center shrink-0"><IconSearch /></span>
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium text-violet-800">有 {uploadedNotReimbursed.length} 笔已上传等待报销</p>
                  <p className="text-xs text-violet-600 mt-0.5">使用子集匹配找到报销组合 → 点击前往匹配</p>
                </div>
                <svg className="w-4 h-4 text-violet-400 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" /></svg>
              </Link>
            )}
            {companyNotUploaded.length > 0 && (
              <Link to="/transactions" className="flex items-center gap-3 p-3 rounded-xl bg-slate-50 border border-slate-200 hover:border-violet-300 transition-colors">
                <span className="w-8 h-8 rounded-lg bg-slate-100 text-slate-600 flex items-center justify-center shrink-0"><IconUpload /></span>
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium text-slate-700">有 {companyNotUploaded.length} 笔公共支出未上传</p>
                  <p className="text-xs text-slate-500 mt-0.5">公共支出记录上传后方可标记报销 → 点击前往明细</p>
                </div>
                <svg className="w-4 h-4 text-slate-400 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" /></svg>
              </Link>
            )}
            {companyUploadedNotReimbursed.length > 0 && (
              <Link to="/match" className="flex items-center gap-3 p-3 rounded-xl bg-emerald-50 border border-emerald-100 hover:border-emerald-300 transition-colors">
                <span className="w-8 h-8 rounded-lg bg-emerald-100 text-emerald-600 flex items-center justify-center shrink-0"><IconSearch /></span>
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium text-emerald-800">有 {companyUploadedNotReimbursed.length} 笔公共支出已上传待报销</p>
                  <p className="text-xs text-emerald-600 mt-0.5">公共支出已提交，等待报销结算 → 点击前往匹配</p>
                </div>
                <svg className="w-4 h-4 text-emerald-400 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" /></svg>
              </Link>
            )}
          </div>
        </div>
      )}

      {/* Feature guide — Premium: clean, flat */}
      <div className="bg-white rounded-2xl border border-gray-100/80 p-5 shadow-sm hover:shadow-md transition-shadow">
        <h2 className="text-xs font-semibold text-gray-400 mb-4 uppercase tracking-wider">功能导航</h2>
        <div className="grid grid-cols-2 gap-3">
          {FEATURES.map((f) => (
            <AnimatedCard
              key={f.to}
              className="rounded-2xl"
            >
            <Link
              to={f.to}
              className="flex items-start gap-3 p-4 rounded-2xl border border-gray-100/80 transition-all hover:border-violet-200 hover:shadow-md hover:shadow-violet-100/30 group bg-white"
            >
              <span className={`w-9 h-9 rounded-xl flex items-center justify-center shrink-0 ${f.iconBg} transition-transform group-hover:scale-105`}>
                <f.Icon />
              </span>
              <div className="min-w-0">
                <p className="text-sm font-semibold text-gray-700">{f.title}</p>
                <p className="text-xs text-gray-400 mt-0.5 leading-relaxed">{f.desc}</p>
              </div>
            </Link>
            </AnimatedCard>
          ))}
        </div>
      </div>

      {/* Workflow guide — Premium */}
      <div className="bg-white rounded-2xl border border-gray-100/80 p-5 shadow-sm hover:shadow-md transition-shadow">
        <h2 className="text-xs font-semibold text-gray-400 mb-4 uppercase tracking-wider">使用流程</h2>
        <div className="space-y-4">
          {/* Personal advance flow */}
          <div>
            <div className="flex items-center gap-1.5 mb-2.5">
              <span className="w-2 h-2 rounded-full bg-amber-400 shrink-0" />
              <p className="text-[11px] font-semibold text-amber-600 uppercase tracking-wider">个人垫付</p>
            </div>
            <div className="grid grid-cols-2 gap-2 md:flex md:items-start md:gap-0">
              {([
                { step: '1', Icon: IconPen,    title: '记录垫付', desc: '添加个人垫付支出，填写类别和项目' },
                { step: '2', Icon: IconUpload, title: '标记上传', desc: '在明细中标记已上传到报销系统' },
                { step: '3', Icon: IconSearch, title: '子集匹配', desc: '输入报销单总额，自动找到对应组合' },
                { step: '4', Icon: IconCheck,  title: '完成报销', desc: '确认报销完成，标记相关交易已报销' },
              ] as const).map((s, i, arr) => (
                <div key={s.step} className="bg-amber-50/60 rounded-xl p-2.5 flex items-start gap-2 md:flex-col md:items-center md:flex-1 md:bg-transparent md:p-0">
                  <div className="w-5 h-5 rounded-full bg-amber-500 text-white text-[10px] font-bold flex items-center justify-center shrink-0 md:w-7 md:h-7">{s.step}</div>
                  <div className="md:text-center md:mt-1.5 md:px-1">
                    <div className="text-amber-500 mb-0.5 md:flex md:justify-center"><s.Icon /></div>
                    <p className="text-xs font-semibold text-gray-700">{s.title}</p>
                    <p className="text-[10px] text-gray-400 mt-0.5 leading-relaxed">{s.desc}</p>
                  </div>
                  {i < arr.length - 1 && <div className="hidden md:block flex-1 h-px bg-gray-200 mt-3.5 mx-1" />}
                </div>
              ))}
            </div>
          </div>

          <div className="border-t border-gray-100" />

          {/* Company account flow */}
          <div>
            <div className="flex items-center gap-1.5 mb-2.5">
              <span className="w-2 h-2 rounded-full bg-sky-500 shrink-0" />
              <p className="text-[11px] font-semibold text-sky-600 uppercase tracking-wider">公共账户</p>
            </div>
            <div className="grid grid-cols-3 gap-2 md:flex md:items-start md:gap-0">
              {([
                { step: '1', Icon: IconPen,    title: '设置账户', desc: '在设置页创建公共资金账户' },
                { step: '2', Icon: IconUpload, title: '记录收支', desc: '添加公共账户的收入与支出' },
                { step: '3', Icon: IconCheck,  title: '查看汇总', desc: '总览实时显示账户余额与待结算金额' },
              ] as const).map((s, i, arr) => (
                <div key={s.step} className="bg-sky-50/60 rounded-xl p-2.5 flex items-start gap-2 md:flex-col md:items-center md:flex-1 md:bg-transparent md:p-0">
                  <div className="w-5 h-5 rounded-full bg-sky-600 text-white text-[10px] font-bold flex items-center justify-center shrink-0 md:w-7 md:h-7">{s.step}</div>
                  <div className="md:text-center md:mt-1.5 md:px-1">
                    <div className="text-sky-500 mb-0.5 md:flex md:justify-center"><s.Icon /></div>
                    <p className="text-xs font-semibold text-gray-700">{s.title}</p>
                    <p className="text-[10px] text-gray-400 mt-0.5 leading-relaxed">{s.desc}</p>
                  </div>
                  {i < arr.length - 1 && <div className="hidden md:block flex-1 h-px bg-gray-200 mt-3.5 mx-1" />}
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
