import { NavLink, useLocation } from 'react-router-dom'
import { useAuth } from '../contexts/AuthContext'
import { useTheme } from '../contexts/ThemeContext'
import { useTranslation } from 'react-i18next'
import { PageTransition } from '../motion'
import { useMode } from '../contexts/ModeContext'
import { LogoMark, LogoBars, BrandDivider } from './Brand'

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

const NAV_ITEMS = [
  { to: '/', labelKey: 'nav.dashboard', Icon: IconHome, end: true },
  { to: '/transactions', labelKey: 'nav.transactions', Icon: IconList },
  { to: '/add', labelKey: 'nav.add', Icon: IconPlus, isPrimary: true },
  { to: '/match', labelKey: 'nav.match', Icon: IconMatch },
  { to: '/stats', labelKey: 'nav.stats', Icon: IconChart },
  { to: '/settings', labelKey: 'nav.settings', Icon: IconSettings },
]

// ── Theme toggle icons ──
const IconSun = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-4 h-4">
    <circle cx="12" cy="12" r="5" /><line x1="12" y1="1" x2="12" y2="3" /><line x1="12" y1="21" x2="12" y2="23" /><line x1="4.22" y1="4.22" x2="5.64" y2="5.64" /><line x1="18.36" y1="18.36" x2="19.78" y2="19.78" /><line x1="1" y1="12" x2="3" y2="12" /><line x1="21" y1="12" x2="23" y2="12" /><line x1="4.22" y1="19.78" x2="5.64" y2="18.36" /><line x1="18.36" y1="5.64" x2="19.78" y2="4.22" />
  </svg>
)
const IconMoon = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-4 h-4">
    <path d="M21 12.79A9 9 0 1111.21 3 7 7 0 0021 12.79z" />
  </svg>
)
const IconLang = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-4 h-4">
    <circle cx="12" cy="12" r="10" /><line x1="2" y1="12" x2="22" y2="12" /><path d="M12 2a15.3 15.3 0 014 10 15.3 15.3 0 01-4 10 15.3 15.3 0 01-4-10 15.3 15.3 0 014-10z" />
  </svg>
)

export default function Layout({ children }: { children: React.ReactNode }) {
  const { user, logout } = useAuth()
  const { resolved, toggle: toggleTheme } = useTheme()
  const { t, i18n } = useTranslation()
  const location = useLocation()
  const initial = ((user?.username || user?.email || '?')[0]).toUpperCase()
  const displayName = user?.username || user?.email || '—'
  const toggleLang = () => i18n.changeLanguage(i18n.language === 'zh' ? 'en' : 'zh')
  const isDark = resolved === 'dark'
  const { mode, setMode, isWorkMode } = useMode()
  const navItems = NAV_ITEMS.filter((item) => isWorkMode || item.to !== '/match')

  return (
    <div className="flex bg-slate-50 dark:bg-[hsl(260,20%,6%)] overflow-x-hidden" style={{ height: '100dvh' }}>

      {/* ── Desktop Sidebar ── */}
      <aside className="hidden md:flex w-[220px] bg-white dark:bg-[hsl(260,15%,11%)] border-r border-gray-100/80 dark:border-gray-800/60 flex-col shrink-0">
        {/* Brand */}
        <div className="px-5 pt-6 pb-5">
          <div className="flex items-center gap-3">
            <LogoMark size={36} className="rounded-xl" />
            <div>
              <h1 className="font-extrabold text-gray-900 dark:text-gray-100 text-base leading-tight tracking-tight">FinArch</h1>
              <p className="text-[10px] text-gray-400 dark:text-gray-500 mt-0.5 tracking-wide">{t('nav.subtitle')}</p>
            </div>
          </div>
        </div>


        <div className="px-5 pb-2">
          <div className="grid grid-cols-2 rounded-xl bg-gray-100 dark:bg-gray-800 p-1">
            <button onClick={() => setMode('work')} className={`px-2 py-1.5 text-xs font-semibold rounded-lg transition ${mode === 'work' ? 'bg-white dark:bg-gray-700 text-violet-600 dark:text-violet-300' : 'text-gray-500 dark:text-gray-400'}`}>Work</button>
            <button onClick={() => setMode('life')} className={`px-2 py-1.5 text-xs font-semibold rounded-lg transition ${mode === 'life' ? 'bg-white dark:bg-gray-700 text-violet-600 dark:text-violet-300' : 'text-gray-500 dark:text-gray-400'}`}>Life</button>
          </div>
        </div>
        {/* Nav */}
        <nav className="flex-1 px-3 py-2 space-y-0.5">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) =>
                `flex items-center gap-3 px-3 py-2.5 text-[13px] font-medium rounded-xl transition-all duration-150 ${
                  isActive
                    ? 'bg-violet-50 dark:bg-violet-500/15 text-violet-700 dark:text-violet-300 shadow-sm shadow-violet-100/50 dark:shadow-none'
                    : 'text-gray-500 dark:text-gray-400 hover:bg-gray-50 dark:hover:bg-white/5 hover:text-gray-800 dark:hover:text-gray-200'
                }`
              }
            >
              <item.Icon />
              {t(item.labelKey)}
            </NavLink>
          ))}
        </nav>

        {/* Theme + Language toggles */}
        <div className="px-3 pb-2 flex gap-1.5">
          <button
            onClick={toggleTheme}
            className="flex-1 flex items-center justify-center gap-1.5 px-2 py-2 text-[11px] font-medium text-gray-400 dark:text-gray-500 hover:text-violet-600 dark:hover:text-violet-400 hover:bg-gray-50 dark:hover:bg-white/5 rounded-lg transition-colors"
            title={t('theme.toggle')}
          >
            {isDark ? <IconSun /> : <IconMoon />}
          </button>
          <button
            onClick={toggleLang}
            className="flex-1 flex items-center justify-center gap-1.5 px-2 py-2 text-[11px] font-medium text-gray-400 dark:text-gray-500 hover:text-violet-600 dark:hover:text-violet-400 hover:bg-gray-50 dark:hover:bg-white/5 rounded-lg transition-colors"
            title={t('language.toggle')}
          >
            <IconLang />
            <span>{i18n.language === 'zh' ? 'EN' : '中'}</span>
          </button>
        </div>

        {/* User section */}
        <div className="px-3 pb-5 pt-3 border-t border-gray-100/80 dark:border-gray-800/60 mt-auto">
          <div className="flex items-center gap-2.5 px-3 py-2 rounded-xl hover:bg-gray-50 dark:hover:bg-white/5 transition-colors group">
            <div className="w-8 h-8 rounded-full bg-gradient-to-br from-violet-500 to-fuchsia-500 text-white flex items-center justify-center text-xs font-bold shrink-0 shadow-sm shadow-violet-200/50 dark:shadow-violet-900/50">
              {initial}
            </div>
            <p className="flex-1 text-[12px] text-gray-700 dark:text-gray-300 truncate font-medium">{displayName}</p>
            <button
              onClick={logout}
              title={t('nav.logout')}
              className="opacity-0 group-hover:opacity-100 text-gray-300 dark:text-gray-600 hover:text-rose-400 transition-all p-1"
            >
              <IconLogout />
            </button>
          </div>
        </div>
      </aside>

      {/* ── Mobile Top Header ── */}
      <header className="gpu-layer md:hidden fixed top-0 left-0 right-0 z-50 h-14 bg-white/95 dark:bg-[hsl(260,15%,11%)]/95 backdrop-blur-sm border-b border-gray-100/80 dark:border-gray-800/60 flex items-center justify-between px-4">
        <div className="flex items-center gap-2.5">
          <LogoMark size={28} className="rounded-lg" />
          <span className="font-extrabold text-gray-900 dark:text-gray-100 text-[15px] tracking-tight">FinArch</span>
        </div>
        <div className="flex items-center gap-1.5">
          <button onClick={toggleTheme} className="p-1.5 rounded-lg text-gray-400 dark:text-gray-500 hover:text-violet-600 dark:hover:text-violet-400 hover:bg-gray-50 dark:hover:bg-white/5 transition-colors">
            {isDark ? <IconSun /> : <IconMoon />}
          </button>
          <button onClick={toggleLang} className="p-1.5 rounded-lg text-gray-400 dark:text-gray-500 hover:text-violet-600 dark:hover:text-violet-400 hover:bg-gray-50 dark:hover:bg-white/5 transition-colors text-[11px] font-medium">
            {i18n.language === 'zh' ? 'EN' : '中'}
          </button>
          <div className="w-7 h-7 rounded-full bg-gradient-to-br from-violet-500 to-fuchsia-500 text-white flex items-center justify-center text-[11px] font-bold shadow-sm shadow-violet-200/50 dark:shadow-violet-900/50">
            {initial}
          </div>
          <button
            onClick={logout}
            className="text-xs text-gray-400 dark:text-gray-500 hover:text-rose-500 transition-colors px-2 py-1 rounded-lg hover:bg-rose-50 dark:hover:bg-rose-500/10"
          >
            {t('nav.logoutShort')}
          </button>
        </div>
        <div className="absolute left-1/2 -translate-x-1/2 bottom-1">
          <div className="grid grid-cols-2 rounded-lg bg-gray-100 dark:bg-gray-800 p-0.5 w-28">
            <button onClick={() => setMode('work')} className={`text-[10px] py-1 rounded-md ${mode === 'work' ? 'bg-white dark:bg-gray-700 text-violet-600' : 'text-gray-500'}`}>Work</button>
            <button onClick={() => setMode('life')} className={`text-[10px] py-1 rounded-md ${mode === 'life' ? 'bg-white dark:bg-gray-700 text-violet-600' : 'text-gray-500'}`}>Life</button>
          </div>
        </div>
      </header>

      {/* ── Main Content ── */}
      <main className="scroll-main flex-1 overflow-y-scroll overflow-x-hidden pt-14 md:pt-0 md:pb-0 flex flex-col" style={{ backgroundColor: 'hsl(var(--background))' }}>
        <div className="flex-1 max-w-7xl w-full mx-auto px-4 py-6 md:px-8 md:py-8">
          <PageTransition motionKey={location.pathname}>
            {children}
          </PageTransition>
        </div>

        {/* ── Footer ── */}
        <footer className="shrink-0 mt-auto">
          <BrandDivider className="mx-6 md:mx-8" />
          <div className="max-w-7xl mx-auto px-4 md:px-8 py-4 flex items-center justify-between gap-4">
            <div className="flex items-center gap-2.5">
              <LogoBars size={16} opacity={0.25} />
              <span className="text-[11px] font-semibold text-gray-300 dark:text-gray-600 tracking-wide">FinArch</span>
              <span className="text-[11px] text-gray-200 dark:text-gray-700">·</span>
              <span className="text-[11px] text-gray-300 dark:text-gray-600">{t('nav.footer')}</span>
            </div>
            <span className="text-[10px] text-gray-300 dark:text-gray-600 font-mono">v2.3</span>
          </div>
        </footer>
      </main>

      {/* ── Mobile Bottom Navigation ── */}
      <nav className="gpu-layer safe-bottom md:hidden fixed bottom-0 left-0 right-0 z-50 bg-white/95 dark:bg-[hsl(260,15%,11%)]/95 backdrop-blur-sm border-t border-gray-100/80 dark:border-gray-800/60 flex items-end">
        {navItems.map((item) => {
          if (item.isPrimary) {
            return (
              <NavLink
                key={item.to}
                to={item.to}
                className="flex-1 flex flex-col items-center pb-2 pt-1 -mt-5"
              >
                <div className="w-12 h-12 rounded-2xl bg-gradient-to-br from-violet-600 to-purple-600 text-white flex items-center justify-center shadow-lg shadow-violet-400/40 dark:shadow-violet-900/40 active:scale-95 transition-transform">
                  <item.Icon />
                </div>
                <span className="text-[10px] font-medium text-gray-400 dark:text-gray-500 mt-0.5">{t(item.labelKey)}</span>
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
                  isActive ? 'text-violet-600 dark:text-violet-400' : 'text-gray-400 dark:text-gray-500 active:text-gray-500'
                }`
              }
            >
              {({ isActive }) => (
                <>
                  <div className={`p-1.5 rounded-xl transition-colors ${isActive ? 'bg-violet-50 dark:bg-violet-500/15' : ''}`}>
                    <item.Icon />
                  </div>
                  <span className={`text-[10px] font-medium ${isActive ? 'text-violet-600 dark:text-violet-400' : ''}`}>{t(item.labelKey)}</span>
                </>
              )}
            </NavLink>
          )
        })}
      </nav>
    </div>
  )
}
