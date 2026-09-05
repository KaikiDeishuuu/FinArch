import { useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { confirmOldEmailForChange } from '../api/client'
import { LogoMark } from '../components/Brand'
import { useThemeColor } from '../hooks/useThemeColor'
import { useActionToken } from '../hooks/useActionToken'

export default function ConfirmOldEmailChangePage() {
  useThemeColor('#7c3aed', '#1e1033')
  const { t } = useTranslation()
  const token = useActionToken()

  const [status, setStatus] = useState<'ready' | 'loading' | 'success' | 'error'>(token ? 'ready' : 'error')
  const [errorMsg, setErrorMsg] = useState('')
  const submittingRef = useRef(false)
  const visibleErrorMsg = token ? errorMsg : t('confirmOldEmailChange.invalidLink')

  async function handleConfirm() {
    if (!token || submittingRef.current) return
    submittingRef.current = true
    setStatus('loading')
    setErrorMsg('')
    try {
      await confirmOldEmailForChange(token)
      setStatus('success')
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      setErrorMsg(msg || t('confirmOldEmailChange.errorDefault'))
      setStatus('error')
    } finally {
      submittingRef.current = false
    }
  }

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
            <div className="w-14 h-14 rounded-2xl bg-violet-100 dark:bg-violet-500/15 text-violet-600 dark:text-violet-400 flex items-center justify-center mx-auto">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} className="w-7 h-7">
                <path strokeLinecap="round" strokeLinejoin="round" d="M9 12.75L11.25 15 15 9.75M12 2.25c2.03 1.8 4.56 2.91 7.25 3.18a11.95 11.95 0 01-7.25 16.32A11.95 11.95 0 014.75 5.43 12.1 12.1 0 0012 2.25z" />
              </svg>
            </div>
            <div className="space-y-2">
              <h2 className="text-lg font-bold text-gray-800 dark:text-gray-100">{t('confirmOldEmailChange.readyTitle')}</h2>
              <p className="text-gray-500 dark:text-gray-400 text-sm leading-relaxed">{t('confirmOldEmailChange.readyDesc')}</p>
            </div>
            <button
              type="button"
              onClick={handleConfirm}
              className="w-full bg-gradient-to-r from-violet-600 to-purple-600 hover:from-violet-700 hover:to-purple-700 text-white text-sm font-semibold px-8 py-3 rounded-xl transition-all shadow-lg shadow-violet-500/25 hover:shadow-violet-500/40 active:scale-[0.98] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-400 focus-visible:ring-offset-2"
            >
              {t('confirmOldEmailChange.confirmButton')}
            </button>
            <Link to="/login" className="inline-block text-xs text-gray-400 dark:text-gray-500 hover:text-violet-600 dark:hover:text-violet-400 transition-colors font-medium">
              {t('confirmOldEmailChange.cancelButton')}
            </Link>
          </div>
        )}

        {status === 'loading' && (
          <div className="space-y-3">
            <div className="w-10 h-10 border-4 border-violet-200 dark:border-violet-800 border-t-violet-600 rounded-full animate-spin mx-auto" />
            <p className="text-gray-500 dark:text-gray-400 text-sm">{t('confirmOldEmailChange.processing')}</p>
          </div>
        )}

        {status === 'success' && (
          <div className="space-y-4">
            <div className="w-14 h-14 rounded-2xl bg-gradient-to-br from-violet-400 to-violet-500 flex items-center justify-center mx-auto shadow-lg shadow-violet-500/25">
              <svg viewBox="0 0 24 24" fill="none" stroke="white" strokeWidth={1.5} className="w-7 h-7">
                <path strokeLinecap="round" strokeLinejoin="round" d="M21.75 6.75v10.5a2.25 2.25 0 01-2.25 2.25h-15a2.25 2.25 0 01-2.25-2.25V6.75m19.5 0A2.25 2.25 0 0019.5 4.5h-15a2.25 2.25 0 00-2.25 2.25m19.5 0v.243a2.25 2.25 0 01-1.07 1.916l-7.5 4.615a2.25 2.25 0 01-2.36 0L3.32 8.91a2.25 2.25 0 01-1.07-1.916V6.75" />
              </svg>
            </div>
            <h2 className="text-lg font-bold text-gray-800 dark:text-gray-100">{t('confirmOldEmailChange.success')}</h2>
            <p className="text-gray-500 dark:text-gray-400 text-sm leading-relaxed">
              {t('confirmOldEmailChange.successDesc')}
            </p>
            <p className="text-xs text-gray-400 dark:text-gray-500">{t('confirmOldEmailChange.successHint')}</p>
            <Link
              to="/login"
              className="inline-block mt-2 bg-gradient-to-r from-violet-600 to-purple-600 hover:from-violet-700 hover:to-purple-700 text-white text-sm font-semibold px-8 py-2.5 rounded-xl transition-all shadow-lg shadow-violet-500/25 hover:shadow-violet-500/40 active:scale-[0.98]"
            >
              {t('confirmOldEmailChange.backToLogin')}
            </Link>
          </div>
        )}

        {status === 'error' && (
          <div className="space-y-4">
            <div className="w-14 h-14 rounded-2xl bg-gradient-to-br from-rose-400 to-rose-500 flex items-center justify-center mx-auto shadow-lg shadow-rose-500/25">
              <svg viewBox="0 0 24 24" fill="none" stroke="white" strokeWidth={2} className="w-7 h-7">
                <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </div>
            <h2 className="text-lg font-bold text-gray-800 dark:text-gray-100">{t('confirmOldEmailChange.errorTitle')}</h2>
            <p className="text-gray-500 dark:text-gray-400 text-sm">{visibleErrorMsg}</p>
            <div className="flex flex-col gap-2">
              <Link
                to="/settings"
                className="inline-block bg-gradient-to-r from-violet-600 to-purple-600 hover:from-violet-700 hover:to-purple-700 text-white text-sm font-semibold px-8 py-2.5 rounded-xl transition-all shadow-lg shadow-violet-500/25 hover:shadow-violet-500/40 active:scale-[0.98]"
              >
                {t('confirmOldEmailChange.backToSettings')}
              </Link>
              <Link to="/login" className="text-xs text-gray-400 dark:text-gray-500 hover:text-violet-600 dark:hover:text-violet-400 transition-colors font-medium">
                {t('confirmOldEmailChange.backToLogin')}
              </Link>
            </div>
          </div>
        )}
      </div>
      <div className="flex-[3]" />
    </div>
  )
}
