import type { ComponentProps, ReactNode } from 'react'

import { cn } from '../../lib/utils'

export type StatTileProps = ComponentProps<'section'> & {
  label: string
  value: ReactNode
  hint?: string
  icon?: ReactNode
  tone?: 'default' | 'positive' | 'negative' | 'warning'
}

export function StatTile({
  className,
  label,
  value,
  hint,
  icon,
  tone = 'default',
  ...props
}: StatTileProps) {
  return (
    <section className={cn('rounded-xl border border-border bg-card p-4 shadow-xs', className)} {...props}>
      <div className="flex items-center justify-between gap-3">
        <p className="text-xs font-medium text-muted-foreground">{label}</p>
        {icon ? (
          <span className="grid size-6 shrink-0 place-items-center rounded-sm bg-muted text-muted-foreground [&>svg]:size-3.5">
            {icon}
          </span>
        ) : null}
      </div>
      <div
        className={cn(
          'mt-1.5 text-xl font-semibold tracking-tight tabular-nums',
          tone === 'positive' && 'text-positive',
          tone === 'negative' && 'text-negative',
          tone === 'warning' && 'text-warning',
        )}
      >
        {value}
      </div>
      {hint ? <p className="mt-1 text-xs text-muted-foreground">{hint}</p> : null}
    </section>
  )
}
