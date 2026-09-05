import type { ButtonHTMLAttributes, HTMLAttributes, ReactNode } from 'react'
import { cn } from '../lib/utils'

interface FinanceCardProps extends HTMLAttributes<HTMLDivElement> {
  interactive?: boolean
}

export function FinanceCard({ className, interactive = false, children, ...props }: FinanceCardProps) {
  return (
    <div
      className={cn(
        'ledger-panel p-4 md:p-5',
        interactive && 'ledger-panel--interactive',
        className,
      )}
      {...props}
    >
      {children}
    </div>
  )
}

export function SectionHeader({ title, subtitle, action }: { title: ReactNode; subtitle?: ReactNode; action?: ReactNode }) {
  return (
    <div className="section-header">
      <div className="min-w-0">
        <h2 className="section-header__title">{title}</h2>
        {subtitle && <p className="section-header__subtitle">{subtitle}</p>}
      </div>
      {action && <div className="shrink-0">{action}</div>}
    </div>
  )
}

export function EmptyState({ title, description, action }: { title: ReactNode; description?: ReactNode; action?: ReactNode }) {
  return (
    <div className="empty-state">
      <p className="empty-state__title">{title}</p>
      {description && <p className="empty-state__description">{description}</p>}
      {action && <div className="mt-3">{action}</div>}
    </div>
  )
}

export function ProgressBar({ value, tone = 'default', className }: { value: number; tone?: 'default' | 'success' | 'warning' | 'danger'; className?: string }) {
  const pct = Math.max(0, Math.min(100, Math.round(value * 100)))
  return (
    <div
      className={cn('progress-track', className)}
      role="progressbar"
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={pct}
    >
      <div className="progress-track__value" data-tone={tone} style={{ width: `${pct}%` }} />
    </div>
  )
}

export function PageHeader({
  kicker,
  title,
  description,
  actions,
  className,
}: {
  kicker?: ReactNode
  title: ReactNode
  description?: ReactNode
  actions?: ReactNode
  className?: string
}) {
  return (
    <header className={cn('flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between', className)}>
      <div className="min-w-0">
        {kicker && <div className="page-kicker mb-3">{kicker}</div>}
        <h1 className="page-title m-0">{title}</h1>
        {description && (
          <p className="mt-2 max-w-2xl text-sm text-[hsl(var(--muted-foreground))]">{description}</p>
        )}
      </div>
      {actions && <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div>}
    </header>
  )
}

interface FinanceButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: 'primary' | 'secondary' | 'ghost' | 'danger'
}

export function FinanceButton({ variant = 'primary', className, type = 'button', ...props }: FinanceButtonProps) {
  return (
    <button
      type={type}
      className={cn('fin-button', variant !== 'primary' && `fin-button--${variant}`, className)}
      {...props}
    />
  )
}

export function FinanceField({
  label,
  htmlFor,
  hint,
  error,
  children,
  className,
}: {
  label: ReactNode
  htmlFor?: string
  hint?: ReactNode
  error?: ReactNode
  children: ReactNode
  className?: string
}) {
  return (
    <div className={className}>
      <label className="field-label" htmlFor={htmlFor}>{label}</label>
      {children}
      {error ? <p className="field-error" role="alert">{error}</p> : hint && <p className="field-help">{hint}</p>}
    </div>
  )
}

type BadgeTone = 'neutral' | 'work' | 'life' | 'success' | 'income' | 'warning' | 'danger' | 'expense'

export function StatusBadge({ children, tone = 'neutral', className }: { children: ReactNode; tone?: BadgeTone; className?: string }) {
  return <span className={cn('status-badge', className)} data-tone={tone}>{children}</span>
}
