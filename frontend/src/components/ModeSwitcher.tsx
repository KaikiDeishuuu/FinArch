import { useTranslation } from 'react-i18next'
import { useMode } from '../hooks/useMode'

interface ModeSwitcherProps {
    variant?: 'header' | 'sidebar'
}

export default function ModeSwitcher({ variant = 'header' }: ModeSwitcherProps) {
  const { mode, setMode } = useMode()
  const { t } = useTranslation()

  return (
    <div className="mode-switcher" data-variant={variant}>
      <button
        type="button"
        aria-pressed={mode === 'work'}
        data-active={mode === 'work'}
        onClick={() => setMode('work')}
        className="mode-switcher__button"
      >
        {t('mode.work')}
      </button>
      <button
        type="button"
        aria-pressed={mode === 'life'}
        data-active={mode === 'life'}
        onClick={() => setMode('life')}
        className="mode-switcher__button"
      >
        {t('mode.life')}
      </button>
    </div>
  )
}
