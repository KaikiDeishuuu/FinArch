import { useState } from 'react'
import type { FormEvent } from 'react'
import { useSearchParams, Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { resetPassword } from '../api/client'
import { useThemeColor } from '../hooks/useThemeColor'
import { LogoMark } from '../components/Brand'

export default function ResetPasswordPage() {
  useThemeColor('#7c3aed')
  const { t } = useTranslation()
  const [searchParams] = useSearchParams()
  const token = searchParams.get('token') ?? ''

  const [newPassword, setNewPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [loading, setLoading] = useState(false)
  const [success, setSuccess] = useState(false)
  const [error, setError] = useState('')

  if (!token) {
    return (
      <div className="min-h-dvh flex flex-col overflow-y-auto overflow-x-hidden bg-gradient-to-br from-violet-600 via-purple-600 to-fuchsia-500 relative px-4 py-4 md:py-6">
        <div className="flex-[1]" />
        <div className="mx-auto w-full max-w-md shrink-0 bg-white/95 dark:bg-gray-900/95 backdrop-blur-xl rounded-3xl shadow-2xl shadow-violet-900/20 p-8 text-center relative z-10">
          <p className="text-rose-600 dark:text-rose-400 text-sm mb-4">{t('resetPassword.invalidLink')}</p>
          <Link to="/forgot-password" className="text-sm text-violet-600 dark:text-violet-400 hover:underline">{t('resetPassword.reapply')}</Link>
        </div>
        <div className="flex-[3]" />
      </div>
    )
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError('')
    if (newPassword !== confirm) {
      setError(t('resetPassword.toast.mismatch'))
      return
    }
    setLoading(true)
    try {
      await resetPassword(token, newPassword)
      setSuccess(true)
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      setError(msg || t('resetPassword.toast.error'))
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
          <h1 className="text-xl font-bold text-gray-800 dark:text-gray-100">{t('resetPassword.title')}</h1>
          <p className="text-xs text-gray-400 dark:text-gray-500 mt-0.5">{t('login.subtitle')}</p>
        </div>

        {success ? (
          <div className="text-center">
            <div className="w-14 h-14 rounded-2xl bg-gradient-to-br from-emerald-400 to-emerald-500 flex items-center justify-center mx-auto mb-5 shadow-lg shadow-emerald-500/25">
              <svg viewBox="0 0 24 24" fill="none" stroke="white" strokeWidth={2} className="w-7 h-7">
                <path strokeLinecap="round" strokeLinejoin="round" d="M4.5 12.75l6 6 9-13.5"/>
              </svg>
            </div>
            <h2 className="text-lg font-bold text-gray-800 dark:text-gray-100 mb-2">{t('resetPassword.success')}</h2>
            <p className="text-gray-400 dark:text-gray-500 text-sm mb-6">{t('resetPassword.successDesc')}</p>
            <Link to="/login" className="inline-block bg-gradient-to-r from-violet-600 to-purple-600 hover:from-violet-700 hover:to-purple-700 text-white text-sm font-semibold px-8 py-2.5 rounded-xl transition-all shadow-lg shadow-violet-500/25 hover:shadow-violet-500/40 active:scale-[0.98]">
              {t('resetPassword.backToLogin')}
            </Link>
          </div>
        ) : (
          <form onSubmit={handleSubmit} className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1.5">{t('resetPassword.newPassword')}</label>
              <input
                type="password"
                required
                minLength={8}
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                className="w-full border border-gray-200/80 dark:border-gray-700 rounded-xl px-3.5 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500/40 focus:border-violet-300 transition-all bg-white/80 dark:bg-gray-800/80 dark:text-gray-100 backdrop-blur-sm placeholder:text-gray-300 dark:placeholder:text-gray-500"
                placeholder={t('resetPassword.passwordPlaceholder')}
                autoFocus
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1.5">{t('resetPassword.confirmPassword')}</label>
              <input
                type="password"
                required
                minLength={8}
                value={confirm}
                onChange={(e) => setConfirm(e.target.value)}
                className="w-full border border-gray-200/80 dark:border-gray-700 rounded-xl px-3.5 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500/40 focus:border-violet-300 transition-all bg-white/80 dark:bg-gray-800/80 dark:text-gray-100 backdrop-blur-sm placeholder:text-gray-300 dark:placeholder:text-gray-500"
                placeholder={t('resetPassword.confirmPlaceholder')}
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
              {loading ? t('resetPassword.submitting') : t('resetPassword.submit')}
            </button>
          </form>
        )}
      </div>
      <div className="flex-[3]" />
    </div>
  )
}
