import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { APP_NAME, APP_VERSION } from '../constants/app'
import { cn } from '../lib/utils'
import { LogoMark } from './Brand'
import { Card } from './ui/card'

interface AuthShellProps {
  children: ReactNode
  className?: string
}

export function AuthShell({ children, className }: AuthShellProps) {
  const { t } = useTranslation()

  return (
    <main
      className="relative min-h-dvh overflow-x-hidden bg-background px-4 py-6 text-foreground sm:px-6 sm:py-10"
      style={{ paddingBottom: 'max(1.5rem, env(safe-area-inset-bottom))' }}
    >
      <div
        aria-hidden="true"
        className="pointer-events-none absolute inset-x-0 top-0 h-64"
        style={{
          background: 'radial-gradient(ellipse at top, color-mix(in srgb, var(--accent) 10%, transparent), transparent 72%)',
        }}
      />

      <div className="relative mx-auto flex min-h-[calc(100dvh-5rem)] w-full max-w-md items-center">
        <Card className={cn('w-full overflow-hidden p-0 shadow-sm', className)}>
          <div className="flex items-center gap-3 border-b border-border px-5 py-4 sm:px-6">
            <LogoMark size={40} decorative className="rounded-lg" />
            <div className="min-w-0">
              <h1 className="text-lg font-semibold tracking-[-0.02em] text-foreground">{APP_NAME}</h1>
              <p className="truncate text-xs text-muted-foreground">{t('login.subtitle')}</p>
            </div>
          </div>

          <div className="px-5 py-5 sm:px-6 sm:py-6">{children}</div>

          <div className="border-t border-border px-5 py-3 text-center sm:px-6">
            <p className="text-[10px] font-medium tracking-[0.12em] text-subtle">
              {APP_NAME.toUpperCase()} · v{APP_VERSION}
            </p>
          </div>
        </Card>
      </div>
    </main>
  )
}
