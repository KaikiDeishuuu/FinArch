import { useState, useRef } from 'react'
import type { FormEvent } from 'react'
import { useNavigate, useSearchParams, Link } from 'react-router-dom'
import { Turnstile } from '@marsidev/react-turnstile'
import type { TurnstileInstance } from '@marsidev/react-turnstile'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../contexts/AuthContext'
import { useConfig } from '../contexts/ConfigContext'
import { resendVerification } from '../api/client'
import { LogoMark, BrandWatermark } from '../components/Brand'
import { useThemeColor } from '../hooks/useThemeColor'

// ─── Password Strength Indicator ─────────────────────────────────────────────
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
    <div className="mt-1.5 space-y-1">
      <div className="h-1 w-full bg-gray-100 dark:bg-gray-700 rounded-full overflow-hidden">
        <div className={`h-full rounded-full transition-all duration-300 ${color} ${bar}`} />
      </div>
      {s !== 'none' && <p className={`text-xs ${tc}`}>{label}</p>}
    </div>
  )
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

  // Match status-bar colour with purple login background
  useThemeColor('#7c3aed')

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
      const msg = (err as { response?: { data?: { message?: string } } })?.response?.data?.message ?? ''
      if (msg.includes('邮箱尚未验证') || msg.includes('email_not_verified')) {
        setUnverifiedEmail(email)
      } else {
        setError(msg || t('login.operationFailed'))
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

  const inputClass = 'w-full border border-gray-200/80 dark:border-gray-700 rounded-xl px-3.5 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500/40 focus:border-violet-300 dark:focus:border-violet-600 transition-all bg-white/80 dark:bg-gray-800/80 dark:text-gray-100 backdrop-blur-sm placeholder:text-gray-300 dark:placeholder:text-gray-500'

  if (pendingVerification) {
    return (
      <div className="min-h-dvh flex flex-col overflow-y-auto overflow-x-hidden bg-gradient-to-br from-violet-600 via-purple-600 to-fuchsia-500 relative px-4 py-4 md:py-6">
        {/* Decorative orbs */}
        <div className="fixed top-1/4 -left-20 w-64 h-64 bg-violet-400/30 rounded-full blur-3xl pointer-events-none" />
        <div className="fixed bottom-1/4 -right-20 w-72 h-72 bg-fuchsia-400/20 rounded-full blur-3xl pointer-events-none" />

        <div className="flex-[1]" />
        <div className="mx-auto w-full max-w-md shrink-0 bg-white/95 dark:bg-gray-900/95 backdrop-blur-xl rounded-3xl shadow-2xl shadow-violet-900/20 p-8 text-center relative z-10">
          <div className="w-16 h-16 rounded-2xl bg-gradient-to-br from-violet-500 to-purple-600 flex items-center justify-center mx-auto mb-5 shadow-lg shadow-violet-500/25">
            <svg viewBox="0 0 24 24" fill="none" stroke="white" strokeWidth={1.5} className="w-8 h-8">
              <path strokeLinecap="round" strokeLinejoin="round" d="M21.75 6.75v10.5a2.25 2.25 0 01-2.25 2.25h-15a2.25 2.25 0 01-2.25-2.25V6.75m19.5 0A2.25 2.25 0 0019.5 4.5h-15a2.25 2.25 0 00-2.25 2.25m19.5 0v.243a2.25 2.25 0 01-1.07 1.916l-7.5 4.615a2.25 2.25 0 01-2.36 0L3.32 8.91a2.25 2.25 0 01-1.07-1.916V6.75" />
          </svg>
          </div>
          <h2 className="text-xl font-bold text-gray-800 dark:text-gray-100 mb-2">{t('login.verification.title')}</h2>
          <p className="text-gray-500 dark:text-gray-400 text-sm mb-6 leading-relaxed">
            {t('login.verification.desc', { email })}
          </p>
          {!resendDone ? (
            <button onClick={handleResend} disabled={resendLoading}
              className="inline-block bg-gradient-to-r from-violet-600 to-purple-600 hover:from-violet-700 hover:to-purple-700 disabled:opacity-50 text-white text-sm font-semibold px-8 py-2.5 rounded-xl transition-all shadow-lg shadow-violet-500/25 hover:shadow-violet-500/40 active:scale-[0.98]">
              {resendLoading ? t('login.sending') : t('login.verification.noEmail')}
            </button>
          ) : (
            <div className="bg-emerald-50 dark:bg-emerald-900/30 border border-emerald-200 dark:border-emerald-700 text-emerald-700 dark:text-emerald-400 rounded-xl px-4 py-3 text-sm flex items-center justify-center gap-2">
              <svg viewBox="0 0 20 20" fill="currentColor" className="w-4 h-4 shrink-0"><path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" /></svg>
              <span>{t('login.verification.resent')}</span>
            </div>
          )}
          <button onClick={() => switchMode('login')}
            className="mt-5 block w-full text-center text-xs text-gray-400 hover:text-violet-600 dark:hover:text-violet-400 transition-colors font-medium">
            {t('login.backToLogin')}
          </button>
        </div>
        <div className="flex-[3]" />
      </div>
    )
  }

  return (
    <div className="min-h-dvh flex flex-col overflow-y-auto overflow-x-hidden bg-gradient-to-br from-violet-600 via-purple-600 to-fuchsia-500 relative px-4 py-4 md:py-6">
      {/* Decorative background orbs */}
      <div className="fixed top-10 -left-32 w-80 h-80 bg-violet-400/30 rounded-full blur-3xl animate-pulse pointer-events-none" style={{ animationDuration: '4s' }} />
      <div className="fixed -bottom-20 -right-32 w-96 h-96 bg-fuchsia-400/20 rounded-full blur-3xl animate-pulse pointer-events-none" style={{ animationDuration: '6s' }} />
      <div className="fixed top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[600px] h-[600px] bg-purple-300/10 rounded-full blur-3xl pointer-events-none" />

      <div className="flex-[1]" />

      <div className="mx-auto w-full max-w-md shrink-0 bg-white/95 dark:bg-gray-900/95 backdrop-blur-xl rounded-3xl shadow-2xl shadow-violet-900/20 p-6 md:p-10 relative z-10 transition-all duration-300 ease-in-out">
        {/* Brand watermark */}
        <BrandWatermark className="absolute top-4 right-4" opacity={0.03} />

        {/* Brand header — compact on mobile register */}
        <div className={`flex flex-col items-center ${mode === 'register' ? 'mb-5' : 'mb-8'}`}>
          <div className="relative mb-3 md:mb-4">
            <LogoMark size={mode === 'register' ? 48 : 64} className="rounded-2xl shadow-lg shadow-violet-500/20" />
            <div className="absolute -bottom-1 -right-1 w-5 h-5 bg-gradient-to-br from-emerald-400 to-emerald-500 rounded-full border-2 border-white flex items-center justify-center">
              <svg viewBox="0 0 20 20" fill="white" className="w-3 h-3"><path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" /></svg>
            </div>
          </div>
          <h1 className={`font-bold text-gray-900 dark:text-gray-100 tracking-tight ${mode === 'register' ? 'text-xl' : 'text-2xl'}`}>FinArch</h1>
          <p className="text-xs text-gray-400 dark:text-gray-500 mt-0.5 tracking-wide">{t('login.subtitle')}</p>
        </div>

        {justVerified && (
          <div className="mb-5 bg-emerald-50 dark:bg-emerald-900/30 border border-emerald-200 dark:border-emerald-700 text-emerald-700 dark:text-emerald-400 rounded-xl px-4 py-3 text-sm flex items-center gap-2.5">
            <div className="w-5 h-5 rounded-full bg-emerald-500 flex items-center justify-center shrink-0">
              <svg viewBox="0 0 20 20" fill="white" className="w-3 h-3"><path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" /></svg>
            </div>
            <span>{t('login.status.verified')}</span>
          </div>
        )}
        {accountDeleted && (
          <div className="mb-5 bg-gray-50 dark:bg-gray-800 border border-gray-200 dark:border-gray-700 text-gray-600 dark:text-gray-400 rounded-xl px-4 py-3 text-sm flex items-center gap-2.5">
            <span className="text-gray-400">✓</span><span>{t('login.status.deleted')}</span>
          </div>
        )}
        {emailChanged && (
          <div className="mb-5 bg-emerald-50 dark:bg-emerald-900/30 border border-emerald-200 dark:border-emerald-700 text-emerald-700 dark:text-emerald-400 rounded-xl px-4 py-3 text-sm flex items-center gap-2.5">
            <div className="w-5 h-5 rounded-full bg-emerald-500 flex items-center justify-center shrink-0">
              <svg viewBox="0 0 20 20" fill="white" className="w-3 h-3"><path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" /></svg>
            </div>
            <span>{t('login.status.emailChanged')}</span>
          </div>
        )}
        {tokenError && (
          <div className="mb-5 bg-rose-50 dark:bg-rose-900/30 border border-rose-200 dark:border-rose-700 text-rose-700 dark:text-rose-400 rounded-xl px-4 py-3 text-sm">
            {t('login.status.tokenError')}
          </div>
        )}

        {/* Login/Register tabs */}
        <div className={`flex rounded-xl bg-gray-100/80 dark:bg-gray-800/80 p-1 ${mode === 'register' ? 'mb-4' : 'mb-6'}`}>
          <button type="button" className={`flex-1 rounded-lg py-2.5 text-sm font-semibold transition-all ${mode === 'login' ? 'bg-white dark:bg-gray-700 shadow-sm text-violet-600 dark:text-violet-400' : 'text-gray-400 hover:text-gray-600 dark:hover:text-gray-300'}`}
            onClick={() => switchMode('login')}>{t('login.tabs.login')}</button>
          <button type="button" className={`flex-1 rounded-lg py-2.5 text-sm font-semibold transition-all ${mode === 'register' ? 'bg-white dark:bg-gray-700 shadow-sm text-violet-600 dark:text-violet-400' : 'text-gray-400 hover:text-gray-600 dark:hover:text-gray-300'}`}
            onClick={() => switchMode('register')}>{t('login.tabs.register')}</button>
        </div>

        <form onSubmit={handleSubmit} className={mode === 'register' ? 'space-y-3' : 'space-y-4'}>
          {mode === 'register' && (
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">{t('login.fields.username')}</label>
              <input type="text" required value={username} onChange={(e) => setUsername(e.target.value)}
                className={inputClass} placeholder={t('login.fields.usernamePlaceholder')}
                autoComplete="username" />
              <p className="mt-1 text-[11px] text-gray-400 dark:text-gray-500">{t('login.fields.usernameHint')}</p>
            </div>
          )}
          {mode === 'register' && (
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">{t('login.fields.nickname')} <span className="text-gray-400 dark:text-gray-500 font-normal">({t('login.fields.nicknameOptional')})</span></label>
              <input type="text" value={nickname} onChange={(e) => setNickname(e.target.value)}
                className={inputClass} placeholder={t('login.fields.nicknamePlaceholder')}
                maxLength={20} />
              <p className="mt-1 text-[11px] text-gray-400 dark:text-gray-500">{t('login.fields.nicknameHint')}</p>
            </div>
          )}
          <div>
            <label className={`block text-sm font-medium text-gray-700 dark:text-gray-300 ${mode === 'register' ? 'mb-1' : 'mb-1.5'}`}>{t('login.fields.email')}</label>
            <input type="email" required value={email} onChange={(e) => setEmail(e.target.value)}
              className={inputClass} placeholder="user@example.com" />
          </div>
          <div>
            <label className={`block text-sm font-medium text-gray-700 dark:text-gray-300 ${mode === 'register' ? 'mb-1' : 'mb-1.5'}`}>{t('login.fields.password')}</label>
            <input type="password" required minLength={8} value={password} onChange={(e) => setPassword(e.target.value)}
              className={inputClass} placeholder={mode === 'register' ? t('login.fields.passwordPlaceholderRegister') : t('login.fields.passwordPlaceholderLogin')} />
            {mode === 'register' && <PasswordStrength password={password} />}
          </div>

          {turnstileSiteKey && configLoaded && (
            <div className="flex justify-center min-h-[65px] overflow-hidden">
              <Turnstile ref={turnstileRef} siteKey={turnstileSiteKey}
                onSuccess={(token) => setCaptchaToken(token)}
                onExpire={() => setCaptchaToken('')}
                onError={() => { setCaptchaToken(''); setError(t('login.captchaLoadError')) }}
                options={{ theme: 'light', language: i18n.language === 'en' ? 'en' : 'zh-cn', size: 'flexible' }} />
            </div>
          )}

          {error && (
            <div className="bg-rose-50 dark:bg-rose-900/30 border border-rose-200 dark:border-rose-700 text-rose-700 dark:text-rose-400 rounded-xl px-4 py-3 text-sm">{error}</div>
          )}

          {unverifiedEmail && (
            <div className="bg-amber-50 dark:bg-amber-900/30 border border-amber-200 dark:border-amber-700 text-amber-800 dark:text-amber-400 rounded-xl px-4 py-3 text-sm space-y-1.5">
              <p>{t('login.unverifiedHint')}</p>
              {!resendDone ? (
                <button type="button" onClick={handleResend} disabled={resendLoading}
                  className="text-violet-600 dark:text-violet-400 hover:text-violet-700 dark:hover:text-violet-300 font-medium hover:underline text-xs disabled:opacity-50 transition-colors">
                  {resendLoading ? t('login.sending') : t('login.verification.resend')}
                </button>
              ) : (
                <p className="text-xs text-emerald-500 font-medium">{t('login.verification.resent')}</p>
              )}
            </div>
          )}

          <button type="submit" disabled={loading || (!!turnstileSiteKey && !captchaToken)}
            className="w-full bg-gradient-to-r from-violet-600 to-purple-600 hover:from-violet-700 hover:to-purple-700 disabled:opacity-50 text-white font-semibold rounded-xl py-3 text-sm transition-all shadow-lg shadow-violet-500/25 hover:shadow-violet-500/40 active:scale-[0.98]">
            {loading ? t('login.processing') : mode === 'login' ? t('login.submitLogin') : t('login.submitRegister')}
          </button>
        </form>

        {mode === 'login' && (
          <div className="text-center mt-5 space-y-2">
            <Link to="/forgot-password" className="text-xs text-gray-400 hover:text-violet-600 dark:hover:text-violet-400 transition-colors font-medium block">
              {t('login.forgotPassword')}
            </Link>
            <Link to="/disaster-restore" className="text-xs text-gray-400 hover:text-violet-600 dark:hover:text-violet-400 transition-colors font-medium block">
              {t('login.disasterRestore')}
            </Link>
          </div>
        )}

        {/* Footer */}
        <div className={`${mode === 'register' ? 'mt-5 pt-4' : 'mt-8 pt-5'} border-t border-gray-100 dark:border-gray-800 text-center`}>
          <p className="text-[10px] text-gray-300 dark:text-gray-600 tracking-wider">POWERED BY FINARCH · v2.2</p>
        </div>
      </div>

      <div className="flex-[3]" />
    </div>
  )
}