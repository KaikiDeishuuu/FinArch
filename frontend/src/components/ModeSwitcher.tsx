import { useTranslation } from 'react-i18next'
import { useMode } from '../hooks/useMode'
import { cn } from '../lib/utils'
import { Segmented, SegmentedButton } from './ui/segmented'

interface ModeSwitcherProps {
  variant?: 'header' | 'sidebar'
  className?: string
}

export default function ModeSwitcher({ variant = 'header', className }: ModeSwitcherProps) {
  const { mode, setMode } = useMode()
  const { t } = useTranslation()
  const isSidebar = variant === 'sidebar'

  return (
    <Segmented
      aria-label={`${t('mode.work')} / ${t('mode.life')}`}
      className={cn(
        'grid grid-cols-2',
        isSidebar ? 'w-full' : 'w-[7.25rem]',
        className,
      )}
    >
      <SegmentedButton
        onClick={() => setMode('work')}
        aria-pressed={mode === 'work'}
        className="min-w-0 px-2 font-semibold aria-pressed:text-mode"
      >
        {t('mode.work')}
      </SegmentedButton>
      <SegmentedButton
        onClick={() => setMode('life')}
        aria-pressed={mode === 'life'}
        className="min-w-0 px-2 font-semibold aria-pressed:text-mode"
      >
        {t('mode.life')}
      </SegmentedButton>
    </Segmented>
  )
}
