import { NavLink } from 'react-router-dom'
import { useAuth } from '../contexts/AuthContext'

// SVG icon components
const IconHome = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-[18px] h-[18px]">
    <path d="M3 9.5L12 3l9 6.5V20a1 1 0 01-1 1H4a1 1 0 01-1-1V9.5z" />
    <path d="M9 21V12h6v9" />
  </svg>
)
const IconList = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-[18px] h-[18px]">
    <path d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01" />
  </svg>
)
const IconPlus = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.5} strokeLinecap="round" className="w-6 h-6">
    <path d="M12 5v14M5 12h14" />
  </svg>
)
const IconMatch = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-[18px] h-[18px]">
    <circle cx="11" cy="11" r="8" />
    <path d="M21 21l-4.35-4.35" />
    <path d="M8 11h6M11 8v6" />
  </svg>
)
const IconChart = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-[18px] h-[18px]">
    <path d="M18 20V10M12 20V4M6 20v-6" />
  </svg>
)
const IconSettings = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-[18px] h-[18px]">
    <circle cx="12" cy="12" r="3" />
    <path d="M19.4 15a1.65 1.65 0 00.33 1.82l.06.06a2 2 0 010 2.83 2 2 0 01-2.83 0l-.06-.06a1.65 1.65 0 00-1.82-.33 1.65 1.65 0 00-1 1.51V21a2 2 0 01-4 0v-.09A1.65 1.65 0 009 19.4a1.65 1.65 0 00-1.82.33l-.06.06a2 2 0 01-2.83-2.83l.06-.06A1.65 1.65 0 004.68 15a1.65 1.65 0 00-1.51-1H3a2 2 0 010-4h.09A1.65 1.65 0 004.6 9a1.65 1.65 0 00-.33-1.82l-.06-.06a2 2 0 012.83-2.83l.06.06A1.65 1.65 0 009 4.68a1.65 1.65 0 001-1.51V3a2 2 0 014 0v.09a1.65 1.65 0 001 1.51 1.65 1.65 0 001.82-.33l.06-.06a2 2 0 012.83 2.83l-.06.06A1.65 1.65 0 0019.4 9a1.65 1.65 0 001.51 1H21a2 2 0 010 4h-.09a1.65 1.65 0 00-1.51 1z" />
  </svg>
)
const IconLogout = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-4 h-4">
    <path d="M9 21H5a2 2 0 01-2-2V5a2 2 0 012-2h4" />
    <polyline points="16 17 21 12 16 7" />
    <line x1="21" y1="12" x2="9" y2="12" />
  </svg>
)

const navItems = [
  { to: '/', label: '概览', Icon: IconHome, end: true },
  { to: '/transactions', label: '明细', Icon: IconList },
  { to: '/add', label: '添加', Icon: IconPlus, isPrimary: true },
  { to: '/match', label: '匹配', Icon: IconMatch },
  { to: '/stats', label: '统计', Icon: IconChart },
  { to: '/settings', label: '设置', Icon: IconSettings },
]

export default function Layout({ children }: { children: React.ReactNode }) {
  const { user, logout } = useAuth()
  const initial = ((user?.username || user?.email || '?')[0]).toUpperCase()
  const displayName = user?.username || user?.email || '—'

  return (
    <div className="flex bg-slate-50" style={{ height: '100dvh' }}>

      {/* ── Desktop Sidebar ── */}
      <aside className="hidden md:flex w-[220px] bg-white border-r border-gray-100/80 flex-col shrink-0">
        {/* Brand */}
        <div className="px-5 pt-6 pb-5">
          <div className="flex items-center gap-3">
            <img src="/logo.svg" alt="FinArch" className="w-9 h-9 rounded-xl shrink-0" />
            <div>
              <h1 className="font-extrabold text-gray-900 text-base leading-tight tracking-tight">FinArch</h1>
              <p className="text-[10px] text-gray-400 mt-0.5 tracking-wide">收支 · 报销 · 统计</p>
            </div>
          </div>
        </div>

        {/* Nav — Premium: refined active state with left accent */}
        <nav className="flex-1 px-3 py-2 space-y-0.5">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) =>
                `flex items-center gap-3 px-3 py-2.5 text-[13px] font-medium rounded-xl transition-all duration-150 ${
                  isActive
                    ? 'bg-violet-50 text-violet-700 shadow-sm shadow-violet-100/50'
                    : 'text-gray-500 hover:bg-gray-50 hover:text-gray-800'
                }`
              }
            >
              <item.Icon />
              {item.label}
            </NavLink>
          ))}
        </nav>

        {/* User section — Premium: gradient avatar */}
        <div className="px-3 pb-5 pt-3 border-t border-gray-100/80 mt-auto">
          <div className="flex items-center gap-2.5 px-3 py-2 rounded-xl hover:bg-gray-50 transition-colors group">
            <div className="w-8 h-8 rounded-full bg-gradient-to-br from-violet-500 to-fuchsia-500 text-white flex items-center justify-center text-xs font-bold shrink-0 shadow-sm shadow-violet-200/50">
              {initial}
            </div>
            <p className="flex-1 text-[12px] text-gray-700 truncate font-medium">{displayName}</p>
            <button
              onClick={logout}
              title="退出登录"
              className="opacity-0 group-hover:opacity-100 text-gray-300 hover:text-rose-400 transition-all p-1"
            >
              <IconLogout />
            </button>
          </div>
        </div>
      </aside>

      {/* ── Mobile Top Header ── */}
      <header className="gpu-layer md:hidden fixed top-0 left-0 right-0 z-50 h-14 bg-white/95 backdrop-blur-sm border-b border-gray-100/80 flex items-center justify-between px-4">
        <div className="flex items-center gap-2.5">
          <img src="/logo.svg" alt="FinArch" className="w-7 h-7 rounded-xl shrink-0" />
          <span className="font-extrabold text-gray-900 text-[15px] tracking-tight">FinArch</span>
        </div>
        <div className="flex items-center gap-2.5">
          <div className="w-7 h-7 rounded-full bg-gradient-to-br from-violet-500 to-fuchsia-500 text-white flex items-center justify-center text-[11px] font-bold shadow-sm shadow-violet-200/50">
            {initial}
          </div>
          <button
            onClick={logout}
            className="text-xs text-gray-400 hover:text-rose-500 transition-colors px-2 py-1 rounded-lg hover:bg-rose-50"
          >
            退出
          </button>
        </div>
      </header>

      {/* ── Main Content ── */}
      <main className="scroll-main flex-1 overflow-y-scroll pt-14 md:pt-0 md:pb-0 flex flex-col" style={{ backgroundColor: '#FAFAF9' }}>
        <div className="flex-1 max-w-4xl w-full mx-auto px-4 py-6 md:px-8 md:py-8 page-enter">
          {children}
        </div>

        {/* ── Footer — Premium: refined ── */}
        <footer className="shrink-0 mt-auto">
          <div className="h-px bg-gradient-to-r from-transparent via-gray-200/60 to-transparent mx-6 md:mx-8" />
          <div className="max-w-4xl mx-auto px-4 md:px-8 py-4 flex items-center justify-between gap-4">
            <div className="flex items-center gap-2">
              <span className="text-[11px] font-semibold text-gray-300 tracking-wide">FinArch</span>
              <span className="text-[11px] text-gray-200">·</span>
              <span className="text-[11px] text-gray-300">记账 · 报销 · 智能匹配</span>
            </div>
            <span className="text-[10px] text-gray-300 font-mono">v2.2</span>
          </div>
        </footer>
      </main>

      {/* ── Mobile Bottom Navigation ── */}
      <nav className="gpu-layer safe-bottom md:hidden fixed bottom-0 left-0 right-0 z-50 bg-white/95 backdrop-blur-sm border-t border-gray-100/80 flex items-end">
        {navItems.map((item) => {
          if (item.isPrimary) {
            return (
              <NavLink
                key={item.to}
                to={item.to}
                className="flex-1 flex flex-col items-center pb-2 pt-1 -mt-5"
              >
                <div className="w-12 h-12 rounded-2xl bg-gradient-to-br from-violet-600 to-purple-600 text-white flex items-center justify-center shadow-lg shadow-violet-400/40 active:scale-95 transition-transform">
                  <item.Icon />
                </div>
                <span className="text-[10px] font-medium text-gray-400 mt-0.5">{item.label}</span>
              </NavLink>
            )
          }
          return (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) =>
                `flex-1 flex flex-col items-center justify-center py-2 gap-0.5 transition-colors ${
                  isActive ? 'text-violet-600' : 'text-gray-400 active:text-gray-500'
                }`
              }
            >
              {({ isActive }) => (
                <>
                  <div className={`p-1.5 rounded-xl transition-colors ${isActive ? 'bg-violet-50' : ''}`}>
                    <item.Icon />
                  </div>
                  <span className={`text-[10px] font-medium ${isActive ? 'text-violet-600' : ''}`}>{item.label}</span>
                </>
              )}
            </NavLink>
          )
        })}
      </nav>
    </div>
  )
}
