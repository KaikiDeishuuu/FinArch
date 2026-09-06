import { ArrowLeft, LockKeyhole, ShieldCheck } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { ButtonLink } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { useConfig } from '../hooks/useConfig'

export default function DisasterRestorePage() {
  const { t } = useTranslation()
  const { systemOperationsEnabled } = useConfig()
  const title = systemOperationsEnabled
    ? t('disasterRestore.browserRestrictedTitle')
    : t('disasterRestore.operationsDisabledTitle')
  const description = systemOperationsEnabled
    ? t('disasterRestore.browserRestrictedDesc')
    : t('disasterRestore.operationsDisabledDesc')

  return (
    <div className="grid min-h-[calc(100dvh-8rem)] place-items-center py-8">
      <Card className="w-full max-w-lg p-6 text-center md:p-8">
        <div className="mx-auto grid size-11 place-items-center rounded-lg border border-border bg-muted text-muted-foreground" aria-hidden="true">
          {systemOperationsEnabled ? <ShieldCheck className="size-5" /> : <LockKeyhole className="size-5" />}
        </div>
        <h1 className="mt-4 text-xl font-semibold tracking-tight text-foreground">{title}</h1>
        <p className="mx-auto mt-2 max-w-md text-sm leading-relaxed text-muted-foreground">{description}</p>
        <ButtonLink to="/" variant="outline" className="mt-5">
          <ArrowLeft className="size-4" />
          {t('disasterRestore.backToDashboard')}
        </ButtonLink>
      </Card>
    </div>
  )
}
