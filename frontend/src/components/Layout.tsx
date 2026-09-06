import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { NavLink, useLocation } from 'react-router-dom'
import { Drawer } from 'vaul'
import {
  ArrowRightLeft,
  BarChart3,
  ChevronRight,
  Home,
  Languages,
  List,
  LogOut,
  MoreHorizontal,
  Plus,
  Repeat2,
  SearchCheck,
  Settings2,
  WalletCards,
  type LucideIcon,
} from 'lucide-react'
import { useAuth } from '../hooks/useAuth'
import { useMode } from '../hooks/useMode'
import { useTheme } from '../hooks/useTheme'
import type { Theme } from '../contexts/themeContextCore'
import { PageTransition } from '../motion'
import { APP_NAME, APP_VERSION } from '../constants/app'
import { cn } from '../lib/utils'
import { LogoBars, LogoMark } from './Brand'
import ModeSwitcher from './ModeSwitcher'
import { Badge } from './ui/badge'
import { Button } from './ui/button'
import { Segmented, SegmentedButton } from './ui/segmented'

interface NavigationItem {
  to: string
  labelKey: string
  Icon: LucideIcon
  end?: boolean
  isPrimary?: boolean
}

const NAV_ITEMS = [
  { to: '/', labelKey: 'nav.dashboard', Icon: Home, end: true },
  { to: '/transactions', labelKey: 'nav.transactions', Icon: List },
  { to: '/add', labelKey: 'nav.add', Icon: Plus, isPrimary: true },
  { to: '/match', labelKey: 'nav.match', Icon: SearchCheck },
  { to: '/stats', labelKey: 'nav.stats', Icon: BarChart3 },
  { to: '/budgets', labelKey: 'nav.budgets', Icon: WalletCards },
  { to: '/recurring', labelKey: 'nav.recurring', Icon: Repeat2 },
  { to: '/exchange', labelKey: 'nav.exchange', Icon: ArrowRightLeft },
  { to: '/settings', labelKey: 'nav.settings', Icon: Settings2 },
] satisfies NavigationItem[]

const MOBILE_MORE_ROUTES = new Set(['/budgets', '/recurring', '/exchange', '/settings'])

function ThemeSelector({
  theme,
  setTheme,
  className,
}: {
  theme: Theme
  setTheme: (theme: Theme) => void
  className?: string
}) {
  const { t } = useTranslation()
  const options: Array<{ value: Theme; label: string }> = [
    { value: 'light', label: t('theme.light') },
    { value: 'dark', label: t('theme.dark') },
    { value: 'system', label: t('theme.system') },
  ]

  return (
    <Segmented aria-label={t('theme.toggle')} className={cn('grid w-full grid-cols-3', className)}>
      {options.map((option) => (
        <SegmentedButton
          key={option.value}
          aria-pressed={theme === option.value}
          onClick={() => setTheme(option.value)}
          className="min-w-0 px-1.5 text-[10px]"
        >
          {option.label}
        </SegmentedButton>
      ))}
    </Segmented>
  )
}

function Avatar({ initial, size = 'md' }: { initial: string; size?: 'sm' | 'md' }) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        'grid shrink-0 place-items-center rounded-full bg-muted font-semibold text-foreground ring-1 ring-border',
        size === 'sm' ? 'size-8 text-[11px]' : 'size-9 text-xs',
      )}
    >
      {initial}
    </span>
  )
}

export default function Layout({ children }: { children: ReactNode }) {
  const { user, logout } = useAuth()
  const { theme, setTheme } = useTheme()
  const { isWorkMode } = useMode()
  const { t, i18n } = useTranslation()
  const location = useLocation()
  const [isMoreOpen, setIsMoreOpen] = useState(false)

  const identity = user?.username || user?.email || '—'
  const initial = (identity === '—' ? '?' : identity[0]!).toUpperCase()
  const isChinese = (i18n.resolvedLanguage ?? i18n.language).toLowerCase().startsWith('zh')
  const mobileNavItems = NAV_ITEMS.filter((item) => !MOBILE_MORE_ROUTES.has(item.to))
  const mobileMoreItems = NAV_ITEMS.filter((item) => MOBILE_MORE_ROUTES.has(item.to))
  const modeLabel = isWorkMode ? t('mode.work') : t('mode.life')
  const modeSubtitle = isWorkMode ? t('nav.subtitle') : t('nav.life.subtitle')

  const toggleLanguage = () => {
    void i18n.changeLanguage(isChinese ? 'en' : 'zh')
  }

  return (
    <div className="flex h-[100dvh] overflow-x-hidden bg-background text-foreground">
      <aside className="hidden w-60 shrink-0 flex-col border-r border-border bg-card md:flex">
        <div className="px-5 pb-4 pt-5">
          <div className="flex items-center gap-3">
            <LogoMark size={34} decorative />
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <h1 className="truncate text-[15px] font-semibold tracking-tight">{APP_NAME}</h1>
                <Badge variant="mode">{modeLabel}</Badge>
              </div>
              <p className="mt-0.5 truncate text-[10px] text-muted-foreground">{modeSubtitle}</p>
            </div>
          </div>
        </div>

        <div className="px-4 pb-3">
          <ModeSwitcher variant="sidebar" />
        </div>

        <nav className="min-h-0 flex-1 space-y-0.5 overflow-y-auto px-3 py-1" aria-label={APP_NAME}>
          {NAV_ITEMS.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) => cn(
                'relative flex h-9 items-center gap-3 rounded-lg px-3 text-[13px] font-medium transition-colors before:absolute before:left-0 before:h-4 before:w-0.5 before:rounded-full before:bg-transparent',
                isActive
                  ? 'bg-muted text-foreground before:bg-accent'
                  : 'text-muted-foreground hover:bg-muted hover:text-foreground',
              )}
            >
              <item.Icon className="size-[17px] shrink-0" strokeWidth={1.9} />
              <span className="truncate">{t(item.labelKey)}</span>
            </NavLink>
          ))}
        </nav>

        <div className="space-y-3 border-t border-border px-3 py-4">
          <div className="space-y-1.5">
            <p className="px-1 text-[10px] font-medium uppercase tracking-[0.12em] text-muted-foreground">
              {t('theme.toggle')}
            </p>
            <ThemeSelector theme={theme} setTheme={setTheme} />
          </div>

          <Button
            variant="ghost"
            size="sm"
            onClick={toggleLanguage}
            className="w-full justify-start text-muted-foreground"
          >
            <Languages className="size-4" />
            <span>{t('language.toggle')}</span>
            <span className="ml-auto font-mono text-[10px] text-muted-foreground">{isChinese ? 'EN' : '中'}</span>
          </Button>

          <div className="flex items-center gap-2.5 rounded-lg border border-border bg-background px-2.5 py-2">
            <Avatar initial={initial} size="sm" />
            <p className="min-w-0 flex-1 truncate text-xs font-medium">{identity}</p>
          </div>

          <Button
            variant="danger"
            size="sm"
            onClick={() => void logout()}
            className="w-full"
          >
            <LogOut className="size-4" />
            {t('nav.logout')}
          </Button>
        </div>
      </aside>

      <Drawer.Root open={isMoreOpen} onOpenChange={setIsMoreOpen} shouldScaleBackground>
        <header
          className="gpu-layer fixed inset-x-0 top-0 z-50 border-b border-border bg-card/95 px-3 pb-2 backdrop-blur-md md:hidden"
          style={{ paddingTop: 'max(0.55rem, env(safe-area-inset-top))' }}
        >
          <div className="flex min-h-11 min-w-0 items-center justify-between gap-2">
            <div className="flex min-w-0 items-center gap-2">
              <LogoMark size={29} decorative />
              <span className="truncate text-sm font-semibold tracking-tight">{APP_NAME}</span>
            </div>
            <ModeSwitcher className="shrink-0" />
            <div className="flex shrink-0 items-center gap-1.5">
              <Avatar initial={initial} size="sm" />
              <Drawer.Trigger asChild>
                <Button
                  variant="outline"
                  size="icon"
                  title={t('nav.more')}
                  aria-label={t('nav.more')}
                >
                  <MoreHorizontal className="size-[18px]" />
                </Button>
              </Drawer.Trigger>
            </div>
          </div>
        </header>

        <Drawer.Portal>
          <Drawer.Overlay className="fixed inset-0 z-[70] bg-foreground/35 backdrop-blur-[2px]" />
          <Drawer.Content className="fixed inset-x-0 bottom-0 z-[80] flex max-h-[88dvh] flex-col rounded-t-xl border border-border bg-card px-4 pb-[calc(env(safe-area-inset-bottom)+1rem)] pt-3 text-card-foreground shadow-[var(--shadow-sm)] outline-none">
            <Drawer.Handle className="mx-auto mb-4 h-1 w-10 shrink-0 rounded-full bg-input" />
            <div className="mx-auto min-h-0 w-full max-w-md overflow-y-auto pb-2">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <Drawer.Title className="text-lg font-semibold tracking-tight text-foreground">
                    {t('nav.moreTitle')}
                  </Drawer.Title>
                  <Drawer.Description className="mt-1 text-xs text-muted-foreground">
                    {t('nav.moreDescription')}
                  </Drawer.Description>
                </div>
                <Badge variant="mode">{modeLabel}</Badge>
              </div>

              <nav className="mt-5 grid grid-cols-2 gap-2" aria-label={t('nav.moreTitle')}>
                {mobileMoreItems.map((item) => (
                  <NavLink
                    key={item.to}
                    to={item.to}
                    onClick={() => setIsMoreOpen(false)}
                    className={({ isActive }) => cn(
                      'group min-h-24 rounded-lg border border-border bg-background p-3 transition-colors hover:border-input hover:bg-muted',
                      isActive && 'border-accent/35 bg-accent-soft',
                    )}
                  >
                    {({ isActive }) => (
                      <>
                        <div className={cn(
                          'mb-3 grid size-8 place-items-center rounded-md bg-card text-muted-foreground ring-1 ring-border',
                          isActive && 'text-accent',
                        )}>
                          <item.Icon className="size-4" />
                        </div>
                        <div className="flex items-center justify-between gap-2">
                          <p className="text-sm font-semibold text-foreground">{t(item.labelKey)}</p>
                          <ChevronRight className="size-3.5 text-muted-foreground transition-transform group-hover:translate-x-0.5" />
                        </div>
                        <p className="mt-1 text-[11px] leading-snug text-muted-foreground">
                          {t(`nav.moreHints.${item.to}`)}
                        </p>
                      </>
                    )}
                  </NavLink>
                ))}
              </nav>

              <section className="mt-4 rounded-lg border border-border bg-background p-3">
                <p className="mb-2 text-[10px] font-medium uppercase tracking-[0.12em] text-muted-foreground">
                  {t('nav.preferences')}
                </p>
                <ThemeSelector theme={theme} setTheme={setTheme} />
                <Button
                  variant="outline"
                  size="sm"
                  onClick={toggleLanguage}
                  className="mt-2 w-full justify-start"
                >
                  <Languages className="size-4" />
                  <span>{t('language.toggle')}</span>
                  <span className="ml-auto font-mono text-[10px] text-muted-foreground">
                    {isChinese ? 'EN' : '中'}
                  </span>
                </Button>
              </section>

              <section className="mt-3 rounded-lg border border-border bg-background p-3">
                <p className="mb-2 text-[10px] font-medium uppercase tracking-[0.12em] text-muted-foreground">
                  {t('nav.account')}
                </p>
                <div className="flex items-center gap-2.5 px-1 py-1">
                  <Avatar initial={initial} size="sm" />
                  <p className="min-w-0 flex-1 truncate text-xs font-medium">{identity}</p>
                </div>
                <Button
                  variant="danger"
                  size="sm"
                  onClick={() => void logout()}
                  className="mt-2 w-full"
                >
                  <LogOut className="size-4" />
                  {t('nav.logout')}
                </Button>
              </section>
            </div>
          </Drawer.Content>
        </Drawer.Portal>
      </Drawer.Root>

      <main className="scroll-main flex min-w-0 flex-1 flex-col overflow-x-hidden overflow-y-auto bg-background pt-[4rem] md:pb-0 md:pt-0">
        <div className="mx-auto w-full max-w-7xl flex-1 px-4 py-6 md:px-8 md:py-8">
          <PageTransition motionKey={location.pathname}>{children}</PageTransition>
        </div>

        <footer className="mx-auto flex w-full max-w-7xl shrink-0 items-center justify-between gap-4 border-t border-border px-4 py-3 text-[10px] text-muted-foreground md:px-8">
          <span className="flex min-w-0 items-center gap-2 truncate">
            <LogoBars size={14} opacity={0.45} />
            <span className="truncate">{isWorkMode ? t('nav.footer') : t('nav.life.footer')}</span>
          </span>
          <span className="shrink-0 font-mono">v{APP_VERSION}</span>
        </footer>
      </main>

      <nav className="gpu-layer safe-bottom fixed inset-x-0 bottom-0 z-50 flex items-end border-t border-border bg-card/95 backdrop-blur-md md:hidden" aria-label={APP_NAME}>
        {mobileNavItems.map((item) => {
          if (item.isPrimary) {
            return (
              <NavLink
                key={item.to}
                to={item.to}
                className="flex flex-1 -mt-4 flex-col items-center pb-2.5 pt-1 text-muted-foreground"
              >
                <span className="grid size-11 place-items-center rounded-lg bg-primary text-primary-foreground shadow-[var(--shadow-sm)] transition-opacity active:opacity-80">
                  <item.Icon className="size-5" strokeWidth={2.2} />
                </span>
                <span className="mt-1 text-[10px] font-medium">{t(item.labelKey)}</span>
              </NavLink>
            )
          }

          return (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) => cn(
                'flex min-h-[4.25rem] flex-1 flex-col items-center justify-center gap-1 py-2.5 text-muted-foreground transition-colors',
                isActive && 'text-accent',
              )}
            >
              {({ isActive }) => (
                <>
                  <span className={cn('grid size-7 place-items-center rounded-md', isActive && 'bg-accent-soft')}>
                    <item.Icon className="size-[17px]" strokeWidth={1.9} />
                  </span>
                  <span className="text-[10px] font-medium leading-none">{t(item.labelKey)}</span>
                </>
              )}
            </NavLink>
          )
        })}
      </nav>
    </div>
  )
}
