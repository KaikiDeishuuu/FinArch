import { useEffect, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { confirmDeleteAccount } from '../api/client'
import { useAuth } from '../hooks/useAuth'
import { LogoMark } from '../components/Brand'
import { useThemeColor } from '../hooks/useThemeColor'
import { useActionToken } from '../hooks/useActionToken'


async function clearPWAState() {
  try {
    localStorage.clear()
    sessionStorage.clear()
    if ('caches' in window) {
      const names = await caches.keys()
      await Promise.all(names.map((n) => caches.delete(n)))
    }
    if ('serviceWorker' in navigator) {
      const regs = await navigator.serviceWorker.getRegistrations()
      await Promise.all(regs.map((r) => r.unregister()))
    }
  } catch {
    // best effort cleanup
  }
}

export default function ConfirmDeleteAccountPage() {
  useThemeColor('#7c3aed', '#1e1033')
  const { t } = useTranslation()
  const token = useActionToken()
  const navigate = useNavigate()
  const { clearSession } = useAuth()

  const [status, setStatus] = useState<'ready' | 'loading' | 'success' | 'error'>(token ? 'ready' : 'error')
  const [errorMsg, setErrorMsg] = useState('')
  const submittingRef = useRef(false)
  const redirectTimerRef = useRef<number | null>(null)
  const visibleErrorMsg = token ? errorMsg : t('confirmDeleteAccount.invalidLink')

  async function handleDelete() {
    if (!token || submittingRef.current) return
    submittingRef.current = true
    setStatus('loading')
    setErrorMsg('')
    try {
      await confirmDeleteAccount(token)
      setStatus('success')
      await clearPWAState()
      clearSession()
      redirectTimerRef.current = window.setTimeout(
        () => navigate('/login?deleted=1', { replace: true }),
        3000,
      )
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      setErrorMsg(msg || t('confirmDeleteAccount.errorDefault'))
      setStatus('error')
    } finally {
      submittingRef.current = false
    }
  }

  useEffect(() => () => {
    if (redirectTimerRef.current !== null) window.clearTimeout(redirectTimerRef.current)
  }, [])

  return (
    <div className="min-h-dvh flex flex-col overflow-y-auto overflow-x-hidden bg-gradient-to-br from-violet-600 via-purple-600 to-fuchsia-500 relative px-4 py-4 md:py-6">
      <div className="flex-[1]" />
      <div className="mx-auto w-full max-w-md shrink-0 bg-white/95 dark:bg-gray-900/95 backdrop-blur-xl rounded-3xl shadow-2xl shadow-violet-900/20 p-8 text-center relative z-10">
        {/* Logo */}
        <div className="flex flex-col items-center mb-6">
          <LogoMark size={48} className="rounded-2xl shadow-lg shadow-violet-500/20 mb-3" />
          <h1 className="text-xl font-extrabold text-gray-900 dark:text-gray-100 tracking-tight">FinArch</h1>
          <p className="text-xs text-gray-400 dark:text-gray-500 mt-0.5">{t('login.subtitle')}</p>
        </div>

        {status === 'ready' && (
          <div className="space-y-5">
            <div className="w-14 h-14 rounded-2xl bg-rose-100 dark:bg-rose-500/15 text-rose-600 dark:text-rose-400 flex items-center justify-center mx-auto">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} className="w-7 h-7">
                <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m9.303 3.376c.866 1.5-.217 3.374-1.948 3.374H4.645c-1.73 0-2.813-1.874-1.948-3.374L10.052 3.38c.865-1.5 3.03-1.5 3.896 0l7.355 12.746zM12 16.5h.008v.008H12V16.5z" />
              </svg>
            </div>
            <div className="space-y-2">
              <h2 className="text-lg font-bold text-gray-900 dark:text-gray-100">{t('confirmDeleteAccount.readyTitle')}</h2>
              <p className="text-gray-500 dark:text-gray-400 text-sm leading-relaxed">{t('confirmDeleteAccount.readyDesc')}</p>
            </div>
            <div className="rounded-xl border border-rose-200 dark:border-rose-500/30 bg-rose-50 dark:bg-rose-500/10 px-4 py-3 text-left">
              <p className="text-xs font-semibold uppercase tracking-wider text-rose-700 dark:text-rose-400">{t('confirmDeleteAccount.warningTitle')}</p>
              <p className="mt-1 text-sm leading-relaxed text-rose-700/90 dark:text-rose-300">{t('confirmDeleteAccount.warningDesc')}</p>
            </div>
            <button
              type="button"
              onClick={handleDelete}
              className="w-full bg-rose-600 hover:bg-rose-700 text-white text-sm font-semibold px-8 py-3 rounded-xl transition-all shadow-lg shadow-rose-500/20 hover:shadow-rose-500/30 active:scale-[0.98] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-rose-400 focus-visible:ring-offset-2"
            >
              {t('confirmDeleteAccount.confirmButton')}
            </button>
            <Link to="/login" className="inline-block text-sm font-medium text-gray-500 dark:text-gray-400 hover:text-violet-600 dark:hover:text-violet-400 transition-colors">
              {t('confirmDeleteAccount.cancelButton')}
            </Link>
          </div>
        )}

        {status === 'loading' && (
          <div className="space-y-3">
            <div className="w-10 h-10 border-4 border-violet-200 dark:border-violet-800 border-t-violet-600 rounded-full animate-spin mx-auto" />
            <p className="text-gray-500 dark:text-gray-400 text-sm">{t('confirmDeleteAccount.processing')}</p>
          </div>
        )}

        {status === 'success' && (
          <div className="space-y-4">
            <div className="w-14 h-14 rounded-2xl bg-gradient-to-br from-emerald-400 to-emerald-500 flex items-center justify-center mx-auto shadow-lg shadow-emerald-500/25">
              <svg viewBox="0 0 24 24" fill="none" stroke="white" strokeWidth={2} className="w-7 h-7">
                <path strokeLinecap="round" strokeLinejoin="round" d="M4.5 12.75l6 6 9-13.5"/>
              </svg>
            </div>
            <h2 className="text-lg font-bold text-gray-800 dark:text-gray-100">{t('confirmDeleteAccount.success')}</h2>
            <p className="text-gray-500 dark:text-gray-400 text-sm leading-relaxed">
              {t('confirmDeleteAccount.successDesc')}
            </p>
            <p className="text-gray-400 dark:text-gray-500 text-xs">{t('confirmDeleteAccount.redirecting')}</p>
            <Link
              to="/login"
              className="inline-block bg-gradient-to-r from-violet-600 to-purple-600 hover:from-violet-700 hover:to-purple-700 text-white text-sm font-semibold px-8 py-2.5 rounded-xl transition-all shadow-lg shadow-violet-500/25 hover:shadow-violet-500/40 active:scale-[0.98]"
            >
              {t('confirmDeleteAccount.loginNow')}
            </Link>
          </div>
        )}

        {status === 'error' && (
          <div className="space-y-4">
            <div className="w-14 h-14 rounded-2xl bg-gradient-to-br from-rose-400 to-rose-500 flex items-center justify-center mx-auto shadow-lg shadow-rose-500/25">
              <svg viewBox="0 0 24 24" fill="none" stroke="white" strokeWidth={2} className="w-7 h-7">
                <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12"/>
              </svg>
            </div>
            <h2 className="text-lg font-bold text-gray-800 dark:text-gray-100">{t('confirmDeleteAccount.errorTitle')}</h2>
            <p className="text-rose-600 dark:text-rose-400 text-sm">{visibleErrorMsg}</p>
            <div className="flex flex-col gap-2">
              <Link
                to="/settings"
                className="inline-block bg-gradient-to-r from-violet-600 to-purple-600 hover:from-violet-700 hover:to-purple-700 text-white text-sm font-semibold px-8 py-2.5 rounded-xl transition-all shadow-lg shadow-violet-500/25 hover:shadow-violet-500/40 active:scale-[0.98]"
              >
                {t('confirmDeleteAccount.reapply')}
              </Link>
              <Link to="/login" className="text-xs text-gray-400 dark:text-gray-500 hover:text-violet-600 dark:hover:text-violet-400 transition-colors font-medium">
                {t('confirmDeleteAccount.backToLogin')}
              </Link>
            </div>
          </div>
        )}
      </div>
      <div className="flex-[3]" />
    </div>
  )
}
