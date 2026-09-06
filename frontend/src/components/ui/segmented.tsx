import type { ComponentProps } from 'react'

import { cn } from '../../lib/utils'

export function Segmented({ className, ...props }: ComponentProps<'div'>) {
  return (
    <div
      role="group"
      className={cn('inline-flex items-center gap-0.5 rounded-lg bg-muted p-0.5', className)}
      {...props}
    />
  )
}

export function SegmentedButton({ className, type = 'button', ...props }: ComponentProps<'button'>) {
  return (
    <button
      type={type}
      className={cn(
        'inline-flex h-7 items-center justify-center gap-1.5 rounded-sm px-2.5 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50 aria-pressed:bg-card aria-pressed:text-foreground aria-pressed:shadow-xs',
        className,
      )}
      {...props}
    />
  )
}
