import { useState } from 'react'
import type { FormEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { forgotPassword } from '../api/client'
import { AuthShell } from '../components/AuthShell'
import { AuthStatus } from '../components/AuthStatus'
import { Alert } from '../components/ui/alert'
import { Button, ButtonLink } from '../components/ui/button'
import { Field, Input } from '../components/ui/input'

export default function ForgotPasswordPage() {
  const { t } = useTranslation()
  const [email, setEmail] = useState('')
  const [loading, setLoading] = useState(false)
  const [sent, setSent] = useState(false)
  const [error, setError] = useState('')

  async function handleSubmit(event: FormEvent) {
    event.preventDefault()
    setError('')
    setLoading(true)
    try {
      await forgotPassword(email)
      setSent(true)
    } catch {
      setError(t('forgotPassword.toast.error'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <AuthShell>
      {sent ? (
        <AuthStatus
          tone="success"
          title={t('forgotPassword.sentTitle')}
          description={t('forgotPassword.sentDesc')}
          hint={t('forgotPassword.sentHint')}
          actions={
            <ButtonLink to="/login" size="lg" className="w-full">
              {t('forgotPassword.backToLogin')}
            </ButtonLink>
          }
        />
      ) : (
        <div className="space-y-5">
          <div className="text-center">
            <h2 className="text-lg font-semibold tracking-tight text-foreground">{t('forgotPassword.title')}</h2>
            <p className="mt-1.5 text-sm leading-6 text-muted-foreground">{t('forgotPassword.desc')}</p>
          </div>

          <form onSubmit={handleSubmit} className="space-y-4">
            <Field label={t('forgotPassword.emailLabel')}>
              <Input
                type="email"
                required
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                placeholder={t('forgotPassword.emailPlaceholder')}
                autoComplete="email"
                autoFocus
              />
            </Field>
            {error ? <Alert variant="negative">{error}</Alert> : null}
            <Button
              type="submit"
              size="lg"
              className="w-full"
              loading={loading}
              loadingText={t('forgotPassword.submitting')}
            >
              {t('forgotPassword.submit')}
            </Button>
          </form>

          <div className="text-center">
            <ButtonLink to="/login" variant="ghost" size="sm">
              {t('forgotPassword.backToLogin')}
            </ButtonLink>
          </div>
        </div>
      )}
    </AuthShell>
  )
}
