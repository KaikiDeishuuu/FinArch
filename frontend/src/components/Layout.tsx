import { NavLink } from 'react-router-dom'
import { useAuth } from '../contexts/AuthContext'

const navItems = [
  { to: '/', label: '概览', icon: '📊', end: true },
  { to: '/transactions', label: '明细', icon: '📋' },
  { to: '/add', label: '添加', icon: '➕' },
  { to: '/match', label: '匹配', icon: '🔍' },
  { to: '/stats', label: '统计', icon: '📈' },
]

export default function Layout({ children }: { children: React.ReactNode }) {
  const { user, logout } = useAuth()

  return (
    <div className="flex h-screen bg-gray-50">
      {/* ── Desktop Sidebar ── */}
      <aside className="hidden md:flex w-60 bg-white border-r border-gray-100 flex-col shrink-0 shadow-sm">
        <div className="px-5 py-5 border-b border-gray-100">
          <div className="flex items-center gap-2.5">
            <div className="w-8 h-8 rounded-lg bg-blue-600 flex items-center justify-center shrink-0 shadow-sm">
              <span className="text-white text-sm font-bold">¥</span>
            </div>
            <div>
              <h1 className="font-bold text-gray-800 text-sm leading-tight">科研经费管理系统</h1>
              <p className="text-[11px] text-gray-400 mt-0.5">FinArch v2</p>
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
              <span className="text-base">{item.icon}</span>
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
      <header className="md:hidden fixed top-0 left-0 right-0 z-50 h-14 bg-white border-b border-gray-100 flex items-center justify-between px-4 shadow-sm">
        <div className="flex items-center gap-2">
          <div className="w-7 h-7 rounded-lg bg-blue-600 flex items-center justify-center shadow-sm">
            <span className="text-white text-xs font-bold">¥</span>
          </div>
          <span className="font-bold text-gray-800 text-sm">科研经费管理系统</span>
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
      <main className="flex-1 overflow-y-auto pt-14 pb-20 md:pt-0 md:pb-0">
        <div className="max-w-5xl mx-auto px-4 py-4 md:px-6 md:py-8">
          {children}
        </div>
      </main>

      {/* ── Mobile Bottom Navigation ── */}
      <nav className="md:hidden fixed bottom-0 left-0 right-0 z-50 bg-white border-t border-gray-100 flex shadow-lg safe-area-inset-bottom">
        {navItems.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.end}
            className={({ isActive }) =>
              `flex-1 flex flex-col items-center justify-center py-2 gap-0.5 transition-colors ${
                isActive ? 'text-blue-600' : 'text-gray-400 active:text-gray-600'
              }`
            }
          >
            <span className="text-xl leading-none">{item.icon}</span>
            <span className="text-[10px] font-medium">{item.label}</span>
          </NavLink>
        ))}
      </nav>
    </div>
  )
}
