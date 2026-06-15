import type { HTMLAttributes, ReactNode } from 'react'
import { cn } from '../lib/utils'

interface FinanceCardProps extends HTMLAttributes<HTMLDivElement> {
  interactive?: boolean
}

export function FinanceCard({ className, interactive = false, children, ...props }: FinanceCardProps) {
  return (
    <div
      className={cn(
        'rounded-2xl border border-gray-100/80 bg-white p-4 shadow-sm dark:border-gray-800/50 dark:bg-[hsl(260,15%,11%)] md:p-5',
        interactive && 'transition-all hover:-translate-y-0.5 hover:border-violet-200 hover:shadow-md dark:hover:border-violet-500/40 dark:hover:shadow-black/20',
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
    <div className="mb-4 flex items-start justify-between gap-3">
      <div className="min-w-0">
        <h2 className="text-sm font-semibold text-gray-800 dark:text-gray-100">{title}</h2>
        {subtitle && <p className="mt-1 text-xs leading-relaxed text-gray-400 dark:text-gray-500">{subtitle}</p>}
      </div>
      {action && <div className="shrink-0">{action}</div>}
    </div>
  )
}

export function EmptyState({ title, description, action }: { title: ReactNode; description?: ReactNode; action?: ReactNode }) {
  return (
    <div className="rounded-xl border border-dashed border-gray-200 bg-gray-50/70 px-4 py-5 text-center dark:border-gray-700 dark:bg-gray-800/35">
      <p className="text-sm font-semibold text-gray-700 dark:text-gray-200">{title}</p>
      {description && <p className="mx-auto mt-1 max-w-sm text-xs leading-relaxed text-gray-400 dark:text-gray-500">{description}</p>}
      {action && <div className="mt-3">{action}</div>}
    </div>
  )
}

export function ProgressBar({ value, tone = 'default', className }: { value: number; tone?: 'default' | 'success' | 'warning' | 'danger'; className?: string }) {
  const pct = Math.max(0, Math.min(100, Math.round(value * 100)))
  const toneClass = {
    default: 'from-violet-500 to-indigo-500',
    success: 'from-emerald-500 to-teal-500',
    warning: 'from-amber-400 to-orange-500',
    danger: 'from-rose-500 to-orange-500',
  }[tone]
  return (
    <div className={cn('h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-gray-800', className)}>
      <div className={cn('h-full rounded-full bg-gradient-to-r transition-all duration-500', toneClass)} style={{ width: `${pct}%` }} />
    </div>
  )
}
