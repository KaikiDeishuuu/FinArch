import { lazy, Suspense, useEffect } from 'react'
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { MotionConfig } from 'framer-motion'
import { Toaster } from 'sonner'
import { ExchangeRateProvider } from './contexts/ExchangeRateContext'
import { AuthProvider } from './contexts/AuthContext'
import { useAuth } from './hooks/useAuth'
import { ConfigProvider } from './contexts/ConfigContext'
import { useConfig } from './hooks/useConfig'
import { ThemeProvider } from './contexts/ThemeContext'
import { useTheme } from './hooks/useTheme'
import { ModeProvider } from './contexts/ModeContext'
import Layout from './components/Layout'
import PwaUpdatePrompt from './components/PwaUpdatePrompt'

const loadLoginPage = () => import('./pages/LoginPage')
const loadForgotPasswordPage = () => import('./pages/ForgotPasswordPage')
const loadResetPasswordPage = () => import('./pages/ResetPasswordPage')
const loadVerifyEmailPage = () => import('./pages/VerifyEmailPage')
const loadConfirmDeleteAccountPage = () => import('./pages/ConfirmDeleteAccountPage')
const loadConfirmEmailChangePage = () => import('./pages/ConfirmEmailChangePage')
const loadConfirmOldEmailChangePage = () => import('./pages/ConfirmOldEmailChangePage')
const loadDisasterRestorePage = () => import('./pages/DisasterRestorePage')
const loadDashboardPage = () => import('./pages/DashboardPage')
const loadTransactionsPage = () => import('./pages/TransactionsPage')
const loadAddTransactionPage = () => import('./pages/AddTransactionPage')
const loadMatchPage = () => import('./pages/MatchPage')
const loadStatsPage = () => import('./pages/StatsPage')
const loadBudgetsPage = () => import('./pages/BudgetsPage')
const loadRecurringPage = () => import('./pages/RecurringPage')
const loadSettingsPage = () => import('./pages/SettingsPage')
const loadExchangeRatePage = () => import('./pages/ExchangeRatePage')

const LoginPage = lazy(loadLoginPage)
const ForgotPasswordPage = lazy(loadForgotPasswordPage)
const ResetPasswordPage = lazy(loadResetPasswordPage)
const VerifyEmailPage = lazy(loadVerifyEmailPage)
const ConfirmDeleteAccountPage = lazy(loadConfirmDeleteAccountPage)
const ConfirmEmailChangePage = lazy(loadConfirmEmailChangePage)
const ConfirmOldEmailChangePage = lazy(loadConfirmOldEmailChangePage)
const DisasterRestorePage = lazy(loadDisasterRestorePage)
const DashboardPage = lazy(loadDashboardPage)
const TransactionsPage = lazy(loadTransactionsPage)
const AddTransactionPage = lazy(loadAddTransactionPage)
const MatchPage = lazy(loadMatchPage)
const StatsPage = lazy(loadStatsPage)
const BudgetsPage = lazy(loadBudgetsPage)
const RecurringPage = lazy(loadRecurringPage)
const SettingsPage = lazy(loadSettingsPage)
const ExchangeRatePage = lazy(loadExchangeRatePage)

function ProtectedRoutes() {
  const { isAuthenticated, isLoading, sessionUnavailable } = useAuth()
  useEffect(() => {
    if (!isAuthenticated) return
    const preload = () => {
      void loadTransactionsPage()
      void loadAddTransactionPage()
      void loadStatsPage()
      void loadBudgetsPage()
      void loadRecurringPage()
      void loadSettingsPage()
    }
    const idleWindow = window as Window & {
      requestIdleCallback?: (callback: () => void, options?: { timeout: number }) => number
      cancelIdleCallback?: (id: number) => void
    }
    if (idleWindow.requestIdleCallback && idleWindow.cancelIdleCallback) {
      const id = idleWindow.requestIdleCallback(preload, { timeout: 2000 })
      return () => idleWindow.cancelIdleCallback?.(id)
    }
    const id = globalThis.setTimeout(preload, 800)
    return () => globalThis.clearTimeout(id)
  }, [isAuthenticated])
  if (sessionUnavailable) return <SessionRecoveryFallback />
  if (isLoading) return <PageFallback />
  if (!isAuthenticated) return <Navigate to="/login" replace />
  return (
    <Layout>
      <Routes>
        <Route path="/" element={<DashboardPage />} />
        <Route path="/transactions" element={<TransactionsPage />} />
        <Route path="/add" element={<AddTransactionPage />} />
        <Route path="/match" element={<MatchPage />} />
        <Route path="/stats" element={<StatsPage />} />
        <Route path="/budgets" element={<BudgetsPage />} />
        <Route path="/recurring" element={<RecurringPage />} />
        <Route path="/exchange" element={<ExchangeRatePage />} />
        <Route path="/settings" element={<SettingsPage />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Layout>
  )
}

function LoginRouteWrapper() {
  const { isAuthenticated, isLoading, sessionUnavailable } = useAuth()
  if (sessionUnavailable) return <SessionRecoveryFallback />
  if (isLoading) return <PageFallback />
  if (isAuthenticated) return <Navigate to="/" replace />
  return <LoginPage />
}

function ProtectedDisasterRestorePage() {
  const { isAuthenticated, isLoading, sessionUnavailable } = useAuth()
  const { loaded: configLoaded, systemOperationsEnabled } = useConfig()
  if (sessionUnavailable) return <SessionRecoveryFallback />
  if (isLoading) return <PageFallback />
  if (!isAuthenticated) return <Navigate to="/login" replace />
  if (!configLoaded) return <PageFallback />
  if (!systemOperationsEnabled) return <Navigate to="/" replace />
  return <DisasterRestorePage />
}

function ThemedToaster() {
  const { resolved } = useTheme()
  return (
    <Toaster
      position="bottom-right"
      theme={resolved}
      mobileOffset={{ bottom: 'calc(var(--mobile-bottom-nav-height) + env(safe-area-inset-bottom) + 12px)' }}
      toastOptions={{
        classNames: {
          toast: '!rounded-lg !border-border !bg-card !text-card-foreground !shadow-[var(--shadow-sm)]',
          title: '!text-foreground',
          description: '!text-muted-foreground',
          actionButton: '!rounded-md !bg-primary !text-primary-foreground',
          cancelButton: '!rounded-md !bg-muted !text-muted-foreground',
          success: '!border-positive/30',
          error: '!border-negative/30',
          warning: '!border-warning/30',
          info: '!border-accent/30',
        },
      }}
    />
  )
}

function PageFallback() {
  return <div className="min-h-screen bg-background" />
}

function SessionRecoveryFallback() {
  const { retrySession } = useAuth()
  const { t } = useTranslation()

  return (
    <main className="flex min-h-screen items-center justify-center bg-background p-6">
      <section
        role="alert"
        className="w-full max-w-md rounded-xl border border-warning/30 bg-card p-6 text-center text-card-foreground shadow-[var(--shadow-sm)]"
      >
        <h1 className="text-xl font-semibold text-foreground">
          {t('login.sessionRecoveryTitle')}
        </h1>
        <p className="mt-2 text-sm leading-6 text-muted-foreground">
          {t('login.sessionRecoveryDescription')}
        </p>
        <button
          type="button"
          onClick={retrySession}
          className="mt-5 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-opacity hover:opacity-90"
        >
          {t('login.sessionRecoveryRetry')}
        </button>
      </section>
    </main>
  )
}

function App() {
  useEffect(() => {
    const splash = document.getElementById('splash')
    if (!splash) return

    let fadeTimer: number | undefined
    let removeTimer: number | undefined
    const frame = window.requestAnimationFrame(() => {
      fadeTimer = window.setTimeout(() => {
        splash.classList.add('splash-fade-out')
        removeTimer = window.setTimeout(() => splash.remove(), 500)
      }, 150)
    })

    return () => {
      window.cancelAnimationFrame(frame)
      if (fadeTimer !== undefined) window.clearTimeout(fadeTimer)
      if (removeTimer !== undefined) window.clearTimeout(removeTimer)
    }
  }, [])

  return (
    <MotionConfig reducedMotion="user">
      <ThemeProvider>
        <BrowserRouter>
          <ExchangeRateProvider>
            <ConfigProvider>
              <ModeProvider>
                <AuthProvider>
                  <Suspense fallback={<PageFallback />}>
                    <Routes>
                      <Route path="/login" element={<LoginRouteWrapper />} />
                      <Route path="/verify-email" element={<VerifyEmailPage />} />
                      <Route path="/forgot-password" element={<ForgotPasswordPage />} />
                      <Route path="/reset-password" element={<ResetPasswordPage />} />
                      <Route path="/confirm-delete-account" element={<ConfirmDeleteAccountPage />} />
                      <Route path="/confirm-email-change-old" element={<ConfirmOldEmailChangePage />} />
                      <Route path="/confirm-email-change" element={<ConfirmEmailChangePage />} />
                      <Route path="/disaster-restore" element={<ProtectedDisasterRestorePage />} />
                      <Route path="/*" element={<ProtectedRoutes />} />
                    </Routes>
                  </Suspense>
                </AuthProvider>
              </ModeProvider>
            </ConfigProvider>
          </ExchangeRateProvider>
          <PwaUpdatePrompt />
          <ThemedToaster />
        </BrowserRouter>
      </ThemeProvider>
    </MotionConfig>
  )
}

export default App
