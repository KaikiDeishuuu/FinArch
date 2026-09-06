import type { ComponentProps, ReactNode } from 'react'

import { cn } from '../../lib/utils'

export type EmptyStateProps = ComponentProps<'section'> & {
  title: string
  description?: string
  icon?: ReactNode
  action?: ReactNode
}

export function EmptyState({
  className,
  title,
  description,
  icon,
  action,
  ...props
}: EmptyStateProps) {
  return (
    <section
      className={cn('grid justify-items-center gap-2 rounded-xl border border-dashed border-border bg-card px-5 py-10 text-center', className)}
      {...props}
    >
      {icon ? <span className="grid size-10 place-items-center rounded-md bg-muted text-muted-foreground [&>svg]:size-5">{icon}</span> : null}
      <h2 className="mt-1 text-sm font-semibold text-foreground">{title}</h2>
      {description ? <p className="max-w-md text-sm text-muted-foreground">{description}</p> : null}
      {action ? <div className="mt-2">{action}</div> : null}
    </section>
  )
}
