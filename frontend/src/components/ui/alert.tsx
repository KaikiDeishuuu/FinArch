import type { ComponentProps } from 'react'

import { cn } from '../../lib/utils'

const alertVariantClasses = {
  neutral: 'border-border bg-card text-foreground',
  positive: 'border-positive/35 bg-positive-soft text-positive',
  negative: 'border-negative/35 bg-negative-soft text-negative',
  warning: 'border-warning/35 bg-warning-soft text-warning',
  info: 'border-accent/35 bg-accent-soft text-accent',
} as const

export type AlertVariant = keyof typeof alertVariantClasses
export type AlertProps = ComponentProps<'div'> & { variant?: AlertVariant }

export function Alert({ className, variant = 'neutral', ...props }: AlertProps) {
  return (
    <div
      role={variant === 'negative' ? 'alert' : 'status'}
      className={cn('flex gap-2.5 rounded-md border px-3 py-2.5 text-sm', alertVariantClasses[variant], className)}
      {...props}
    />
  )
}
