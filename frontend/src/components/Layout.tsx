import { NavLink } from 'react-router-dom'
import { useAuth } from '../contexts/AuthContext'

// SVG icon components
const IconHome = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-5 h-5">
    <path d="M3 9.5L12 3l9 6.5V20a1 1 0 01-1 1H4a1 1 0 01-1-1V9.5z" />
    <path d="M9 21V12h6v9" />
  </svg>
)
const IconList = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-5 h-5">
    <path d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01" />
  </svg>
)
const IconPlus = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.5} strokeLinecap="round" className="w-6 h-6">
    <path d="M12 5v14M5 12h14" />
  </svg>
)
const IconMatch = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-5 h-5">
    <circle cx="11" cy="11" r="8" />
    <path d="M21 21l-4.35-4.35" />
    <path d="M8 11h6M11 8v6" />
  </svg>
)
const IconChart = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-5 h-5">
    <path d="M18 20V10M12 20V4M6 20v-6" />
  </svg>
)
const IconSettings = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-5 h-5">
    <circle cx="12" cy="12" r="3" />
    <path d="M19.4 15a1.65 1.65 0 00.33 1.82l.06.06a2 2 0 010 2.83 2 2 0 01-2.83 0l-.06-.06a1.65 1.65 0 00-1.82-.33 1.65 1.65 0 00-1 1.51V21a2 2 0 01-4 0v-.09A1.65 1.65 0 009 19.4a1.65 1.65 0 00-1.82.33l-.06.06a2 2 0 01-2.83-2.83l.06-.06A1.65 1.65 0 004.68 15a1.65 1.65 0 00-1.51-1H3a2 2 0 010-4h.09A1.65 1.65 0 004.6 9a1.65 1.65 0 00-.33-1.82l-.06-.06a2 2 0 012.83-2.83l.06.06A1.65 1.65 0 009 4.68a1.65 1.65 0 001-1.51V3a2 2 0 014 0v.09a1.65 1.65 0 001 1.51 1.65 1.65 0 001.82-.33l.06-.06a2 2 0 012.83 2.83l-.06.06A1.65 1.65 0 0019.4 9a1.65 1.65 0 001.51 1H21a2 2 0 010 4h-.09a1.65 1.65 0 00-1.51 1z" />
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

  return (
    <div className="flex bg-gray-50" style={{ height: '100dvh' }}>
      {/* ── Desktop Sidebar ── */}
      <aside className="hidden md:flex w-60 bg-white border-r border-gray-100 flex-col shrink-0 shadow-sm">
        <div className="px-5 py-5 border-b border-gray-100">
          <div className="flex items-center gap-2.5">
            <div className="w-8 h-8 rounded-lg bg-blue-600 flex items-center justify-center shrink-0 shadow-sm">
              <span className="text-white text-sm font-bold">¥</span>
            </div>
            <div>
              <h1 className="font-bold text-gray-800 text-sm leading-tight">FinArch</h1>
              <p className="text-[11px] text-gray-400 mt-0.5">收支 · 报销 · 统计</p>
            </div>
          </div>
        </div>
        <nav className="flex-1 py-3">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) =>
                `flex items-center gap-2.5 px-4 py-2.5 text-sm font-medium transition-all mx-2 rounded-lg ${
                  isActive
                    ? 'bg-blue-600 text-white shadow-sm'
                    : 'text-gray-500 hover:bg-gray-100 hover:text-gray-800'
                }`
              }
            >
              <item.Icon />
              {item.label}
            </NavLink>
          ))}
        </nav>
        <div className="px-4 py-4 border-t border-gray-100">
          <div className="flex items-center gap-2.5 mb-2.5">
            <div className="w-7 h-7 rounded-full bg-blue-100 text-blue-600 flex items-center justify-center text-xs font-bold shrink-0">
              {((user?.name || user?.email || '?')[0]).toUpperCase()}
            </div>
            <p className="text-xs text-gray-700 truncate font-medium">{user?.name || user?.email}</p>
          </div>
          <button
            onClick={logout}
            className="w-full text-xs text-gray-400 hover:text-red-500 transition-colors text-left pl-9"
          >
            退出登录
          </button>
        </div>
      </aside>

      {/* ── Mobile Top Header ── */}
      <header className="gpu-layer md:hidden fixed top-0 left-0 right-0 z-50 h-14 bg-white border-b border-gray-100 flex items-center justify-between px-4 shadow-sm">
        <div className="flex items-center gap-2">
          <div className="w-7 h-7 rounded-lg bg-blue-600 flex items-center justify-center shadow-sm">
            <span className="text-white text-xs font-bold">¥</span>
          </div>
          <span className="font-bold text-gray-800 text-sm">FinArch</span>
        </div>
        <div className="flex items-center gap-3">
          <div className="w-7 h-7 rounded-full bg-blue-100 text-blue-600 flex items-center justify-center text-xs font-bold">
            {((user?.name || user?.email || '?')[0]).toUpperCase()}
          </div>
          <button onClick={logout} className="text-xs text-gray-400 hover:text-red-500 transition-colors">
            退出
          </button>
        </div>
      </header>

      {/* ── Main Content ── */}
      {/* scroll-main: iOS 动量滚动 + overscroll-contain 防橡皮筋上传 */}
      <main className="scroll-main flex-1 overflow-y-scroll pt-14 md:pt-0 md:pb-0">
        <div className="max-w-5xl mx-auto px-4 py-4 md:px-6 md:py-8">
          {children}
        </div>
        {/* ── Footer（仅桌面）── */}
        <footer className="hidden md:block border-t border-gray-100 bg-white">
          <div className="max-w-5xl mx-auto px-6 py-3 flex items-center justify-between">
            <span className="text-[11px] text-gray-400">
              &copy; {new Date().getFullYear()} FinArch — 收支与报销管理
            </span>
            <span className="text-[11px] text-gray-300">
              v2.0 &nbsp;·&nbsp; Powered by Go + React
            </span>
          </div>
        </footer>
      </main>

      {/* ── Mobile Bottom Navigation ── */}
      <nav className="gpu-layer safe-bottom md:hidden fixed bottom-0 left-0 right-0 z-50 bg-white/90 backdrop-blur border-t border-gray-100 flex items-end shadow-[0_-1px_8px_rgba(0,0,0,0.06)]">
        {navItems.map((item) => {
          if (item.isPrimary) {
            return (
              <NavLink
                key={item.to}
                to={item.to}
                className="flex-1 flex flex-col items-center pb-2 pt-1 -mt-4"
              >
                <div className="w-12 h-12 rounded-full bg-blue-600 text-white flex items-center justify-center shadow-lg shadow-blue-200 active:scale-95 transition-transform">
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
                  isActive ? 'text-blue-600' : 'text-gray-400 active:text-gray-500'
                }`
              }
            >
              {({ isActive }) => (
                <>
                  <item.Icon />
                  <span className={`text-[10px] font-medium ${isActive ? 'text-blue-600' : ''}`}>{item.label}</span>
                </>
              )}
            </NavLink>
          )
        })}
      </nav>
    </div>
  )
}
