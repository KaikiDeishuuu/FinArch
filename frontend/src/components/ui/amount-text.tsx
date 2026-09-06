import type { ComponentProps } from 'react'

import { cn } from '../../lib/utils'
import CompactAmount from '../CompactAmount'
import { formatAmount, formatAmountCompact, formatAmountExact } from '../../utils/format'

export type AmountTextProps = Omit<ComponentProps<'span'>, 'children'> & {
  amount: number
  currency: string
  compact?: boolean
  exact?: boolean
  prefix?: string
}

export function AmountText({
  className,
  amount,
  currency,
  compact = false,
  exact = false,
  prefix = '',
  ...props
}: AmountTextProps) {
  const value = exact ? formatAmountExact(amount, currency) : formatAmount(amount, currency)

  return (
    <span className={cn('tabular-nums', className)} {...props}>
      {compact ? (
        <CompactAmount
          compact={formatAmountCompact(amount, currency)}
          exact={formatAmountExact(amount, currency)}
          prefix={prefix}
        />
      ) : (
        `${prefix}${value}`
      )}
    </span>
  )
}
