import type { ComponentProps } from 'react'

import { cn } from '../../lib/utils'

export type SpinnerProps = ComponentProps<'span'> & {
  size?: 'sm' | 'md' | 'lg'
}

export function Spinner({ className, size = 'sm', ...props }: SpinnerProps) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        'inline-block shrink-0 animate-spin rounded-full border-2 border-current border-r-transparent opacity-70',
        size === 'sm' && 'size-3.5',
        size === 'md' && 'size-4',
        size === 'lg' && 'size-5',
        className,
      )}
      {...props}
    />
  )
}
