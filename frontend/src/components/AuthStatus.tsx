import type { ReactNode } from 'react'
import { AlertTriangle, Check, Mail, ShieldCheck, X } from 'lucide-react'

import { cn } from '../lib/utils'
import { Spinner } from './ui/spinner'

export type AuthStatusTone = 'info' | 'success' | 'error' | 'warning' | 'loading'

interface AuthStatusProps {
  title?: ReactNode
  description?: ReactNode
  hint?: ReactNode
  actions?: ReactNode
  children?: ReactNode
  icon?: ReactNode
  tone?: AuthStatusTone
  className?: string
}

const toneClasses = {
  info: 'border-accent/25 bg-accent-soft text-accent',
  success: 'border-positive/25 bg-positive-soft text-positive',
  error: 'border-negative/25 bg-negative-soft text-negative',
  warning: 'border-warning/25 bg-warning-soft text-warning',
  loading: 'border-border bg-muted text-muted-foreground',
} as const

function defaultIcon(tone: AuthStatusTone) {
  if (tone === 'loading') return <Spinner size="lg" />
  if (tone === 'success') return <Check className="size-6" />
  if (tone === 'error') return <X className="size-6" />
  if (tone === 'warning') return <AlertTriangle className="size-6" />
  if (tone === 'info') return <Mail className="size-6" />
  return <ShieldCheck className="size-6" />
}

export function AuthStatus({
  title,
  description,
  hint,
  actions,
  children,
  icon,
  tone = 'info',
  className,
}: AuthStatusProps) {
  return (
    <div className={cn('text-center', className)}>
      <div
        aria-hidden="true"
        className={cn(
          'mx-auto flex size-12 items-center justify-center rounded-xl border',
          toneClasses[tone],
        )}
      >
        {icon ?? defaultIcon(tone)}
      </div>

      {title || description ? (
        <div className="mt-4 space-y-1.5">
          {title ? <h2 className="text-lg font-semibold tracking-tight text-foreground">{title}</h2> : null}
          {description ? <div className="text-sm leading-6 text-muted-foreground">{description}</div> : null}
        </div>
      ) : null}

      {children ? <div className="mt-4">{children}</div> : null}
      {hint ? <div className="mt-3 text-xs leading-5 text-subtle">{hint}</div> : null}
      {actions ? <div className="mt-5 flex flex-col items-stretch gap-2">{actions}</div> : null}
    </div>
  )
}
