import { useState } from 'react'
import type { FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { forgotPassword } from '../api/client'
import { useThemeColor } from '../hooks/useThemeColor'
import { LogoMark } from '../components/Brand'

export default function ForgotPasswordPage() {
  useThemeColor('#7c3aed')
  const { t } = useTranslation()
  const [email, setEmail] = useState('')
  const [loading, setLoading] = useState(false)
  const [sent, setSent] = useState(false)
  const [error, setError] = useState('')

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      await forgotPassword(email)
      setSent(true)
    } catch {
      setError(t('forgotPassword.toast.error'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-dvh flex flex-col overflow-y-auto overflow-x-hidden bg-gradient-to-br from-violet-600 via-purple-600 to-fuchsia-500 relative px-4 py-4 md:py-6">
      <div className="flex-[1]" />
      <div className="mx-auto w-full max-w-md shrink-0 bg-white/95 dark:bg-gray-900/95 backdrop-blur-xl rounded-3xl shadow-2xl shadow-violet-900/20 p-8 relative z-10">
        <div className="flex flex-col items-center mb-6">
          <LogoMark size={48} className="rounded-2xl shadow-lg shadow-violet-500/20 mb-3" />
          <h1 className="text-xl font-bold text-gray-800 dark:text-gray-100">{t('forgotPassword.title')}</h1>
          <p className="text-xs text-gray-400 dark:text-gray-500 mt-0.5">{t('login.subtitle')}</p>
        </div>

        {sent ? (
          <div className="text-center">
            <div className="w-14 h-14 rounded-2xl bg-gradient-to-br from-emerald-400 to-emerald-500 flex items-center justify-center mx-auto mb-5 shadow-lg shadow-emerald-500/25">
              <svg viewBox="0 0 24 24" fill="none" stroke="white" strokeWidth={2} className="w-7 h-7">
                <path strokeLinecap="round" strokeLinejoin="round" d="M4.5 12.75l6 6 9-13.5"/>
              </svg>
            </div>
            <h2 className="text-lg font-bold text-gray-800 dark:text-gray-100 mb-2">{t('forgotPassword.sentTitle')}</h2>
            <p className="text-gray-500 dark:text-gray-400 text-sm mb-1 leading-relaxed">{t('forgotPassword.sentDesc')}</p>
            <p className="text-gray-400 dark:text-gray-500 text-xs mb-6">{t('forgotPassword.sentHint')}</p>
            <Link to="/login" className="inline-block bg-gradient-to-r from-violet-600 to-purple-600 hover:from-violet-700 hover:to-purple-700 text-white text-sm font-semibold px-8 py-2.5 rounded-xl transition-all shadow-lg shadow-violet-500/25 hover:shadow-violet-500/40 active:scale-[0.98]">
              {t('forgotPassword.backToLogin')}
            </Link>
          </div>
        ) : (
          <>
            <p className="text-sm text-gray-500 dark:text-gray-400 mb-5 text-center">
              {t('forgotPassword.desc')}
            </p>
            <form onSubmit={handleSubmit} className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1.5">{t('forgotPassword.emailLabel')}</label>
                <input
                  type="email"
                  required
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  className="w-full border border-gray-200/80 dark:border-gray-700 rounded-xl px-3.5 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500/40 focus:border-violet-300 transition-all bg-white/80 dark:bg-gray-800/80 dark:text-gray-100 backdrop-blur-sm placeholder:text-gray-300 dark:placeholder:text-gray-500"
                  placeholder={t('forgotPassword.emailPlaceholder')}
                  autoFocus
                />
              </div>
              {error && (
                <div className="bg-rose-50 dark:bg-rose-900/30 border border-rose-200 dark:border-rose-700 text-rose-700 dark:text-rose-400 rounded-xl px-4 py-3 text-sm">{error}</div>
              )}
              <button
                type="submit"
                disabled={loading}
                className="w-full bg-gradient-to-r from-violet-600 to-purple-600 hover:from-violet-700 hover:to-purple-700 disabled:opacity-50 text-white font-semibold rounded-xl py-3 text-sm transition-all shadow-lg shadow-violet-500/25 hover:shadow-violet-500/40 active:scale-[0.98]"
              >
                {loading ? t('forgotPassword.submitting') : t('forgotPassword.submit')}
              </button>
            </form>
            <div className="text-center mt-5">
              <Link to="/login" className="text-xs text-gray-400 dark:text-gray-500 hover:text-violet-600 dark:hover:text-violet-400 transition-colors font-medium">
                {t('forgotPassword.backToLogin')}
              </Link>
            </div>
          </>
        )}
      </div>
      <div className="flex-[3]" />
    </div>
  )
}
