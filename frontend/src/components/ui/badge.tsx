import type { ComponentProps } from 'react'

import { cn } from '../../lib/utils'

const badgeVariantClasses = {
  neutral: 'bg-muted text-muted-foreground',
  accent: 'bg-accent-soft text-accent',
  positive: 'bg-positive-soft text-positive',
  negative: 'bg-negative-soft text-negative',
  warning: 'bg-warning-soft text-warning',
  mode: 'bg-mode-soft text-mode text-[10px] font-semibold tracking-[0.08em]',
} as const

export type BadgeVariant = keyof typeof badgeVariantClasses

export type BadgeProps = ComponentProps<'span'> & {
  variant?: BadgeVariant
  dot?: boolean
}

export function Badge({ className, variant = 'neutral', dot = false, children, ...props }: BadgeProps) {
  return (
    <span
      className={cn(
        'inline-flex h-5 shrink-0 items-center gap-1.5 rounded-full px-2 text-xs font-medium whitespace-nowrap',
        badgeVariantClasses[variant],
        className,
      )}
      {...props}
    >
      {dot ? <span aria-hidden="true" className="size-1.5 rounded-full bg-current opacity-85" /> : null}
      {children}
    </span>
  )
}
