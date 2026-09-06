import { Children, cloneElement, isValidElement, useId, type ComponentProps, type ReactElement } from 'react'

import { cn } from '../../lib/utils'

const controlClasses = 'w-full rounded-lg border border-input bg-card px-3 text-sm text-foreground shadow-xs outline-none placeholder:text-subtle focus:border-ring focus:ring-2 focus:ring-ring/20 disabled:cursor-not-allowed disabled:opacity-50 aria-[invalid=true]:border-negative aria-[invalid=true]:focus:ring-negative/20'

export function Input({ className, ...props }: ComponentProps<'input'>) {
  return <input className={cn('h-9', controlClasses, className)} {...props} />
}

export function Textarea({ className, ...props }: ComponentProps<'textarea'>) {
  return <textarea className={cn('min-h-20 py-2', controlClasses, className)} {...props} />
}

export function Label({ className, ...props }: ComponentProps<'label'>) {
  return <label className={cn('text-xs font-medium text-muted-foreground', className)} {...props} />
}

type FieldControlProps = {
  id?: string
  'aria-describedby'?: string
  'aria-invalid'?: boolean | 'false' | 'true' | 'grammar' | 'spelling'
}

export type FieldProps = Omit<ComponentProps<'div'>, 'children'> & {
  label?: string
  hint?: string
  error?: string
  htmlFor?: string
  children: ReactElement<FieldControlProps>
}

export function Field({
  className,
  label,
  hint,
  error,
  htmlFor,
  children,
  ...props
}: FieldProps) {
  const generatedId = useId()
  const controlId = htmlFor ?? children.props.id ?? `field-${generatedId}`
  const message = error ?? hint
  const messageId = message ? `${controlId}-message` : undefined
  const describedBy = [children.props['aria-describedby'], messageId].filter(Boolean).join(' ') || undefined
  const control = Children.only(children)

  return (
    <div className={cn('grid gap-1.5', className)} {...props}>
      {label ? <Label htmlFor={controlId}>{label}</Label> : null}
      {isValidElement(control) ? cloneElement(control, {
        id: controlId,
        'aria-describedby': describedBy,
        'aria-invalid': error ? true : control.props['aria-invalid'],
      }) : control}
      {message ? (
        <p id={messageId} className={cn('text-xs', error ? 'text-negative' : 'text-subtle')}>
          {message}
        </p>
      ) : null}
    </div>
  )
}
