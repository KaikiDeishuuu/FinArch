import { useEffect, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'

import { confirmEmailChange } from '../api/client'
import { AuthShell } from '../components/AuthShell'
import { AuthStatus } from '../components/AuthStatus'
import { Button, ButtonLink } from '../components/ui/button'
import { useActionToken } from '../hooks/useActionToken'
import { useAuth } from '../hooks/useAuth'
import { getApiError } from '../lib/errors'

export default function ConfirmEmailChangePage() {
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
      setErrorMsg(getApiError(err).message || t('confirmEmailChange.errorDefault'))
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
          title={t('confirmEmailChange.readyTitle')}
          description={t('confirmEmailChange.readyDesc')}
          actions={
            <>
              <Button type="button" size="lg" className="w-full" onClick={handleConfirm}>
                {t('confirmEmailChange.confirmButton')}
              </Button>
              <Link to="/login" className="text-xs font-medium text-muted-foreground hover:text-accent">
                {t('confirmEmailChange.cancelButton')}
              </Link>
            </>
          }
        />
      ) : null}

      {status === 'loading' ? (
        <AuthStatus tone="loading" description={t('confirmEmailChange.processing')} />
      ) : null}

      {status === 'success' ? (
        <AuthStatus
          tone="success"
          title={t('confirmEmailChange.success')}
          description={t('confirmEmailChange.successDesc')}
          hint={t('confirmEmailChange.redirecting')}
          actions={
            <ButtonLink to="/login?email_changed=1" size="lg" className="w-full">
              {t('confirmEmailChange.loginNow')}
            </ButtonLink>
          }
        />
      ) : null}

      {status === 'error' ? (
        <AuthStatus
          tone="error"
          title={t('confirmEmailChange.errorTitle')}
          description={visibleErrorMsg}
          actions={
            <>
              <ButtonLink to="/settings" size="lg" className="w-full">
                {t('confirmEmailChange.backToSettings')}
              </ButtonLink>
              <ButtonLink to="/login" variant="ghost" size="sm">
                {t('confirmEmailChange.backToLogin')}
              </ButtonLink>
            </>
          }
        />
      ) : null}
    </AuthShell>
  )
}
