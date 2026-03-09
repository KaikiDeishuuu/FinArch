import { useEffect } from 'react'
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { Toaster } from 'sonner'
import { ExchangeRateProvider } from './contexts/ExchangeRateContext'
import { AuthProvider, useAuth } from './contexts/AuthContext'
import { ConfigProvider } from './contexts/ConfigContext'
import { ThemeProvider, useTheme } from './contexts/ThemeContext'
import { ModeProvider } from './contexts/ModeContext'
import Layout from './components/Layout'
import PwaUpdatePrompt from './components/PwaUpdatePrompt'
import LoginPage from './pages/LoginPage'
import ForgotPasswordPage from './pages/ForgotPasswordPage'
import ResetPasswordPage from './pages/ResetPasswordPage'
import VerifyEmailPage from './pages/VerifyEmailPage'
import ConfirmDeleteAccountPage from './pages/ConfirmDeleteAccountPage'
import ConfirmEmailChangePage from './pages/ConfirmEmailChangePage'
import ConfirmOldEmailChangePage from './pages/ConfirmOldEmailChangePage'
import DisasterRestorePage from './pages/DisasterRestorePage'
import DashboardPage from './pages/DashboardPage'
import TransactionsPage from './pages/TransactionsPage'
import AddTransactionPage from './pages/AddTransactionPage'
import MatchPage from './pages/MatchPage'
import StatsPage from './pages/StatsPage'
import SettingsPage from './pages/SettingsPage'
import ExchangeRatePage from './pages/ExchangeRatePage'

function ProtectedRoutes() {
  const { isAuthenticated } = useAuth()
  if (!isAuthenticated) return <Navigate to="/login" replace />
  return (
    <Layout>
      <Routes>
        <Route path="/" element={<DashboardPage />} />
        <Route path="/transactions" element={<TransactionsPage />} />
        <Route path="/add" element={<AddTransactionPage />} />
        <Route path="/match" element={<MatchPage />} />
        <Route path="/stats" element={<StatsPage />} />
        <Route path="/exchange" element={<ExchangeRatePage />} />
        <Route path="/settings" element={<SettingsPage />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Layout>
  )
}

function LoginRouteWrapper() {
  const { isAuthenticated } = useAuth()
  if (isAuthenticated) return <Navigate to="/" replace />
  return <LoginPage />
}

function ThemedToaster() {
  const { resolved } = useTheme()
  return (
    <Toaster
      position="bottom-right"
      richColors
      theme={resolved}
      toastOptions={{ style: { fontFamily: 'Plus Jakarta Sans, system-ui, sans-serif' } }}
    />
  )
}

function App() {
  // Fade out the inline splash screen after React mounts
  useEffect(() => {
    const splash = document.getElementById('splash')
    if (!splash) return
    // Show splash ~2.5s, then fade-out 450ms, then remove
    const fadeTimer = setTimeout(() => {
      splash.classList.add('splash-fade-out')
      const removeTimer = setTimeout(() => splash.remove(), 500)
      return () => clearTimeout(removeTimer)
    }, 2500)
    return () => clearTimeout(fadeTimer)
  }, [])

  return (
    <ThemeProvider>
    <BrowserRouter>
      <ExchangeRateProvider>
      <ConfigProvider>
        <ModeProvider>
        <AuthProvider>
          <Routes>
            <Route path="/login" element={<LoginRouteWrapper />} />
            <Route path="/verify-email" element={<VerifyEmailPage />} />
            <Route path="/forgot-password" element={<ForgotPasswordPage />} />
            <Route path="/reset-password" element={<ResetPasswordPage />} />
            <Route path="/confirm-delete-account" element={<ConfirmDeleteAccountPage />} />
            <Route path="/confirm-email-change-old" element={<ConfirmOldEmailChangePage />} />
            <Route path="/confirm-email-change" element={<ConfirmEmailChangePage />} />
            <Route path="/disaster-restore" element={<DisasterRestorePage />} />
            <Route path="/*" element={<ProtectedRoutes />} />
          </Routes>
        </AuthProvider>
        </ModeProvider>
      </ConfigProvider>
      </ExchangeRateProvider>
      <PwaUpdatePrompt />
      <ThemedToaster />
    </BrowserRouter>
    </ThemeProvider>
  )
}

export default App

