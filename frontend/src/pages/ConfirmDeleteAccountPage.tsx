import { useEffect, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'

import { confirmDeleteAccount } from '../api/client'
import { AuthShell } from '../components/AuthShell'
import { AuthStatus } from '../components/AuthStatus'
import { Alert } from '../components/ui/alert'
import { Button, ButtonLink } from '../components/ui/button'
import { useActionToken } from '../hooks/useActionToken'
import { useAuth } from '../hooks/useAuth'
import { getApiError } from '../lib/errors'

async function clearPWAState() {
  try {
    localStorage.clear()
    sessionStorage.clear()
    if ('caches' in window) {
      const names = await caches.keys()
      await Promise.all(names.map((name) => caches.delete(name)))
    }
    if ('serviceWorker' in navigator) {
      const registrations = await navigator.serviceWorker.getRegistrations()
      await Promise.all(registrations.map((registration) => registration.unregister()))
    }
  } catch {
    // best effort cleanup
  }
}

export default function ConfirmDeleteAccountPage() {
  const { t } = useTranslation()
  const token = useActionToken()
  const navigate = useNavigate()
  const { clearSession } = useAuth()

  const [status, setStatus] = useState<'ready' | 'loading' | 'success' | 'error'>(token ? 'ready' : 'error')
  const [errorMsg, setErrorMsg] = useState('')
  const submittingRef = useRef(false)
  const redirectTimerRef = useRef<number | null>(null)
  const visibleErrorMsg = token ? errorMsg : t('confirmDeleteAccount.invalidLink')

  async function handleDelete() {
    if (!token || submittingRef.current) return
    submittingRef.current = true
    setStatus('loading')
    setErrorMsg('')
    try {
      await confirmDeleteAccount(token)
      setStatus('success')
      await clearPWAState()
      clearSession()
      redirectTimerRef.current = window.setTimeout(
        () => navigate('/login?deleted=1', { replace: true }),
        3000,
      )
    } catch (err: unknown) {
      setErrorMsg(getApiError(err).message || t('confirmDeleteAccount.errorDefault'))
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
          tone="warning"
          title={t('confirmDeleteAccount.readyTitle')}
          description={t('confirmDeleteAccount.readyDesc')}
          actions={
            <>
              <Alert variant="negative" className="block text-left">
                <p className="text-xs font-semibold uppercase tracking-[0.08em]">
                  {t('confirmDeleteAccount.warningTitle')}
                </p>
                <p className="mt-1 text-sm leading-6">{t('confirmDeleteAccount.warningDesc')}</p>
              </Alert>
              <Button type="button" variant="danger" size="lg" className="w-full" onClick={handleDelete}>
                {t('confirmDeleteAccount.confirmButton')}
              </Button>
              <Link to="/login" className="text-sm font-medium text-muted-foreground hover:text-accent">
                {t('confirmDeleteAccount.cancelButton')}
              </Link>
            </>
          }
        />
      ) : null}

      {status === 'loading' ? (
        <AuthStatus tone="loading" description={t('confirmDeleteAccount.processing')} />
      ) : null}

      {status === 'success' ? (
        <AuthStatus
          tone="success"
          title={t('confirmDeleteAccount.success')}
          description={t('confirmDeleteAccount.successDesc')}
          hint={t('confirmDeleteAccount.redirecting')}
          actions={
            <ButtonLink to="/login" size="lg" className="w-full">
              {t('confirmDeleteAccount.loginNow')}
            </ButtonLink>
          }
        />
      ) : null}

      {status === 'error' ? (
        <AuthStatus
          tone="error"
          title={t('confirmDeleteAccount.errorTitle')}
          description={visibleErrorMsg}
          actions={
            <>
              <ButtonLink to="/settings" size="lg" className="w-full">
                {t('confirmDeleteAccount.reapply')}
              </ButtonLink>
              <ButtonLink to="/login" variant="ghost" size="sm">
                {t('confirmDeleteAccount.backToLogin')}
              </ButtonLink>
            </>
          }
        />
      ) : null}
    </AuthShell>
  )
}
