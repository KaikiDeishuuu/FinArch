import type { ComponentProps } from 'react'

import { cn } from '../../lib/utils'

const toneClasses = {
  default: 'bg-positive',
  positive: 'bg-positive',
  warning: 'bg-warning',
  negative: 'bg-negative',
  accent: 'bg-accent',
} as const

type ProgressBarLabel =
  | { label: string; 'aria-label'?: never; 'aria-labelledby'?: never }
  | { label?: never; 'aria-label': string; 'aria-labelledby'?: never }
  | { label?: never; 'aria-label'?: never; 'aria-labelledby': string }

export type ProgressBarProps = Omit<ComponentProps<'div'>, 'value' | 'aria-label' | 'aria-labelledby' | 'aria-valuetext'> & ProgressBarLabel & {
  'aria-valuetext'?: string
  value: number
  max?: number
  tone?: keyof typeof toneClasses
}

export function ProgressBar({
  className,
  value,
  max = 100,
  tone = 'default',
  label,
  ...props
}: ProgressBarProps) {
  const safeMax = Number.isFinite(max) && max > 0 ? max : 100
  const safeValue = Number.isFinite(value) ? value : 0
  const clampedValue = Math.min(Math.max(safeValue, 0), safeMax)
  const percentage = (clampedValue / safeMax) * 100

  return (
    <div
      aria-label={label}
      aria-valuemax={safeMax}
      aria-valuemin={0}
      aria-valuenow={clampedValue}
      role="progressbar"
      className={cn('h-1.5 overflow-hidden rounded-full bg-muted', className)}
      {...props}
    >
      <div className={cn('h-full rounded-full transition-[width]', toneClasses[tone])} style={{ width: `${percentage}%` }} />
    </div>
  )
}
