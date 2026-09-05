import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
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
    <main data-mode="work" className="relative flex min-h-dvh items-center justify-center overflow-y-auto bg-[var(--draft)] px-4 py-12 font-sans text-[hsl(var(--foreground))] dark:bg-[var(--ink)]">
      <div className="pointer-events-none fixed inset-y-0 left-[12%] w-px bg-[var(--rule)] opacity-40 dark:opacity-15" aria-hidden="true" />
      <section className="ledger-panel relative w-full max-w-lg overflow-hidden rounded-[4px] border border-[var(--rule)] bg-[hsl(var(--card))] text-center">
        <div className="h-1 bg-[var(--oxide)]" aria-hidden="true" />
        <div className="space-y-4 px-6 py-8 sm:px-10 sm:py-10">
        <div className="mx-auto flex h-11 w-11 items-center justify-center rounded-[3px] border border-[var(--oxide)]/35 bg-[var(--oxide)]/10 text-[var(--oxide)]" aria-hidden="true">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="h-5 w-5">
            <rect x="3" y="11" width="18" height="10" rx="2" />
            <path d="M7 11V7a5 5 0 0110 0v4" />
          </svg>
        </div>
        <h1 className="font-serif text-2xl font-semibold tracking-tight text-[hsl(var(--foreground))]">{title}</h1>
        <p className="text-sm leading-relaxed text-[hsl(var(--muted-foreground))]">{description}</p>
        <Link className="fin-button fin-button--ghost inline-flex" to="/">
          {t('disasterRestore.backToDashboard')}
        </Link>
        </div>
      </section>
    </main>
  )
}
