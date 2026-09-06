import { useState } from 'react'
import type { FormEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { resetPassword } from '../api/client'
import { AuthShell } from '../components/AuthShell'
import { AuthStatus } from '../components/AuthStatus'
import { PasswordStrength } from '../components/PasswordStrength'
import { Alert } from '../components/ui/alert'
import { Button, ButtonLink } from '../components/ui/button'
import { Field, Input, Label } from '../components/ui/input'
import { useActionToken } from '../hooks/useActionToken'
import { useAuth } from '../hooks/useAuth'
import { getApiError } from '../lib/errors'

export default function ResetPasswordPage() {
  const { t } = useTranslation()
  const { clearSession } = useAuth()
  const token = useActionToken()

  const [newPassword, setNewPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [loading, setLoading] = useState(false)
  const [success, setSuccess] = useState(false)
  const [error, setError] = useState('')

  async function handleSubmit(event: FormEvent) {
    event.preventDefault()
    setError('')
    if (newPassword !== confirm) {
      setError(t('resetPassword.toast.mismatch'))
      return
    }

    setLoading(true)
    try {
      await resetPassword(token, newPassword)
      clearSession()
      setSuccess(true)
    } catch (err: unknown) {
      setError(getApiError(err).message || t('resetPassword.toast.error'))
    } finally {
      setLoading(false)
    }
  }

  if (!token) {
    return (
      <AuthShell>
        <AuthStatus
          tone="error"
          title={t('resetPassword.invalidLink')}
          actions={
            <ButtonLink to="/forgot-password" size="lg" className="w-full">
              {t('resetPassword.reapply')}
            </ButtonLink>
          }
        />
      </AuthShell>
    )
  }

  return (
    <AuthShell>
      {success ? (
        <AuthStatus
          tone="success"
          title={t('resetPassword.success')}
          description={t('resetPassword.successDesc')}
          actions={
            <ButtonLink to="/login" size="lg" className="w-full">
              {t('resetPassword.backToLogin')}
            </ButtonLink>
          }
        />
      ) : (
        <div className="space-y-5">
          <div className="text-center">
            <h2 className="text-lg font-semibold tracking-tight text-foreground">{t('resetPassword.title')}</h2>
          </div>

          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="grid gap-1.5">
              <Label htmlFor="reset-new-password">{t('resetPassword.newPassword')}</Label>
              <Input
                id="reset-new-password"
                type="password"
                required
                minLength={8}
                value={newPassword}
                onChange={(event) => setNewPassword(event.target.value)}
                placeholder={t('resetPassword.passwordPlaceholder')}
                autoComplete="new-password"
                autoFocus
              />
              <PasswordStrength password={newPassword} />
            </div>
            <Field
              label={t('resetPassword.confirmPassword')}
              error={confirm && newPassword !== confirm ? t('resetPassword.toast.mismatch') : undefined}
            >
              <Input
                type="password"
                required
                minLength={8}
                value={confirm}
                onChange={(event) => setConfirm(event.target.value)}
                placeholder={t('resetPassword.confirmPlaceholder')}
                autoComplete="new-password"
              />
            </Field>
            {error ? <Alert variant="negative">{error}</Alert> : null}
            <Button
              type="submit"
              size="lg"
              className="w-full"
              loading={loading}
              loadingText={t('resetPassword.submitting')}
            >
              {t('resetPassword.submit')}
            </Button>
          </form>
        </div>
      )}
    </AuthShell>
  )
}
