import { useState } from 'react'
import type { FormEvent, ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { resetPassword } from '../api/client'
import { useAuth } from '../hooks/useAuth'
import { useThemeColor } from '../hooks/useThemeColor'
import { LogoMark } from '../components/Brand'
import { useActionToken } from '../hooks/useActionToken'

function ResetPasswordShell({ title, subtitle, children }: { title: string; subtitle: string; children: ReactNode }) {
  return (
    <main data-mode="work" className="relative flex min-h-dvh items-center justify-center overflow-y-auto overflow-x-hidden bg-[var(--draft)] px-4 py-8 font-sans text-[hsl(var(--foreground))] dark:bg-[var(--ink)] sm:py-12">
      <div className="pointer-events-none fixed inset-y-0 left-[12%] w-px bg-[var(--rule)] opacity-40 dark:opacity-15" aria-hidden="true" />
      <section className="ledger-panel relative z-10 w-full max-w-md overflow-hidden rounded-[4px] border border-[var(--rule)] bg-[hsl(var(--card))]">
        <div className="h-1 bg-[var(--work)]" aria-hidden="true" />
        <header className="flex items-center gap-3 border-b border-[var(--rule)] px-6 py-5 sm:px-8">
          <LogoMark size={40} className="rounded-[3px]" />
          <div>
            <h1 className="font-serif text-xl font-semibold tracking-tight text-[hsl(var(--foreground))]">{title}</h1>
            <p className="mt-0.5 text-xs text-[hsl(var(--muted-foreground))]">{subtitle}</p>
          </div>
        </header>
        <div className="px-6 py-7 sm:px-8">{children}</div>
      </section>
    </main>
  )
}

export default function ResetPasswordPage() {
  useThemeColor('#F3F6F2', '#17201D')
  const { t } = useTranslation()
  const { clearSession } = useAuth()
  const token = useActionToken()

  const [newPassword, setNewPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [loading, setLoading] = useState(false)
  const [success, setSuccess] = useState(false)
  const [error, setError] = useState('')

  if (!token) {
    return (
      <ResetPasswordShell title={t('resetPassword.title')} subtitle={t('login.subtitle')}>
        <div className="text-center">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-[3px] border border-[var(--oxide)]/35 bg-[var(--oxide)]/10 text-[var(--oxide)]" aria-hidden="true">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} className="h-6 w-6">
              <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </div>
          <p className="mb-4 text-sm text-[var(--oxide)]">{t('resetPassword.invalidLink')}</p>
          <Link to="/forgot-password" className="text-sm font-semibold text-[var(--work)] underline-offset-4 hover:underline">{t('resetPassword.reapply')}</Link>
        </div>
      </ResetPasswordShell>
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
      clearSession()
      setSuccess(true)
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      setError(msg || t('resetPassword.toast.error'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <ResetPasswordShell title={t('resetPassword.title')} subtitle={t('login.subtitle')}>
      {success ? (
        <div className="text-center">
            <div className="mx-auto mb-5 flex h-12 w-12 items-center justify-center rounded-[3px] border border-[var(--life)]/35 bg-[var(--life)]/10 text-[var(--life)]">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} className="h-6 w-6">
                <path strokeLinecap="round" strokeLinejoin="round" d="M4.5 12.75l6 6 9-13.5"/>
              </svg>
            </div>
            <h2 className="mb-2 font-serif text-xl font-semibold text-[hsl(var(--foreground))]">{t('resetPassword.success')}</h2>
            <p className="mb-6 text-sm text-[hsl(var(--muted-foreground))]">{t('resetPassword.successDesc')}</p>
            <Link to="/login" className="fin-button inline-flex">
              {t('resetPassword.backToLogin')}
            </Link>
        </div>
      ) : (
        <form onSubmit={handleSubmit} className="space-y-4">
            <div>
              <label className="mb-1.5 block text-xs font-semibold text-[hsl(var(--foreground))]">{t('resetPassword.newPassword')}</label>
              <input
                type="password"
                required
                minLength={8}
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                className="fin-input w-full rounded-[3px] border-[var(--rule)] bg-[hsl(var(--card))] px-3.5 py-3 text-sm"
                placeholder={t('resetPassword.passwordPlaceholder')}
                autoFocus
              />
            </div>
            <div>
              <label className="mb-1.5 block text-xs font-semibold text-[hsl(var(--foreground))]">{t('resetPassword.confirmPassword')}</label>
              <input
                type="password"
                required
                minLength={8}
                value={confirm}
                onChange={(e) => setConfirm(e.target.value)}
                className="fin-input w-full rounded-[3px] border-[var(--rule)] bg-[hsl(var(--card))] px-3.5 py-3 text-sm"
                placeholder={t('resetPassword.confirmPlaceholder')}
              />
            </div>
            {error && (
              <div className="rounded-[3px] border border-[var(--oxide)]/35 bg-[var(--oxide)]/10 px-4 py-3 text-sm text-[var(--oxide)]">{error}</div>
            )}
            <button
              type="submit"
              disabled={loading}
              className="fin-button w-full disabled:opacity-50"
            >
              {loading ? t('resetPassword.submitting') : t('resetPassword.submit')}
            </button>
        </form>
      )}
    </ResetPasswordShell>
  )
}
