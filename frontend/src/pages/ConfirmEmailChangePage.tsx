import { useEffect, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { confirmEmailChange } from '../api/client'
import { useAuth } from '../hooks/useAuth'
import { LogoMark } from '../components/Brand'
import { useThemeColor } from '../hooks/useThemeColor'
import { useActionToken } from '../hooks/useActionToken'

export default function ConfirmEmailChangePage() {
  useThemeColor('#F3F6F2', '#17201D')
  const { t } = useTranslation()
  const { clearSession } = useAuth()
  const token = useActionToken()
  const navigate = useNavigate()

  const [status, setStatus] = useState<'ready' | 'loading' | 'success' | 'error'>(token ? 'ready' : 'error')
  const [errorMsg, setErrorMsg] = useState('')
  const submittingRef = useRef(false)
  const redirectTimerRef = useRef<number | null>(null)
  const visibleErrorMsg = token ? errorMsg : t('confirmEmailChange.invalidLink')

  async function handleConfirm() {
    if (!token || submittingRef.current) return
    submittingRef.current = true
    setStatus('loading')
    setErrorMsg('')
    try {
      await confirmEmailChange(token)
      clearSession()
      setStatus('success')
      redirectTimerRef.current = window.setTimeout(
        () => navigate('/login?email_changed=1', { replace: true }),
        3000,
      )
    } catch (err: unknown) {
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message
      setErrorMsg(msg || t('confirmEmailChange.errorDefault'))
      setStatus('error')
    } finally {
      submittingRef.current = false
    }
  }

  useEffect(() => () => {
    if (redirectTimerRef.current !== null) window.clearTimeout(redirectTimerRef.current)
  }, [])

  return (
    <main data-mode="work" className="relative flex min-h-dvh items-center justify-center overflow-y-auto overflow-x-hidden bg-[var(--draft)] px-4 py-8 font-sans text-[hsl(var(--foreground))] dark:bg-[var(--ink)] sm:py-12">
      <div className="pointer-events-none fixed inset-y-0 left-[12%] w-px bg-[var(--rule)] opacity-40 dark:opacity-15" aria-hidden="true" />
      <section className="ledger-panel relative z-10 w-full max-w-md overflow-hidden rounded-[4px] border border-[var(--rule)] bg-[hsl(var(--card))] text-center">
        <div className="h-1 bg-[var(--work)]" aria-hidden="true" />
        <header className="flex items-center gap-3 border-b border-[var(--rule)] px-6 py-5 text-left sm:px-8">
          <LogoMark size={40} className="rounded-[3px]" />
          <div>
            <h1 className="font-serif text-xl font-semibold tracking-tight text-[hsl(var(--foreground))]">FinArch</h1>
            <p className="mt-0.5 text-xs text-[hsl(var(--muted-foreground))]">{t('login.subtitle')}</p>
          </div>
        </header>

        <div className="px-6 py-7 sm:px-8">

        {status === 'ready' && (
          <div className="space-y-5">
            <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-[3px] border border-[var(--work)]/35 bg-[var(--work)]/10 text-[var(--work)]">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} className="w-7 h-7">
                <path strokeLinecap="round" strokeLinejoin="round" d="M7.5 12l3 3 6-6m4.5 3a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
            </div>
            <div className="space-y-2">
              <h2 className="font-serif text-xl font-semibold text-[hsl(var(--foreground))]">{t('confirmEmailChange.readyTitle')}</h2>
              <p className="text-sm leading-relaxed text-[hsl(var(--muted-foreground))]">{t('confirmEmailChange.readyDesc')}</p>
            </div>
            <button
              type="button"
              onClick={handleConfirm}
              className="fin-button w-full"
            >
              {t('confirmEmailChange.confirmButton')}
            </button>
            <Link to="/login" className="inline-block text-xs font-semibold text-[var(--work)] underline-offset-4 hover:underline">
              {t('confirmEmailChange.cancelButton')}
            </Link>
          </div>
        )}

        {status === 'loading' && (
          <div className="space-y-3">
            <div className="mx-auto h-9 w-9 animate-spin rounded-full border-2 border-[var(--rule)] border-t-[var(--work)]" />
            <p className="text-sm text-[hsl(var(--muted-foreground))]">{t('confirmEmailChange.processing')}</p>
          </div>
        )}

        {status === 'success' && (
          <div className="space-y-4">
            <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-[3px] border border-[var(--life)]/35 bg-[var(--life)]/10 text-[var(--life)]">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} className="h-6 w-6">
                <path strokeLinecap="round" strokeLinejoin="round" d="M4.5 12.75l6 6 9-13.5" />
              </svg>
            </div>
            <h2 className="font-serif text-xl font-semibold text-[hsl(var(--foreground))]">{t('confirmEmailChange.success')}</h2>
            <p className="text-sm leading-relaxed text-[hsl(var(--muted-foreground))]">
              {t('confirmEmailChange.successDesc')}
            </p>
            <p className="font-mono text-xs text-[hsl(var(--muted-foreground))]">{t('confirmEmailChange.redirecting')}</p>
            <Link
              to="/login?email_changed=1"
              className="fin-button inline-flex"
            >
              {t('confirmEmailChange.loginNow')}
            </Link>
          </div>
        )}

        {status === 'error' && (
          <div className="space-y-4">
            <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-[3px] border border-[var(--oxide)]/35 bg-[var(--oxide)]/10 text-[var(--oxide)]">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} className="h-6 w-6">
                <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </div>
            <h2 className="font-serif text-xl font-semibold text-[hsl(var(--foreground))]">{t('confirmEmailChange.errorTitle')}</h2>
            <p className="text-sm text-[var(--oxide)]">{visibleErrorMsg}</p>
            <div className="flex flex-col gap-2">
              <Link
                to="/settings"
                className="fin-button inline-flex"
              >
                {t('confirmEmailChange.backToSettings')}
              </Link>
              <Link to="/login" className="text-xs font-semibold text-[var(--work)] underline-offset-4 hover:underline">
                {t('confirmEmailChange.backToLogin')}
              </Link>
            </div>
          </div>
        )}
        </div>
      </section>
    </main>
  )
}
