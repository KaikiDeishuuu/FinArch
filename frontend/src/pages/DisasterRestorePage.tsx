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
    <div className="min-h-screen bg-gray-50 dark:bg-gray-950 flex items-center justify-center px-4 py-12">
      <div className="w-full max-w-lg bg-white dark:bg-gray-900 rounded-2xl border border-gray-100 dark:border-gray-800 shadow-sm p-6 text-center space-y-3">
        <div className="mx-auto flex h-10 w-10 items-center justify-center rounded-full bg-violet-50 text-violet-600 dark:bg-violet-500/10 dark:text-violet-300" aria-hidden="true">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" className="h-5 w-5">
            <rect x="3" y="11" width="18" height="10" rx="2" />
            <path d="M7 11V7a5 5 0 0110 0v4" />
          </svg>
        </div>
        <h1 className="text-xl font-semibold text-gray-900 dark:text-gray-100">{title}</h1>
        <p className="text-sm text-gray-500 dark:text-gray-400">{description}</p>
        <Link className="inline-block pt-2 text-sm font-medium text-violet-600 hover:underline dark:text-violet-400" to="/">
          {t('disasterRestore.backToDashboard')}
        </Link>
      </div>
    </div>
  )
}
