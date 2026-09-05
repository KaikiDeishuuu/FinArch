import { useState } from 'react'
import type { FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { forgotPassword } from '../api/client'
import { useThemeColor } from '../hooks/useThemeColor'
import { LogoMark } from '../components/Brand'

export default function ForgotPasswordPage() {
  useThemeColor('#F3F6F2', '#17201D')
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
    <main data-mode="work" className="relative flex min-h-dvh items-center justify-center overflow-y-auto overflow-x-hidden bg-[var(--draft)] px-4 py-8 font-sans text-[hsl(var(--foreground))] dark:bg-[var(--ink)] sm:py-12">
      <div className="pointer-events-none fixed inset-y-0 left-[12%] w-px bg-[var(--rule)] opacity-40 dark:opacity-15" aria-hidden="true" />
      <section className="ledger-panel relative z-10 w-full max-w-md overflow-hidden rounded-[4px] border border-[var(--rule)] bg-[hsl(var(--card))]">
        <div className="h-1 bg-[var(--work)]" aria-hidden="true" />
        <header className="flex items-center gap-3 border-b border-[var(--rule)] px-6 py-5 sm:px-8">
          <LogoMark size={40} className="rounded-[3px]" />
          <div>
            <h1 className="font-serif text-xl font-semibold tracking-tight text-[hsl(var(--foreground))]">{t('forgotPassword.title')}</h1>
            <p className="mt-0.5 text-xs text-[hsl(var(--muted-foreground))]">{t('login.subtitle')}</p>
          </div>
        </header>

        <div className="px-6 py-7 sm:px-8">
          {sent ? (
            <div className="text-center">
              <div className="mx-auto mb-5 flex h-12 w-12 items-center justify-center rounded-[3px] border border-[var(--life)]/35 bg-[var(--life)]/10 text-[var(--life)]">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} className="h-6 w-6">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M4.5 12.75l6 6 9-13.5"/>
                </svg>
              </div>
              <h2 className="mb-2 font-serif text-xl font-semibold text-[hsl(var(--foreground))]">{t('forgotPassword.sentTitle')}</h2>
              <p className="mb-1 text-sm leading-relaxed text-[hsl(var(--muted-foreground))]">{t('forgotPassword.sentDesc')}</p>
              <p className="mb-6 text-xs text-[hsl(var(--muted-foreground))]">{t('forgotPassword.sentHint')}</p>
              <Link to="/login" className="fin-button inline-flex">
                {t('forgotPassword.backToLogin')}
              </Link>
            </div>
          ) : (
            <>
              <p className="mb-5 text-sm text-[hsl(var(--muted-foreground))]">
                {t('forgotPassword.desc')}
              </p>
              <form onSubmit={handleSubmit} className="space-y-4">
                <div>
                  <label className="mb-1.5 block text-xs font-semibold text-[hsl(var(--foreground))]">{t('forgotPassword.emailLabel')}</label>
                  <input
                    type="email"
                    required
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    className="fin-input w-full rounded-[3px] border-[var(--rule)] bg-[hsl(var(--card))] px-3.5 py-3 text-sm"
                    placeholder={t('forgotPassword.emailPlaceholder')}
                    autoFocus
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
                  {loading ? t('forgotPassword.submitting') : t('forgotPassword.submit')}
                </button>
              </form>
              <div className="mt-5 border-t border-[var(--rule)] pt-4 text-center">
                <Link to="/login" className="text-xs font-semibold text-[var(--work)] underline-offset-4 hover:underline">
                  {t('forgotPassword.backToLogin')}
                </Link>
              </div>
            </>
          )}
        </div>
      </section>
    </main>
  )
}
