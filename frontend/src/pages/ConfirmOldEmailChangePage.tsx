import { useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { MailCheck } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { confirmOldEmailForChange } from '../api/client'
import { AuthShell } from '../components/AuthShell'
import { AuthStatus } from '../components/AuthStatus'
import { Button, ButtonLink } from '../components/ui/button'
import { useActionToken } from '../hooks/useActionToken'
import { getApiError } from '../lib/errors'

export default function ConfirmOldEmailChangePage() {
  const { t } = useTranslation()
  const token = useActionToken()

  const [status, setStatus] = useState<'ready' | 'loading' | 'success' | 'error'>(token ? 'ready' : 'error')
  const [errorMsg, setErrorMsg] = useState('')
  const submittingRef = useRef(false)
  const visibleErrorMsg = token ? errorMsg : t('confirmOldEmailChange.invalidLink')

  async function handleConfirm() {
    if (!token || submittingRef.current) return
    submittingRef.current = true
    setStatus('loading')
    setErrorMsg('')
    try {
      await confirmOldEmailForChange(token)
      setStatus('success')
    } catch (err: unknown) {
      setErrorMsg(getApiError(err).message || t('confirmOldEmailChange.errorDefault'))
      setStatus('error')
    } finally {
      submittingRef.current = false
    }
  }

  return (
    <AuthShell>
      {status === 'ready' ? (
        <AuthStatus
          tone="info"
          title={t('confirmOldEmailChange.readyTitle')}
          description={t('confirmOldEmailChange.readyDesc')}
          actions={
            <>
              <Button type="button" size="lg" className="w-full" onClick={handleConfirm}>
                {t('confirmOldEmailChange.confirmButton')}
              </Button>
              <Link to="/login" className="text-xs font-medium text-muted-foreground hover:text-accent">
                {t('confirmOldEmailChange.cancelButton')}
              </Link>
            </>
          }
        />
      ) : null}

      {status === 'loading' ? (
        <AuthStatus tone="loading" description={t('confirmOldEmailChange.processing')} />
      ) : null}

      {status === 'success' ? (
        <AuthStatus
          tone="success"
          icon={<MailCheck className="size-6" />}
          title={t('confirmOldEmailChange.success')}
          description={t('confirmOldEmailChange.successDesc')}
          hint={t('confirmOldEmailChange.successHint')}
          actions={
            <ButtonLink to="/login" size="lg" className="w-full">
              {t('confirmOldEmailChange.backToLogin')}
            </ButtonLink>
          }
        />
      ) : null}

      {status === 'error' ? (
        <AuthStatus
          tone="error"
          title={t('confirmOldEmailChange.errorTitle')}
          description={visibleErrorMsg}
          actions={
            <>
              <ButtonLink to="/settings" size="lg" className="w-full">
                {t('confirmOldEmailChange.backToSettings')}
              </ButtonLink>
              <ButtonLink to="/login" variant="ghost" size="sm">
                {t('confirmOldEmailChange.backToLogin')}
              </ButtonLink>
            </>
          }
        />
      ) : null}
    </AuthShell>
  )
}
