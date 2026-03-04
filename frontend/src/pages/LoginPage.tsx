import { useState, useRef } from 'react'
import type { FormEvent, ReactNode } from 'react'
import { useNavigate, useSearchParams, Link } from 'react-router-dom'
import { Turnstile } from '@marsidev/react-turnstile'
import type { TurnstileInstance } from '@marsidev/react-turnstile'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../contexts/AuthContext'
import { useConfig } from '../contexts/ConfigContext'
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
  const color = { none: '', weak: 'bg-rose-400', medium: 'bg-amber-400', strong: 'bg-emerald-500' }[s]
  const label = { none: '', weak: t('login.passwordStrength.weakHint'), medium: t('login.passwordStrength.mediumHint'), strong: t('login.passwordStrength.strong') }[s]
  const tc = { none: '', weak: 'text-rose-500', medium: 'text-amber-600 dark:text-amber-400', strong: 'text-emerald-500' }[s]

  return (
    <div className="mt-2 space-y-1">
      <div className="h-1.5 w-full bg-gray-100 dark:bg-gray-700 rounded-full overflow-hidden">
        <div className={`h-full rounded-full transition-all duration-300 ${color} ${bar}`} />
      </div>
      {s !== 'none' && <p className={`text-xs ${tc}`}>{label}</p>}
    </div>
  )
}

function LoginShell({ children }: { children: ReactNode }) {
  return (
    <div className="min-h-dvh overflow-x-hidden bg-gradient-to-b from-violet-700 via-purple-700 to-fuchsia-700" style={{ paddingBottom: 'env(safe-area-inset-bottom)' }}>
      <div className="fixed inset-0 pointer-events-none">
        <div className="absolute -top-28 -right-24 h-72 w-72 rounded-full bg-fuchsia-300/20 blur-3xl" />
        <div className="absolute top-1/3 -left-28 h-80 w-80 rounded-full bg-violet-300/25 blur-3xl" />
      </div>

      <div className="relative z-10 flex min-h-dvh items-start justify-center px-3 py-4 sm:px-6 sm:py-8 md:items-center">
        <div className="mx-auto w-full max-w-md overflow-y-auto rounded-3xl border border-white/15 bg-white/92 p-4 shadow-2xl shadow-violet-900/30 backdrop-blur-xl max-h-[calc(100dvh-2rem)] dark:border-gray-700/70 dark:bg-gray-900/92 sm:max-h-[calc(100dvh-4rem)] sm:p-6 md:max-h-none md:overflow-visible">
          {children}
        </div>
      </div>
    </div>
  )
}

function StatusMessage({ children, tone = 'neutral' }: { children: ReactNode; tone?: 'neutral' | 'success' | 'error' | 'warning' }) {
  const toneClass = {
    neutral: 'bg-gray-50 dark:bg-gray-800 border-gray-200 dark:border-gray-700 text-gray-600 dark:text-gray-300',
    success: 'bg-emerald-50 dark:bg-emerald-900/30 border-emerald-200 dark:border-emerald-700 text-emerald-700 dark:text-emerald-400',
    error: 'bg-rose-50 dark:bg-rose-900/30 border-rose-200 dark:border-rose-700 text-rose-700 dark:text-rose-400',
    warning: 'bg-amber-50 dark:bg-amber-900/30 border-amber-200 dark:border-amber-700 text-amber-800 dark:text-amber-400',
  }[tone]

  return <div className={`rounded-xl border px-3 py-2.5 text-sm ${toneClass}`}>{children}</div>
}

export default function LoginPage() {
  const { t, i18n } = useTranslation()
  const { login, register } = useAuth()
  const { turnstileSiteKey, loaded: configLoaded } = useConfig()
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

  useThemeColor('#6d28d9', '#160a2a')

  const justVerified = searchParams.get('verified') === '1'
  const tokenError = searchParams.get('error') === 'invalid_token'
  const accountDeleted = searchParams.get('deleted') === '1'
  const emailChanged = searchParams.get('email_changed') === '1'

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
    if (turnstileSiteKey && !captchaToken) {
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

  const inputClass = 'w-full rounded-xl border border-gray-200 px-3.5 py-3 text-[15px] text-gray-800 outline-none transition-all placeholder:text-gray-400 focus:border-violet-400 focus:ring-4 focus:ring-violet-500/15 dark:border-gray-700 dark:bg-gray-800/80 dark:text-gray-100 dark:placeholder:text-gray-500'

  if (pendingVerification) {
    return (
      <LoginShell>
        <div className="space-y-4 text-center py-3">
          <div className="mx-auto flex h-16 w-16 items-center justify-center rounded-2xl bg-gradient-to-br from-violet-500 to-purple-600 shadow-lg shadow-violet-500/30">
            <svg viewBox="0 0 24 24" fill="none" stroke="white" strokeWidth={1.5} className="w-8 h-8">
              <path strokeLinecap="round" strokeLinejoin="round" d="M21.75 6.75v10.5a2.25 2.25 0 01-2.25 2.25h-15a2.25 2.25 0 01-2.25-2.25V6.75m19.5 0A2.25 2.25 0 0019.5 4.5h-15a2.25 2.25 0 00-2.25 2.25m19.5 0v.243a2.25 2.25 0 01-1.07 1.916l-7.5 4.615a2.25 2.25 0 01-2.36 0L3.32 8.91a2.25 2.25 0 01-1.07-1.916V6.75" />
            </svg>
          </div>
          <h2 className="text-xl font-bold text-gray-800 dark:text-gray-100">{t('login.verification.title')}</h2>
          <p className="text-sm leading-relaxed text-gray-500 dark:text-gray-400">{t('login.verification.desc', { email })}</p>

          {!resendDone ? (
            <button
              onClick={handleResend}
              disabled={resendLoading}
              className="w-full rounded-xl bg-gradient-to-r from-violet-600 to-purple-600 py-3 text-sm font-semibold text-white transition-all hover:from-violet-700 hover:to-purple-700 disabled:opacity-50"
            >
              {resendLoading ? t('login.sending') : t('login.verification.noEmail')}
            </button>
          ) : (
            <StatusMessage tone="success">{t('login.verification.resent')}</StatusMessage>
          )}

          <button onClick={() => switchMode('login')} className="block w-full text-center text-xs font-medium text-gray-500 transition-colors hover:text-violet-600 dark:text-gray-400 dark:hover:text-violet-400">
            {t('login.backToLogin')}
          </button>
        </div>
      </LoginShell>
    )
  }

  return (
    <LoginShell>
      <div className="space-y-4 sm:space-y-5">
        <div className="text-center">
          <div className="relative mx-auto mb-3 flex w-fit">
            <LogoMark size={52} className="rounded-2xl shadow-lg shadow-violet-500/20" />
          </div>
          <h1 className="text-xl font-bold tracking-tight text-gray-900 dark:text-gray-100">FinArch</h1>
          <p className="mt-1 text-xs tracking-wide text-gray-500 dark:text-gray-400">{t('login.subtitle')}</p>
        </div>

        <div className="space-y-2">
          {justVerified && <StatusMessage tone="success">{t('login.status.verified')}</StatusMessage>}
          {accountDeleted && <StatusMessage>{t('login.status.deleted')}</StatusMessage>}
          {emailChanged && <StatusMessage tone="success">{t('login.status.emailChanged')}</StatusMessage>}
          {tokenError && <StatusMessage tone="error">{t('login.status.tokenError')}</StatusMessage>}
        </div>

        <div className="rounded-xl bg-gray-100 p-1 dark:bg-gray-800">
          <div className="grid grid-cols-2 gap-1">
            <button
              type="button"
              className={`rounded-lg py-2 text-sm font-semibold transition-all ${mode === 'login' ? 'bg-white text-violet-600 shadow-sm dark:bg-gray-700 dark:text-violet-400' : 'text-gray-500 hover:text-gray-700 dark:text-gray-300 dark:hover:text-gray-100'}`}
              onClick={() => switchMode('login')}
            >
              {t('login.tabs.login')}
            </button>
            <button
              type="button"
              className={`rounded-lg py-2 text-sm font-semibold transition-all ${mode === 'register' ? 'bg-white text-violet-600 shadow-sm dark:bg-gray-700 dark:text-violet-400' : 'text-gray-500 hover:text-gray-700 dark:text-gray-300 dark:hover:text-gray-100'}`}
              onClick={() => switchMode('register')}
            >
              {t('login.tabs.register')}
            </button>
          </div>
        </div>

        <form onSubmit={handleSubmit} className="space-y-3.5">
          {mode === 'register' && (
            <div>
              <label className="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">{t('login.fields.username')}</label>
              <input type="text" required value={username} onChange={(e) => setUsername(e.target.value)} className={inputClass} placeholder={t('login.fields.usernamePlaceholder')} autoComplete="username" />
              <p className="mt-1 text-[11px] text-gray-500 dark:text-gray-400">{t('login.fields.usernameHint')}</p>
            </div>
          )}

          {mode === 'register' && (
            <div>
              <label className="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">
                {t('login.fields.nickname')} <span className="font-normal text-gray-400">({t('login.fields.nicknameOptional')})</span>
              </label>
              <input type="text" value={nickname} onChange={(e) => setNickname(e.target.value)} className={inputClass} placeholder={t('login.fields.nicknamePlaceholder')} maxLength={20} />
              <p className="mt-1 text-[11px] text-gray-500 dark:text-gray-400">{t('login.fields.nicknameHint')}</p>
            </div>
          )}

          <div>
            <label className="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">{t('login.fields.email')}</label>
            <input type="email" required value={email} onChange={(e) => setEmail(e.target.value)} className={inputClass} placeholder="user@example.com" autoComplete="email" />
          </div>

          <div>
            <label className="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">{t('login.fields.password')}</label>
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

          {turnstileSiteKey && configLoaded && (
            <div className="rounded-xl border border-gray-200 bg-white/80 p-2 dark:border-gray-700 dark:bg-gray-800/70">
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

          {error && <StatusMessage tone="error">{error}</StatusMessage>}

          {unverifiedEmail && (
            <StatusMessage tone="warning">
              <div className="space-y-1.5">
                <p>{t('login.unverifiedHint')}</p>
                {!resendDone ? (
                  <button type="button" onClick={handleResend} disabled={resendLoading} className="text-xs font-medium text-violet-700 transition-colors hover:text-violet-800 hover:underline disabled:opacity-50 dark:text-violet-400 dark:hover:text-violet-300">
                    {resendLoading ? t('login.sending') : t('login.verification.resend')}
                  </button>
                ) : (
                  <p className="text-xs font-medium text-emerald-600 dark:text-emerald-400">{t('login.verification.resent')}</p>
                )}
              </div>
            </StatusMessage>
          )}

          <button
            type="submit"
            disabled={loading || (!!turnstileSiteKey && !captchaToken)}
            className="w-full rounded-xl bg-gradient-to-r from-violet-600 to-purple-600 py-3 text-sm font-semibold text-white shadow-lg shadow-violet-500/25 transition-all hover:from-violet-700 hover:to-purple-700 disabled:opacity-50"
          >
            {loading ? t('login.processing') : mode === 'login' ? t('login.submitLogin') : t('login.submitRegister')}
          </button>
        </form>

        {mode === 'login' && (
          <div className="space-y-1 text-center">
            <Link to="/forgot-password" className="block text-xs font-medium text-gray-500 transition-colors hover:text-violet-600 dark:text-gray-400 dark:hover:text-violet-400">
              {t('login.forgotPassword')}
            </Link>
            <Link to="/disaster-restore" className="block text-xs font-medium text-gray-500 transition-colors hover:text-violet-600 dark:text-gray-400 dark:hover:text-violet-400">
              {t('login.disasterRestore')}
            </Link>
          </div>
        )}

        <div className="border-t border-gray-100 pt-3 text-center dark:border-gray-800">
          <p className="text-[10px] tracking-wider text-gray-400 dark:text-gray-600">POWERED BY FINARCH · v2.2</p>
        </div>
      </div>
    </LoginShell>
  )
}
