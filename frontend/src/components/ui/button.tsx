import type { ComponentProps, ComponentPropsWithRef, ReactNode } from 'react'
import { Link } from 'react-router-dom'

import { cn } from '../../lib/utils'
import { Spinner } from './spinner'

const buttonVariantClasses = {
  primary: 'border-primary bg-primary text-primary-foreground hover:opacity-90',
  secondary: 'border-muted bg-muted text-foreground hover:opacity-80',
  outline: 'border-border bg-card text-foreground hover:bg-muted',
  ghost: 'border-transparent bg-transparent text-muted-foreground hover:bg-muted hover:text-foreground',
  danger: 'border-negative-soft bg-negative-soft text-negative hover:opacity-80',
  positive: 'border-positive-soft bg-positive-soft text-positive hover:opacity-80',
} as const

const buttonSizeClasses = {
  sm: 'h-8 gap-1.5 px-3 text-xs',
  md: 'h-9 gap-2 px-3.5 text-sm',
  lg: 'h-10 gap-2 px-4 text-sm',
  icon: 'size-8 p-0',
} as const

export type ButtonVariant = keyof typeof buttonVariantClasses
export type ButtonSize = keyof typeof buttonSizeClasses

// eslint-disable-next-line react-refresh/only-export-components
export function buttonVariants({
  variant = 'primary',
  size = 'md',
  className,
}: {
  variant?: ButtonVariant
  size?: ButtonSize
  className?: string
} = {}) {
  return cn(
    'inline-flex shrink-0 items-center justify-center rounded-lg border font-medium whitespace-nowrap transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:pointer-events-none disabled:opacity-50',
    buttonVariantClasses[variant],
    buttonSizeClasses[size],
    className,
  )
}

export type ButtonProps = ComponentPropsWithRef<'button'> & {
  variant?: ButtonVariant
  size?: ButtonSize
  loading?: boolean
  loadingText?: ReactNode
}

export function Button({
  className,
  variant,
  size,
  loading = false,
  loadingText,
  disabled,
  children,
  type = 'button',
  ref,
  ...props
}: ButtonProps) {
  return (
    <button
      ref={ref}
      className={buttonVariants({ variant, size, className })}
      disabled={disabled || loading}
      type={type}
      {...props}
    >
      {loading ? <Spinner /> : null}
      {loading ? (loadingText ?? children) : children}
    </button>
  )
}

export type ButtonLinkProps = ComponentProps<typeof Link> & {
  variant?: ButtonVariant
  size?: ButtonSize
  loading?: boolean
  loadingText?: ReactNode
}

export function ButtonLink({
  className,
  variant,
  size,
  loading = false,
  loadingText,
  children,
  onClick,
  tabIndex,
  ...props
}: ButtonLinkProps) {
  return (
    <Link
      aria-disabled={loading || undefined}
      className={buttonVariants({
        variant,
        size,
        className: cn(loading && 'pointer-events-none opacity-50', className),
      })}
      onClick={loading ? (event) => event.preventDefault() : onClick}
      tabIndex={loading ? -1 : tabIndex}
      {...props}
    >
      {loading ? <Spinner /> : null}
      {loading ? (loadingText ?? children) : children}
    </Link>
  )
}
