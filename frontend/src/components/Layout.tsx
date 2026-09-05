import { useState } from 'react'
import { Drawer } from 'vaul'
import { NavLink, useLocation } from 'react-router-dom'
import { useAuth } from '../hooks/useAuth'
import { useTheme } from '../hooks/useTheme'
import { useTranslation } from 'react-i18next'
import { PageTransition } from '../motion'
import { useMode } from '../hooks/useMode'
import { LogoMark, LogoBars, BrandDivider } from './Brand'
import ModeSwitcher from './ModeSwitcher'

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

const IconBudget = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-[18px] h-[18px]">
    <path d="M4 19V5a2 2 0 012-2h12a2 2 0 012 2v14" />
    <path d="M8 7h8M8 11h8M8 15h3" />
    <path d="M16 15h4v4h-4z" />
  </svg>
)

const IconExchange = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-[18px] h-[18px]">
    <path d="M4 7h13" />
    <path d="M13 4l4 3-4 3" />
    <path d="M20 17H7" />
    <path d="M11 14l-4 3 4 3" />
  </svg>
)

const IconRepeat = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-[18px] h-[18px]">
    <path d="M17 1l4 4-4 4" />
    <path d="M3 11V9a4 4 0 014-4h14" />
    <path d="M7 23l-4-4 4-4" />
    <path d="M21 13v2a4 4 0 01-4 4H3" />
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
  { to: '/budgets', labelKey: 'nav.budgets', Icon: IconBudget },
  { to: '/recurring', labelKey: 'nav.recurring', Icon: IconRepeat },
  { to: '/exchange', labelKey: 'nav.exchange', Icon: IconExchange },
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
const IconMore = () => (
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="w-5 h-5">
    <circle cx="5" cy="12" r="1" />
    <circle cx="12" cy="12" r="1" />
    <circle cx="19" cy="12" r="1" />
  </svg>
)

const controlBtnClass = 'fin-control flex items-center justify-center gap-1.5 px-2.5 py-2 text-[hsl(var(--muted-foreground))] hover:text-[hsl(var(--foreground))]'

export default function Layout({ children }: { children: React.ReactNode }) {
  const { user, logout } = useAuth()
  const { resolved, toggle: toggleTheme } = useTheme()
  const { t, i18n } = useTranslation()
  const location = useLocation()
  const initial = ((user?.username || user?.email || '?')[0]).toUpperCase()
  const displayName = user?.username || user?.email || '—'
  const toggleLang = () => i18n.changeLanguage(i18n.language === 'zh' ? 'en' : 'zh')
  const isDark = resolved === 'dark'
  const { isWorkMode } = useMode()
  const [isMoreOpen, setIsMoreOpen] = useState(false)
  const navItems = NAV_ITEMS
  const mobileMoreRoutes = ['/budgets', '/recurring', '/exchange', '/settings']
  const mobileNavItems = NAV_ITEMS.filter((item) => !mobileMoreRoutes.includes(item.to))
  const mobileMoreItems = NAV_ITEMS.filter((item) => mobileMoreRoutes.includes(item.to))

  return (
    <div className="flex overflow-x-hidden bg-[hsl(var(--background))] text-[hsl(var(--foreground))] transition-colors duration-200" style={{ height: '100dvh' }}>

      {/* ── Desktop Sidebar ── */}
      <aside className="relative hidden w-[248px] shrink-0 flex-col overflow-hidden border-r border-white/10 bg-[#17201d] text-[#f2f5f1] md:flex">
        <div className="pointer-events-none absolute inset-y-0 right-3 w-px bg-white/[0.045]" aria-hidden="true" />
        <div className="pointer-events-none absolute right-[9px] top-24 flex flex-col gap-20" aria-hidden="true">
          <span className="h-px w-2 bg-white/15" /><span className="h-px w-2 bg-white/15" /><span className="h-px w-2 bg-white/15" />
        </div>
        {/* Brand */}
        <div className="px-5 pb-5 pt-6">
          <div className="flex items-center gap-3 border-b border-white/10 pb-5">
            <LogoMark size={40} className="rounded-[9px]" />
            <div className="min-w-0">
              <h1 className="font-display text-[17px] font-semibold leading-tight tracking-[-0.02em] text-white">FinArch</h1>
              <p className="mt-1 truncate text-[9px] font-semibold uppercase tracking-[0.18em] text-white/45">{isWorkMode ? t('nav.subtitle') : t('nav.life.subtitle')}</p>
            </div>
          </div>
        </div>


        <div className="px-5 pb-4">
          <ModeSwitcher variant="sidebar" />
        </div>
        {/* Nav */}
        <nav className="min-h-0 flex-1 space-y-1 overflow-y-auto px-3 py-2">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) =>
                `relative flex items-center gap-3 rounded-[7px] px-3 py-2.5 text-[13px] font-medium transition-colors duration-150 ${isActive
                  ? 'bg-white/[0.09] text-white after:absolute after:inset-y-2 after:left-0 after:w-0.5 after:bg-[hsl(var(--mode-accent))]'
                  : 'text-white/55 hover:bg-white/[0.055] hover:text-white/90'
                }`
              }
            >
              <item.Icon />
              {t(item.labelKey)}
            </NavLink>
          ))}
        </nav>

        {/* Theme + Language toggles */}
        <div className="grid grid-cols-2 gap-2 px-3 pb-3">
          <button
            onClick={toggleTheme}
            type="button"
            className={`${controlBtnClass} border-white/10 bg-white/[0.04] text-[11px] font-medium text-white/60 hover:border-white/20 hover:bg-white/[0.08] hover:text-white`}
            title={t('theme.toggle')}
          >
            {isDark ? <IconSun /> : <IconMoon />}
            <span>{t('theme.toggle')}</span>
          </button>
          <button
            onClick={toggleLang}
            type="button"
            className={`${controlBtnClass} border-white/10 bg-white/[0.04] text-[11px] font-semibold text-white/60 hover:border-white/20 hover:bg-white/[0.08] hover:text-white`}
            title={t('language.toggle')}
          >
            <IconLang />
            <span>{i18n.language === 'zh' ? 'EN' : '中'}</span>
          </button>
        </div>

        {/* User section */}
        <div className="mt-auto space-y-2 border-t border-white/10 px-3 pb-5 pt-3">
          <div className="flex items-center gap-2.5 rounded-[8px] bg-white/[0.045] px-3 py-2.5">
            <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-[7px] border border-white/10 bg-white/[0.08] font-data text-xs font-bold text-white">
              {initial}
            </div>
            <p className="flex-1 truncate text-[12px] font-medium text-white/70">{displayName}</p>
          </div>
          <button
            onClick={logout}
            title={t('nav.logout')}
            type="button"
            className="flex w-full items-center justify-center gap-2 rounded-[7px] border border-[#b95642]/25 px-3 py-2 text-[12px] font-semibold text-[#dc8b78] transition-colors hover:border-[#b95642]/45 hover:bg-[#b95642]/10 hover:text-[#efaa99]"
          >
            <IconLogout />
            <span>{t('nav.logout')}</span>
          </button>
        </div>
      </aside>

      {/* ── Mobile Top Header ── */}
      <Drawer.Root open={isMoreOpen} onOpenChange={setIsMoreOpen} shouldScaleBackground>
        <header
          className="gpu-layer fixed inset-x-0 top-0 z-50 border-b border-[hsl(var(--border))] bg-[hsl(var(--card))]/95 px-3 pb-2.5 backdrop-blur-md md:hidden"
          style={{ paddingTop: 'max(0.55rem, env(safe-area-inset-top))' }}
        >
          <div className="flex min-h-11 min-w-0 items-center justify-between gap-2 overflow-hidden">
            <div className="flex items-center gap-2.5 min-w-0">
              <LogoMark size={30} className="shrink-0 rounded-[7px]" />
              <div className="min-w-0">
                <span className="font-display block truncate text-[15px] font-semibold tracking-tight text-[hsl(var(--foreground))]">FinArch</span>
                <span className="block truncate text-[8px] font-semibold uppercase tracking-[0.14em] text-[hsl(var(--muted-foreground))]">{isWorkMode ? t('nav.subtitle') : t('nav.life.subtitle')}</span>
              </div>
            </div>
            <div className="flex shrink-0 items-center gap-2">
              <ModeSwitcher />
              <Drawer.Trigger asChild>
                <button
                  type="button"
                  title={t('nav.more')}
                  aria-label={t('nav.more')}
                  className="fin-control flex h-10 w-10 items-center justify-center text-[hsl(var(--foreground))] transition-transform active:scale-95"
                >
                  <IconMore />
                </button>
              </Drawer.Trigger>
            </div>
          </div>
        </header>

        <Drawer.Portal>
          <Drawer.Overlay className="fixed inset-0 z-[70] bg-[#101815]/60 backdrop-blur-[2px]" />
          <Drawer.Content className="fixed inset-x-0 bottom-0 z-[80] max-h-[88dvh] rounded-t-[18px] border border-[hsl(var(--border))] bg-[hsl(var(--card))] px-4 pb-[calc(env(safe-area-inset-bottom)+1rem)] pt-3 shadow-[0_-18px_60px_rgba(9,18,14,0.18)] outline-none">
            <Drawer.Handle className="mx-auto mb-4 h-1 w-12 rounded-full bg-[hsl(var(--border))]" />
            <div className="mx-auto max-w-md overflow-y-auto pb-2">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <Drawer.Title className="font-display text-lg font-semibold tracking-tight text-[hsl(var(--foreground))]">
                    {t('nav.moreTitle')}
                  </Drawer.Title>
                  <Drawer.Description className="mt-1 text-xs text-[hsl(var(--muted-foreground))]">
                    {t('nav.moreDescription')}
                  </Drawer.Description>
                </div>
                <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-[8px] bg-[hsl(var(--foreground))] font-data text-xs font-bold text-[hsl(var(--background))]">
                  {initial}
                </div>
              </div>

              <div className="mt-5 grid grid-cols-2 gap-3">
                {mobileMoreItems.map((item) => (
                  <NavLink
                    key={item.to}
                    to={item.to}
                    onClick={() => setIsMoreOpen(false)}
                    className={({ isActive }) =>
                      `group min-h-[6rem] rounded-[10px] border p-3.5 transition-all active:scale-[0.98] ${isActive
                        ? 'border-[hsl(var(--mode-accent))] bg-[hsl(var(--mode-accent-wash))] text-[hsl(var(--mode-accent-strong))]'
                        : 'border-[hsl(var(--border))] bg-[hsl(var(--muted))]/60 text-[hsl(var(--foreground))] hover:border-[hsl(var(--mode-accent))]/50'
                      }`
                    }
                  >
                    <div className="mb-3 flex h-9 w-9 items-center justify-center rounded-[7px] border border-[hsl(var(--border))] bg-[hsl(var(--card))] text-[hsl(var(--mode-accent-strong))] transition-transform group-hover:-translate-y-0.5">
                      <item.Icon />
                    </div>
                    <p className="text-sm font-bold leading-tight">{t(item.labelKey)}</p>
                    <p className="mt-1 text-[11px] leading-snug text-[hsl(var(--muted-foreground))]">{t(`nav.moreHints.${item.to}`)}</p>
                  </NavLink>
                ))}
              </div>

              <div className="mt-5 rounded-[10px] border border-[hsl(var(--border))] bg-[hsl(var(--muted))]/55 p-3">
                <p className="page-kicker px-1 pb-2">{t('nav.preferences')}</p>
                <div className="grid grid-cols-2 gap-2">
                  <button onClick={toggleTheme} className={`${controlBtnClass} h-11 text-[12px] font-semibold`}>
                    {isDark ? <IconSun /> : <IconMoon />}
                    <span>{t('theme.toggle')}</span>
                  </button>
                  <button onClick={toggleLang} className={`${controlBtnClass} h-11 text-[12px] font-semibold`}>
                    <IconLang />
                    <span>{i18n.language === 'zh' ? 'EN' : '中'}</span>
                  </button>
                </div>
              </div>

              <div className="mt-3 rounded-[10px] border border-[hsl(var(--border))] bg-[hsl(var(--card))] p-3">
                <p className="page-kicker px-1 pb-2">{t('nav.account')}</p>
                <div className="flex items-center gap-2.5 rounded-[7px] bg-[hsl(var(--muted))] px-3 py-2.5">
                  <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-[6px] bg-[hsl(var(--foreground))] font-data text-xs font-bold text-[hsl(var(--background))]">
                    {initial}
                  </div>
                  <p className="flex-1 truncate text-[12px] font-medium text-[hsl(var(--foreground))]">{displayName}</p>
                </div>
                <button
                  onClick={logout}
                  title={t('nav.logout')}
                  type="button"
                  className="mt-2 flex w-full items-center justify-center gap-2 rounded-[7px] border border-[hsl(var(--expense))]/30 px-3 py-2.5 text-[12px] font-semibold text-[hsl(var(--expense))] transition-colors hover:bg-[hsl(var(--expense))]/10"
                >
                  <IconLogout />
                  <span>{t('nav.logout')}</span>
                </button>
              </div>
            </div>
          </Drawer.Content>
        </Drawer.Portal>
      </Drawer.Root>

      {/* ── Main Content ── */}
      <main className="scroll-main ledger-workspace flex min-w-0 flex-1 flex-col overflow-x-hidden overflow-y-auto pt-[4rem] md:pb-0 md:pt-0">
        <div className="mx-auto w-full max-w-[1440px] flex-1 px-4 py-6 sm:px-5 md:px-8 md:py-8 lg:px-10">
          <PageTransition motionKey={location.pathname}>
            {children}
          </PageTransition>
        </div>

        {/* ── Footer ── */}
        <footer className="shrink-0 mt-auto">
          <BrandDivider className="mx-6 md:mx-8" />
          <div className="mx-auto flex max-w-[1440px] items-center justify-between gap-4 px-4 py-4 md:px-8 lg:px-10">
            <div className="flex items-center gap-2.5">
              <LogoBars size={16} opacity={0.25} />
              <span className="text-[11px] font-semibold tracking-wide text-[hsl(var(--muted-foreground))]/70">FinArch</span>
              <span className="text-[11px] text-[hsl(var(--border))]">·</span>
              <span className="text-[11px] text-[hsl(var(--muted-foreground))]/70">{isWorkMode ? t('nav.footer') : t('nav.life.footer')}</span>
            </div>
            <span className="font-data text-[10px] text-[hsl(var(--muted-foreground))]/60">v2.3</span>
          </div>
        </footer>
      </main>

      {/* ── Mobile Bottom Navigation ── */}
      <nav className="gpu-layer safe-bottom fixed inset-x-0 bottom-0 z-50 flex items-end border-t border-[hsl(var(--border))] bg-[hsl(var(--card))]/95 backdrop-blur-md md:hidden">
        {mobileNavItems.map((item) => {
          if (item.isPrimary) {
            return (
              <NavLink
                key={item.to}
                to={item.to}
                className="flex min-h-[4.25rem] flex-1 flex-col items-center justify-center gap-1 py-2.5 text-[hsl(var(--mode-accent-strong))]"
              >
                <div className="flex h-8 w-10 items-center justify-center rounded-[7px] bg-[hsl(var(--mode-accent))] text-[hsl(var(--primary-foreground))] transition-transform active:scale-95">
                  <item.Icon />
                </div>
                <span className="text-[10px] font-semibold">{t(item.labelKey)}</span>
              </NavLink>
            )
          }
          return (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) =>
                `flex min-h-[4.25rem] flex-1 flex-col items-center justify-center gap-1 py-2.5 transition-colors ${isActive ? 'text-[hsl(var(--mode-accent-strong))]' : 'text-[hsl(var(--muted-foreground))] active:text-[hsl(var(--foreground))]'
                }`
              }
            >
              {({ isActive }) => (
                <>
                  <div className={`rounded-[6px] border-b-2 p-1.5 transition-colors ${isActive ? 'border-[hsl(var(--mode-accent))] bg-[hsl(var(--mode-accent-wash))]' : 'border-transparent'}`}>
                    <item.Icon />
                  </div>
                  <span className="text-[11px] font-semibold leading-none">{t(item.labelKey)}</span>
                </>
              )}
            </NavLink>
          )
        })}
      </nav>
    </div>
  )
}
