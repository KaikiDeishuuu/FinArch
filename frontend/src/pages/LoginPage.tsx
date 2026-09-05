import { useState, useRef } from 'react'
import type { FormEvent, ReactNode } from 'react'
import { useNavigate, useSearchParams, Link } from 'react-router-dom'
import { Turnstile } from '@marsidev/react-turnstile'
import type { TurnstileInstance } from '@marsidev/react-turnstile'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../hooks/useAuth'
import { useConfig } from '../hooks/useConfig'
import { resendVerification } from '../api/client'
import { LogoMark } from '../components/Brand'
import { useThemeColor } from '../hooks/useThemeColor'
import { getApiError } from '../lib/errors'

type Strength = 'none' | 'weak' | 'medium' | 'strong'

function calcStrength(pw: string): Strength {
  if (!pw) return 'none'
  if (pw.length < 8) return 'weak'
  if (/^\d+$/.test(pw)) return 'weak'
  let score = 0
  if (/[a-z]/.test(pw)) score++
  if (/[A-Z]/.test(pw)) score++
  if (/[0-9]/.test(pw)) score++
  if (/[^a-zA-Z0-9]/.test(pw)) score++
  if (score <= 1) return 'weak'
  if (score === 2) return 'medium'
  return 'strong'
}

function PasswordStrength({ password }: { password: string }) {
  const { t } = useTranslation()
  const s = calcStrength(password)
  if (!password) return null
  const bar = { none: 'w-0', weak: 'w-1/3', medium: 'w-2/3', strong: 'w-full' }[s]
  const color = { none: '', weak: 'bg-[var(--oxide)]', medium: 'bg-[hsl(var(--pending))]', strong: 'bg-[var(--life)]' }[s]
  const label = { none: '', weak: t('login.passwordStrength.weakHint'), medium: t('login.passwordStrength.mediumHint'), strong: t('login.passwordStrength.strong') }[s]
  const tc = { none: '', weak: 'text-[var(--oxide)]', medium: 'text-[hsl(var(--pending))]', strong: 'text-[var(--life)]' }[s]

  return (
    <div className="mt-2 space-y-1 font-sans">
      <div className="h-1 w-full overflow-hidden bg-[var(--field)] dark:bg-white/10">
        <div className={`h-full transition-[width] duration-300 ${color} ${bar}`} />
      </div>
      {s !== 'none' && <p className={`text-xs ${tc}`}>{label}</p>}
    </div>
  )
}

function LoginShell({ children }: { children: ReactNode }) {
  const { t } = useTranslation()

  return (
    <main
      data-mode="work"
      className="relative h-dvh overflow-x-hidden overflow-y-auto bg-[var(--draft)] font-sans text-[hsl(var(--foreground))] dark:bg-[var(--ink)]"
      style={{ paddingBottom: 'env(safe-area-inset-bottom)' }}
    >
      <div className="pointer-events-none fixed inset-y-0 left-[8%] w-px bg-[var(--rule)] opacity-40 dark:opacity-15" aria-hidden="true" />
      <div className="pointer-events-none fixed inset-y-0 right-[8%] w-px bg-[var(--rule)] opacity-40 dark:opacity-15" aria-hidden="true" />

      <div className="relative z-10 flex min-h-dvh items-start justify-center px-3 py-3 sm:px-6 sm:py-8 lg:items-center">
        <div className="ledger-panel mx-auto grid w-full max-w-5xl overflow-hidden rounded-[6px] border border-[var(--rule)] bg-[hsl(var(--card))] lg:grid-cols-[0.88fr_1.12fr]">
          <aside className="relative flex min-h-44 flex-col overflow-hidden bg-[var(--ink)] px-7 py-6 text-[#f3f6f2] sm:px-9 lg:min-h-[640px] lg:px-12 lg:py-10">
            <div className="absolute inset-y-0 left-4 w-px bg-white/15" aria-hidden="true" />
            <div className="absolute inset-y-0 left-7 w-px bg-white/10" aria-hidden="true" />
            <div className="relative flex items-center gap-4">
              <LogoMark size={46} className="rounded-[3px] ring-1 ring-white/20" />
              <div>
                <h1 className="font-serif text-3xl font-semibold tracking-[-0.03em] sm:text-4xl" style={{ color: '#f3f6f2' }}>FinArch</h1>
                <p className="mt-1 text-xs tracking-[0.12em] text-white/60">{t('login.subtitle')}</p>
              </div>
            </div>

            <div className="relative mt-8 hidden flex-1 lg:block" aria-hidden="true">
              <div className="absolute inset-x-0 top-1/3 border-t border-white/20" />
              <div className="absolute inset-x-0 top-1/2 border-t border-white/10" />
              <div className="absolute inset-x-0 top-2/3 border-t border-white/20" />
              <div className="absolute inset-y-[18%] left-[28%] border-l border-white/10" />
              <div className="absolute inset-y-[18%] left-[64%] border-l border-white/10" />
              <div className="absolute bottom-[18%] left-[28%] h-16 w-3 bg-[var(--work)]" />
              <div className="absolute bottom-[18%] left-[46%] h-28 w-3 bg-[var(--life)]" />
              <div className="absolute bottom-[18%] left-[64%] h-40 w-3 bg-[var(--oxide)]" />
            </div>

            <div className="relative mt-auto hidden items-center gap-3 border-t border-white/20 pt-4 lg:flex" aria-hidden="true">
              <span className="h-2 w-2 bg-[var(--work)]" />
              <span className="h-2 w-2 bg-[var(--life)]" />
              <span className="h-2 w-2 bg-[var(--oxide)]" />
              <span className="ml-auto font-mono text-[10px] tracking-[0.18em] text-white/45">FA / 02.3</span>
            </div>
          </aside>

          <section className="bg-[hsl(var(--card))] p-5 sm:p-8 lg:flex lg:flex-col lg:justify-center lg:p-10">
            {children}
          </section>
        </div>
      </div>
    </main>
  )
}

function StatusMessage({ children, tone = 'neutral' }: { children: ReactNode; tone?: 'neutral' | 'success' | 'error' | 'warning' }) {
  const toneClass = {
    neutral: 'border-[var(--rule)] bg-[var(--draft)] text-[hsl(var(--muted-foreground))] dark:bg-white/[0.04]',
    success: 'border-[var(--life)]/35 bg-[var(--life)]/10 text-[var(--life)]',
    error: 'border-[var(--oxide)]/35 bg-[var(--oxide)]/10 text-[var(--oxide)]',
    warning: 'border-[hsl(var(--pending))]/35 bg-[hsl(var(--pending))]/10 text-[hsl(var(--foreground))]',
  }[tone]

  return <div className={`rounded-[3px] border px-3 py-2.5 text-sm ${toneClass}`}>{children}</div>
}

export default function LoginPage() {
  const { t, i18n } = useTranslation()
  const { login, register } = useAuth()
  const { turnstileSiteKey, captchaEnabled, loaded: configLoaded, loadError: configLoadError } = useConfig()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()

  const [mode, setMode] = useState<'login' | 'register'>('login')
  const [email, setEmail] = useState('')
  const [username, setUsername] = useState('')
  const [nickname, setNickname] = useState('')
  const [password, setPassword] = useState('')
  const [captchaToken, setCaptchaToken] = useState<string>('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [pendingVerification, setPendingVerification] = useState(false)
  const [unverifiedEmail, setUnverifiedEmail] = useState('')
  const [resendLoading, setResendLoading] = useState(false)
  const [resendDone, setResendDone] = useState(false)
  const turnstileRef = useRef<TurnstileInstance>(null)

  useThemeColor('#F3F6F2', '#17201D')

  const justVerified = searchParams.get('verified') === '1'
  const tokenError = searchParams.get('error') === 'invalid_token'
  const accountDeleted = searchParams.get('deleted') === '1'
  const emailChanged = searchParams.get('email_changed') === '1'
  const passwordChanged = searchParams.get('password_changed') === '1'
  const captchaRequired = captchaEnabled && !!turnstileSiteKey
  const captchaUnavailable = configLoadError || (captchaEnabled && !turnstileSiteKey)

  function switchMode(next: 'login' | 'register') {
    if (next === mode) return
    setMode(next)
    setError('')
    setPendingVerification(false)
    setUnverifiedEmail('')
    setCaptchaToken('')
    turnstileRef.current?.reset()
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError('')
    setPendingVerification(false)
    if (!configLoaded) {
      setError(t('login.configLoading'))
      return
    }
    if (captchaUnavailable) {
      setError(t('login.configLoadError'))
      return
    }
    if (captchaRequired && !captchaToken) {
      setError(t('login.captchaError'))
      return
    }

    setLoading(true)
    try {
      if (mode === 'login') {
        await login({ email, password, captcha_token: captchaToken || undefined })
        navigate('/')
      } else {
        const pending = await register({ email, username, password, nickname: nickname || undefined, captcha_token: captchaToken || undefined })
        if (pending) {
          setPendingVerification(true)
        } else {
          navigate('/')
        }
      }
    } catch (err: unknown) {
      const apiError = getApiError(err)
      if (apiError.code === 'email_not_verified') {
        setUnverifiedEmail(email)
      } else {
        setError(apiError.message || t('login.operationFailed'))
      }
      setCaptchaToken('')
      turnstileRef.current?.reset()
    } finally {
      setLoading(false)
    }
  }

  async function handleResend() {
    setResendLoading(true)
    try {
      await resendVerification(unverifiedEmail || email)
      setResendDone(true)
    } finally {
      setResendLoading(false)
    }
  }

  const inputClass = 'fin-input w-full rounded-[3px] border-[var(--rule)] bg-[hsl(var(--card))] px-3.5 py-3 font-sans text-[15px] text-[hsl(var(--foreground))] outline-none placeholder:text-[hsl(var(--muted-foreground))] focus:border-[var(--work)] focus:ring-2 focus:ring-[var(--work)]/20'

  if (pendingVerification) {
    return (
      <LoginShell>
        <div className="space-y-5 py-3 text-center">
          <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-[3px] border border-[var(--life)]/35 bg-[var(--life)]/10 text-[var(--life)]">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5} className="h-7 w-7">
              <path strokeLinecap="round" strokeLinejoin="round" d="M21.75 6.75v10.5a2.25 2.25 0 01-2.25 2.25h-15a2.25 2.25 0 01-2.25-2.25V6.75m19.5 0A2.25 2.25 0 0019.5 4.5h-15a2.25 2.25 0 00-2.25 2.25m19.5 0v.243a2.25 2.25 0 01-1.07 1.916l-7.5 4.615a2.25 2.25 0 01-2.36 0L3.32 8.91a2.25 2.25 0 01-1.07-1.916V6.75" />
            </svg>
          </div>
          <h2 className="font-serif text-2xl font-semibold text-[hsl(var(--foreground))]">{t('login.verification.title')}</h2>
          <p className="text-sm leading-relaxed text-[hsl(var(--muted-foreground))]">{t('login.verification.desc', { email })}</p>

          {!resendDone ? (
            <button
              onClick={handleResend}
              disabled={resendLoading}
              className="fin-button w-full disabled:opacity-50"
            >
              {resendLoading ? t('login.sending') : t('login.verification.noEmail')}
            </button>
          ) : (
            <StatusMessage tone="success">{t('login.verification.resent')}</StatusMessage>
          )}

          <button onClick={() => switchMode('login')} className="fin-button fin-button--ghost w-full text-xs">
            {t('login.backToLogin')}
          </button>
        </div>
      </LoginShell>
    )
  }

  return (
    <LoginShell>
      <div className="space-y-5">
        <div className="border-b border-[var(--rule)] pb-4">
          <p className="page-kicker">FinArch</p>
          <p className="mt-1 text-sm text-[hsl(var(--muted-foreground))]">{t('login.subtitle')}</p>
        </div>

        <div className="space-y-2">
          {justVerified && <StatusMessage tone="success">{t('login.status.verified')}</StatusMessage>}
          {accountDeleted && <StatusMessage>{t('login.status.deleted')}</StatusMessage>}
          {emailChanged && <StatusMessage tone="success">{t('login.status.emailChanged')}</StatusMessage>}
          {passwordChanged && <StatusMessage tone="success">{t('login.status.passwordChanged')}</StatusMessage>}
          {tokenError && <StatusMessage tone="error">{t('login.status.tokenError')}</StatusMessage>}
        </div>

        <div className="border-b border-[var(--rule)]">
          <div className="grid grid-cols-2">
            <button
              type="button"
              aria-pressed={mode === 'login'}
              className={`border-b-2 px-3 py-2.5 text-sm font-semibold transition-colors ${mode === 'login' ? 'border-[var(--work)] text-[var(--work)]' : 'border-transparent text-[hsl(var(--muted-foreground))] hover:text-[hsl(var(--foreground))]'}`}
              onClick={() => switchMode('login')}
            >
              {t('login.tabs.login')}
            </button>
            <button
              type="button"
              aria-pressed={mode === 'register'}
              className={`border-b-2 px-3 py-2.5 text-sm font-semibold transition-colors ${mode === 'register' ? 'border-[var(--work)] text-[var(--work)]' : 'border-transparent text-[hsl(var(--muted-foreground))] hover:text-[hsl(var(--foreground))]'}`}
              onClick={() => switchMode('register')}
            >
              {t('login.tabs.register')}
            </button>
          </div>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          {mode === 'register' && (
            <div>
              <label className="mb-1.5 block text-xs font-semibold text-[hsl(var(--foreground))]">{t('login.fields.username')}</label>
              <input type="text" required value={username} onChange={(e) => setUsername(e.target.value)} className={inputClass} placeholder={t('login.fields.usernamePlaceholder')} autoComplete="username" />
              <p className="mt-1 text-[11px] text-[hsl(var(--muted-foreground))]">{t('login.fields.usernameHint')}</p>
            </div>
          )}

          {mode === 'register' && (
            <div>
              <label className="mb-1.5 block text-xs font-semibold text-[hsl(var(--foreground))]">
                {t('login.fields.nickname')} <span className="font-normal text-[hsl(var(--muted-foreground))]">({t('login.fields.nicknameOptional')})</span>
              </label>
              <input type="text" value={nickname} onChange={(e) => setNickname(e.target.value)} className={inputClass} placeholder={t('login.fields.nicknamePlaceholder')} maxLength={20} />
              <p className="mt-1 text-[11px] text-[hsl(var(--muted-foreground))]">{t('login.fields.nicknameHint')}</p>
            </div>
          )}

          <div>
            <label className="mb-1.5 block text-xs font-semibold text-[hsl(var(--foreground))]">{t('login.fields.email')}</label>
            <input type="email" required value={email} onChange={(e) => setEmail(e.target.value)} className={inputClass} placeholder="user@example.com" autoComplete="email" />
          </div>

          <div>
            <label className="mb-1.5 block text-xs font-semibold text-[hsl(var(--foreground))]">{t('login.fields.password')}</label>
            <input
              type="password"
              required
              minLength={8}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className={inputClass}
              placeholder={mode === 'register' ? t('login.fields.passwordPlaceholderRegister') : t('login.fields.passwordPlaceholderLogin')}
              autoComplete={mode === 'login' ? 'current-password' : 'new-password'}
            />
            {mode === 'register' && <PasswordStrength password={password} />}
          </div>

          {captchaRequired && configLoaded && (
            <div className="rounded-[3px] border border-[var(--rule)] bg-[var(--draft)] p-2 dark:bg-white/[0.04]">
              <div className="mx-auto max-w-full overflow-hidden" style={{ minHeight: 70 }}>
                <Turnstile
                  ref={turnstileRef}
                  siteKey={turnstileSiteKey}
                  onSuccess={(token) => setCaptchaToken(token)}
                  onExpire={() => setCaptchaToken('')}
                  onError={() => {
                    setCaptchaToken('')
                    setError(t('login.captchaLoadError'))
                  }}
                  options={{ theme: 'light', language: i18n.language === 'en' ? 'en' : 'zh-cn', size: 'flexible' }}
                />
              </div>
            </div>
          )}

          {captchaUnavailable && <StatusMessage tone="error">{t('login.configLoadError')}</StatusMessage>}
          {error && <StatusMessage tone="error">{error}</StatusMessage>}

          {unverifiedEmail && (
            <StatusMessage tone="warning">
              <div className="space-y-1.5">
                <p>{t('login.unverifiedHint')}</p>
                {!resendDone ? (
                  <button type="button" onClick={handleResend} disabled={resendLoading} className="text-xs font-semibold text-[var(--work)] underline-offset-4 hover:underline disabled:opacity-50">
                    {resendLoading ? t('login.sending') : t('login.verification.resend')}
                  </button>
                ) : (
                  <p className="text-xs font-medium text-[var(--life)]">{t('login.verification.resent')}</p>
                )}
              </div>
            </StatusMessage>
          )}

          <button
            type="submit"
            disabled={loading || !configLoaded || captchaUnavailable || (captchaRequired && !captchaToken)}
            className="fin-button w-full disabled:opacity-50"
          >
            {loading ? t('login.processing') : mode === 'login' ? t('login.submitLogin') : t('login.submitRegister')}
          </button>
        </form>

        {mode === 'login' && (
          <div className="space-y-1 text-center">
            <Link to="/forgot-password" className="block text-xs font-semibold text-[var(--work)] underline-offset-4 hover:underline">
              {t('login.forgotPassword')}
            </Link>
          </div>
        )}

        <div className="border-t border-[var(--rule)] pt-3 text-center">
          <p className="font-mono text-[10px] tracking-[0.14em] text-[hsl(var(--muted-foreground))]">POWERED BY FINARCH · v2.2</p>
        </div>
      </div>
    </LoginShell>
  )
}
