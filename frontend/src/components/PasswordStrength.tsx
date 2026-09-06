import { useTranslation } from 'react-i18next'

import { cn } from '../lib/utils'
import { ProgressBar } from './ui/progress-bar'

export type PasswordStrengthValue = 'none' | 'weak' | 'medium' | 'strong'

interface PasswordStrengthProps {
  password: string
  labelPrefix?: 'login.passwordStrength' | 'settings.password.strength'
  detailed?: boolean
}

function calculatePasswordStrength(password: string): PasswordStrengthValue {
  if (!password) return 'none'
  if (password.length < 8 || /^\d+$/.test(password)) return 'weak'

  let score = 0
  if (/[a-z]/.test(password)) score += 1
  if (/[A-Z]/.test(password)) score += 1
  if (/[0-9]/.test(password)) score += 1
  if (/[^a-zA-Z0-9]/.test(password)) score += 1

  if (score <= 1) return 'weak'
  if (score === 2) return 'medium'
  return 'strong'
}

export function PasswordStrength({
  password,
  labelPrefix = 'settings.password.strength',
  detailed = false,
}: PasswordStrengthProps) {
  const { t } = useTranslation()
  const strength = calculatePasswordStrength(password)
  if (strength === 'none') return null

  const value = { weak: 33, medium: 66, strong: 100 }[strength]
  const tone = ({
    weak: 'negative',
    medium: 'warning',
    strong: 'positive',
  } as const)[strength]
  const labelKey = detailed && strength !== 'strong' ? `${strength}Hint` : strength
  const label = t(`${labelPrefix}.${labelKey}`)

  return (
    <div className="mt-1.5 space-y-1.5">
      <ProgressBar value={value} tone={tone} label={label} />
      <p
        className={cn(
          'text-xs font-medium',
          strength === 'weak' && 'text-negative',
          strength === 'medium' && 'text-warning',
          strength === 'strong' && 'text-positive',
        )}
      >
        {label}
      </p>
    </div>
  )
}
