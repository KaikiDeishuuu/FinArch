import { useEffect, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'

import { verifyEmail } from '../api/client'
import { AuthShell } from '../components/AuthShell'
import { AuthStatus } from '../components/AuthStatus'
import { Button, ButtonLink } from '../components/ui/button'
import { useActionToken } from '../hooks/useActionToken'
import { getApiError } from '../lib/errors'

export default function VerifyEmailPage() {
  const { t } = useTranslation()
  const token = useActionToken()
  const navigate = useNavigate()

  const [status, setStatus] = useState<'ready' | 'loading' | 'success' | 'error'>(token ? 'ready' : 'error')
  const [errorMsg, setErrorMsg] = useState('')
  const submittingRef = useRef(false)
  const redirectTimerRef = useRef<number | null>(null)
  const visibleErrorMsg = token ? errorMsg : t('verifyEmail.invalidLink')

  async function handleVerify() {
    if (!token || submittingRef.current) return
    submittingRef.current = true
    setStatus('loading')
    setErrorMsg('')
    try {
      await verifyEmail(token)
      setStatus('success')
      redirectTimerRef.current = window.setTimeout(
        () => navigate('/login?verified=1', { replace: true }),
        3000,
      )
    } catch (err: unknown) {
      setErrorMsg(getApiError(err).message || t('verifyEmail.errorDefault'))
      setStatus('error')
    } finally {
      submittingRef.current = false
    }
  }

  useEffect(() => () => {
    if (redirectTimerRef.current !== null) window.clearTimeout(redirectTimerRef.current)
  }, [])

  return (
    <AuthShell>
      {status === 'ready' ? (
        <AuthStatus
          tone="info"
          title={t('verifyEmail.readyTitle')}
          description={t('verifyEmail.readyDesc')}
          actions={
            <>
              <Button type="button" size="lg" className="w-full" onClick={handleVerify}>
                {t('verifyEmail.confirmButton')}
              </Button>
              <Link to="/login" className="text-xs font-medium text-muted-foreground hover:text-accent">
                {t('verifyEmail.backToLogin')}
              </Link>
            </>
          }
        />
      ) : null}

      {status === 'loading' ? (
        <AuthStatus tone="loading" description={t('verifyEmail.verifying')} />
      ) : null}

      {status === 'success' ? (
        <AuthStatus
          tone="success"
          title={t('verifyEmail.success')}
          description={t('verifyEmail.successDesc')}
          hint={t('verifyEmail.redirecting')}
          actions={
            <ButtonLink to="/login?verified=1" size="lg" className="w-full">
              {t('verifyEmail.loginNow')}
            </ButtonLink>
          }
        />
      ) : null}

      {status === 'error' ? (
        <AuthStatus
          tone="error"
          title={t('verifyEmail.errorTitle')}
          description={visibleErrorMsg}
          hint={t('verifyEmail.resendHint')}
          actions={
            <ButtonLink to="/login" size="lg" className="w-full">
              {t('verifyEmail.backToLogin')}
            </ButtonLink>
          }
        />
      ) : null}
    </AuthShell>
  )
}
