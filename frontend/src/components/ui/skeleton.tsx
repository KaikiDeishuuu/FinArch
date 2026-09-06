import type { ComponentProps } from 'react'

import { cn } from '../../lib/utils'

export type SkeletonProps = ComponentProps<'div'> & {
  width?: string
  height?: string
  rounded?: boolean
}

export function Skeleton({
  className,
  width = 'w-full',
  height = 'h-4',
  rounded = false,
  ...props
}: SkeletonProps) {
  return (
    <div
      aria-hidden="true"
      className={cn('animate-pulse bg-muted', rounded ? 'rounded-full' : 'rounded-md', width, height, className)}
      {...props}
    />
  )
}

export function CardSkeleton({ className, ...props }: ComponentProps<'div'>) {
  return (
    <div className={cn('space-y-3 rounded-xl border border-border bg-card p-4', className)} {...props}>
      <div className="flex items-center gap-2">
        <Skeleton width="w-6" height="h-6" rounded />
        <Skeleton width="w-24" height="h-3" />
      </div>
      <Skeleton width="w-36" height="h-6" />
      <Skeleton width="w-20" height="h-3" />
    </div>
  )
}

export function RowSkeleton({ className, ...props }: ComponentProps<'div'>) {
  return (
    <div className={cn('flex items-center gap-3 px-4 py-3', className)} {...props}>
      <Skeleton width="w-3" height="h-3" rounded />
      <Skeleton width="w-24" height="h-4" />
      <Skeleton width="w-16" height="h-4" />
      <div className="flex-1" />
      <Skeleton width="w-20" height="h-4" />
      <Skeleton width="w-14" height="h-5" className="rounded-full" />
    </div>
  )
}

export default Skeleton
