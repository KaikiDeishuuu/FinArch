import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { AuthProvider, useAuth } from './contexts/AuthContext'
import { ConfigProvider } from './contexts/ConfigContext'
import Layout from './components/Layout'
import LoginPage from './pages/LoginPage'
import ForgotPasswordPage from './pages/ForgotPasswordPage'
import ResetPasswordPage from './pages/ResetPasswordPage'
import ConfirmDeleteAccountPage from './pages/ConfirmDeleteAccountPage'
import DashboardPage from './pages/DashboardPage'
import TransactionsPage from './pages/TransactionsPage'
import AddTransactionPage from './pages/AddTransactionPage'
import MatchPage from './pages/MatchPage'
import StatsPage from './pages/StatsPage'
import SettingsPage from './pages/SettingsPage'

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

function App() {
  return (
    <BrowserRouter>
      <ConfigProvider>
        <AuthProvider>
          <Routes>
            <Route path="/login" element={<LoginRouteWrapper />} />
            <Route path="/forgot-password" element={<ForgotPasswordPage />} />
            <Route path="/reset-password" element={<ResetPasswordPage />} />
            <Route path="/confirm-delete-account" element={<ConfirmDeleteAccountPage />} />
            <Route path="/*" element={<ProtectedRoutes />} />
          </Routes>
        </AuthProvider>
      </ConfigProvider>
    </BrowserRouter>
  )
}

export default App

