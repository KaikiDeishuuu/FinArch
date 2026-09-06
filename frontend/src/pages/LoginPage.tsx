import { useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { Turnstile } from '@marsidev/react-turnstile'
import type { TurnstileInstance } from '@marsidev/react-turnstile'
import { Mail } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { resendVerification } from '../api/client'
import { AuthShell } from '../components/AuthShell'
import { AuthStatus } from '../components/AuthStatus'
import { PasswordStrength } from '../components/PasswordStrength'
import { Alert } from '../components/ui/alert'
import { Button } from '../components/ui/button'
import { Field, Input, Label } from '../components/ui/input'
import { Segmented, SegmentedButton } from '../components/ui/segmented'
import { useAuth } from '../hooks/useAuth'
import { useConfig } from '../hooks/useConfig'
import { useTheme } from '../hooks/useTheme'
import { getApiError } from '../lib/errors'

export default function LoginPage() {
  const { t, i18n } = useTranslation()
  const { login, register } = useAuth()
  const { resolved } = useTheme()
  const { turnstileSiteKey, captchaEnabled, loaded: configLoaded, loadError: configLoadError } = useConfig()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()

  const [mode, setMode] = useState<'login' | 'register'>('login')
  const [email, setEmail] = useState('')
  const [username, setUsername] = useState('')
  const [nickname, setNickname] = useState('')
  const [password, setPassword] = useState('')
  const [captchaToken, setCaptchaToken] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [pendingVerification, setPendingVerification] = useState(false)
  const [unverifiedEmail, setUnverifiedEmail] = useState('')
  const [resendLoading, setResendLoading] = useState(false)
  const [resendDone, setResendDone] = useState(false)
  const turnstileRef = useRef<TurnstileInstance>(null)

  const justVerified = searchParams.get('verified') === '1'
  const tokenError = searchParams.get('error') === 'invalid_token'
  const accountDeleted = searchParams.get('deleted') === '1'
  const emailChanged = searchParams.get('email_changed') === '1'
  const passwordChanged = searchParams.get('password_changed') === '1'
  const captchaRequired = captchaEnabled && Boolean(turnstileSiteKey)
  const captchaUnavailable = configLoadError || (captchaEnabled && !turnstileSiteKey)

  function switchMode(next: 'login' | 'register') {
    if (next === mode) return
    setMode(next)
    setError('')
    setPendingVerification(false)
    setUnverifiedEmail('')
    setResendDone(false)
    setCaptchaToken('')
    turnstileRef.current?.reset()
  }

  async function handleSubmit(event: FormEvent) {
    event.preventDefault()
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
        const pending = await register({
          email,
          username,
          password,
          nickname: nickname || undefined,
          captcha_token: captchaToken || undefined,
        })
        if (pending) setPendingVerification(true)
        else navigate('/')
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
    setError('')
    try {
      await resendVerification(unverifiedEmail || email)
      setResendDone(true)
    } catch (err: unknown) {
      setError(getApiError(err).message || t('login.operationFailed'))
    } finally {
      setResendLoading(false)
    }
  }

  if (pendingVerification) {
    return (
      <AuthShell>
        <AuthStatus
          tone="info"
          icon={<Mail className="size-6" />}
          title={t('login.verification.title')}
          description={t('login.verification.desc', { email })}
          actions={
            <>
              {resendDone ? (
                <Alert variant="positive" className="justify-center">
                  {t('login.verification.resent')}
                </Alert>
              ) : (
                <Button
                  type="button"
                  size="lg"
                  loading={resendLoading}
                  loadingText={t('login.sending')}
                  onClick={handleResend}
                >
                  {t('login.verification.noEmail')}
                </Button>
              )}
              {error ? <Alert variant="negative">{error}</Alert> : null}
              <Button type="button" variant="ghost" onClick={() => switchMode('login')}>
                {t('login.backToLogin')}
              </Button>
            </>
          }
        />
      </AuthShell>
    )
  }

  return (
    <AuthShell>
      <div className="space-y-5">
        <div className="space-y-2">
          {justVerified ? <Alert variant="positive">{t('login.status.verified')}</Alert> : null}
          {accountDeleted ? <Alert>{t('login.status.deleted')}</Alert> : null}
          {emailChanged ? <Alert variant="positive">{t('login.status.emailChanged')}</Alert> : null}
          {passwordChanged ? <Alert variant="positive">{t('login.status.passwordChanged')}</Alert> : null}
          {tokenError ? <Alert variant="negative">{t('login.status.tokenError')}</Alert> : null}
        </div>

        <Segmented className="grid w-full grid-cols-2" aria-label={t('login.subtitle')}>
          <SegmentedButton
            type="button"
            aria-pressed={mode === 'login'}
            onClick={() => switchMode('login')}
          >
            {t('login.tabs.login')}
          </SegmentedButton>
          <SegmentedButton
            type="button"
            aria-pressed={mode === 'register'}
            onClick={() => switchMode('register')}
          >
            {t('login.tabs.register')}
          </SegmentedButton>
        </Segmented>

        <form onSubmit={handleSubmit} className="space-y-4">
          {mode === 'register' ? (
            <Field label={t('login.fields.username')} hint={t('login.fields.usernameHint')}>
              <Input
                type="text"
                required
                value={username}
                onChange={(event) => setUsername(event.target.value)}
                placeholder={t('login.fields.usernamePlaceholder')}
                autoComplete="username"
                autoFocus
              />
            </Field>
          ) : null}

          {mode === 'register' ? (
            <Field
              label={`${t('login.fields.nickname')} (${t('login.fields.nicknameOptional')})`}
              hint={t('login.fields.nicknameHint')}
            >
              <Input
                type="text"
                value={nickname}
                onChange={(event) => setNickname(event.target.value)}
                placeholder={t('login.fields.nicknamePlaceholder')}
              />
            </Field>
          ) : null}

          <Field label={t('login.fields.email')}>
            <Input
              type="email"
              required
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              placeholder="user@example.com"
              autoComplete="email"
              autoFocus={mode === 'login'}
            />
          </Field>

          <div className="grid gap-1.5">
            <Label htmlFor="login-password">{t('login.fields.password')}</Label>
            <Input
              id="login-password"
              type="password"
              required
              minLength={8}
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              placeholder={mode === 'register'
                ? t('login.fields.passwordPlaceholderRegister')
                : t('login.fields.passwordPlaceholderLogin')}
              autoComplete={mode === 'login' ? 'current-password' : 'new-password'}
            />
            {mode === 'register' ? (
              <PasswordStrength
                password={password}
                labelPrefix="login.passwordStrength"
                detailed
              />
            ) : null}
          </div>

          {captchaRequired && configLoaded ? (
            <div className="rounded-lg border border-border bg-muted p-2">
              <div className="mx-auto max-w-full overflow-hidden" style={{ minHeight: 70 }}>
                <Turnstile
                  ref={turnstileRef}
                  siteKey={turnstileSiteKey}
                  onSuccess={setCaptchaToken}
                  onExpire={() => setCaptchaToken('')}
                  onError={() => {
                    setCaptchaToken('')
                    setError(t('login.captchaLoadError'))
                  }}
                  options={{
                    theme: resolved,
                    language: (i18n.resolvedLanguage ?? i18n.language).toLowerCase().startsWith('en') ? 'en' : 'zh-cn',
                    size: 'flexible',
                  }}
                />
              </div>
            </div>
          ) : null}

          {captchaUnavailable ? <Alert variant="negative">{t('login.configLoadError')}</Alert> : null}
          {error ? <Alert variant="negative">{error}</Alert> : null}

          {unverifiedEmail ? (
            <Alert variant="warning" className="block">
              <p>{t('login.unverifiedHint')}</p>
              {resendDone ? (
                <p className="mt-1.5 text-xs font-medium text-positive">
                  {t('login.verification.resent')}
                </p>
              ) : (
                <button
                  type="button"
                  onClick={handleResend}
                  disabled={resendLoading}
                  className="mt-1.5 text-xs font-medium text-accent hover:underline disabled:opacity-50"
                >
                  {resendLoading ? t('login.sending') : t('login.verification.resend')}
                </button>
              )}
            </Alert>
          ) : null}

          <Button
            type="submit"
            size="lg"
            className="w-full"
            loading={loading}
            loadingText={t('login.processing')}
            disabled={!configLoaded || captchaUnavailable || (captchaRequired && !captchaToken)}
          >
            {mode === 'login' ? t('login.submitLogin') : t('login.submitRegister')}
          </Button>
        </form>

        {mode === 'login' ? (
          <div className="text-center">
            <Link to="/forgot-password" className="text-xs font-medium text-muted-foreground hover:text-accent">
              {t('login.forgotPassword')}
            </Link>
          </div>
        ) : null}
      </div>
    </AuthShell>
  )
}
