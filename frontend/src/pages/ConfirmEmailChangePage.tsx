import { useEffect, useState } from 'react'
import { useSearchParams, Link, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { confirmEmailChange } from '../api/client'
import { LogoMark } from '../components/Brand'
import { useThemeColor } from '../hooks/useThemeColor'

export default function ConfirmEmailChangePage() {
  useThemeColor('#7c3aed', '#1e1033')
  const { t } = useTranslation()
  const [searchParams] = useSearchParams()
  const token = searchParams.get('token') ?? ''
  const navigate = useNavigate()

  const [status, setStatus] = useState<'loading' | 'success' | 'error'>('loading')
  const [errorMsg, setErrorMsg] = useState('')

  useEffect(() => {
    if (!token) {
      setStatus('error')
      setErrorMsg(t('confirmEmailChange.invalidLink'))
      return
    }
    confirmEmailChange(token)
      .then(() => {
        setStatus('success')
        // Redirect to login so user re-authenticates with new email
        setTimeout(() => navigate('/login?email_changed=1', { replace: true }), 3000)
      })
      .catch((err: unknown) => {
        const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
        setErrorMsg(msg || t('confirmEmailChange.errorDefault'))
        setStatus('error')
      })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token])

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

        {status === 'loading' && (
          <div className="space-y-3">
            <div className="w-10 h-10 border-4 border-violet-200 dark:border-violet-800 border-t-violet-600 rounded-full animate-spin mx-auto" />
            <p className="text-gray-500 dark:text-gray-400 text-sm">{t('confirmEmailChange.processing')}</p>
          </div>
        )}

        {status === 'success' && (
          <div className="space-y-4">
            <div className="w-14 h-14 rounded-2xl bg-gradient-to-br from-emerald-400 to-emerald-500 flex items-center justify-center mx-auto shadow-lg shadow-emerald-500/25">
              <svg viewBox="0 0 24 24" fill="none" stroke="white" strokeWidth={2} className="w-7 h-7">
                <path strokeLinecap="round" strokeLinejoin="round" d="M4.5 12.75l6 6 9-13.5" />
              </svg>
            </div>
            <h2 className="text-lg font-bold text-gray-800 dark:text-gray-100">{t('confirmEmailChange.success')}</h2>
            <p className="text-gray-500 dark:text-gray-400 text-sm leading-relaxed">
              {t('confirmEmailChange.successDesc')}
            </p>
            <p className="text-gray-400 dark:text-gray-500 text-xs">{t('confirmEmailChange.redirecting')}</p>
            <Link
              to="/login?email_changed=1"
              className="inline-block bg-gradient-to-r from-violet-600 to-purple-600 hover:from-violet-700 hover:to-purple-700 text-white text-sm font-semibold px-8 py-2.5 rounded-xl transition-all shadow-lg shadow-violet-500/25 hover:shadow-violet-500/40 active:scale-[0.98]"
            >
              {t('confirmEmailChange.loginNow')}
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
            <h2 className="text-lg font-bold text-gray-800 dark:text-gray-100">{t('confirmEmailChange.errorTitle')}</h2>
            <p className="text-gray-500 dark:text-gray-400 text-sm">{errorMsg}</p>
            <div className="flex flex-col gap-2">
              <Link
                to="/settings"
                className="inline-block bg-gradient-to-r from-violet-600 to-purple-600 hover:from-violet-700 hover:to-purple-700 text-white text-sm font-semibold px-8 py-2.5 rounded-xl transition-all shadow-lg shadow-violet-500/25 hover:shadow-violet-500/40 active:scale-[0.98]"
              >
                {t('confirmEmailChange.backToSettings')}
              </Link>
              <Link to="/login" className="text-xs text-gray-400 dark:text-gray-500 hover:text-violet-600 dark:hover:text-violet-400 transition-colors font-medium">
                {t('confirmEmailChange.backToLogin')}
              </Link>
            </div>
          </div>
        )}
      </div>
      <div className="flex-[3]" />
    </div>
  )
}
