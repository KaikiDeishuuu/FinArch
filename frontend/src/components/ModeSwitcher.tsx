import { useTranslation } from 'react-i18next'
import { useMode } from '../contexts/ModeContext'

interface ModeSwitcherProps {
    variant?: 'header' | 'sidebar'
}

export default function ModeSwitcher({ variant = 'header' }: ModeSwitcherProps) {
    const { mode, setMode } = useMode()
    const { t } = useTranslation()

    const isSidebar = variant === 'sidebar'

    const baseBtn = 'font-semibold rounded-lg transition-all duration-150'
    const sizeClass = isSidebar ? 'px-2 py-1.5 text-xs' : 'text-[11px] py-1 rounded-md'

    const activeClass =
        'bg-white dark:bg-gray-700 text-violet-600 dark:text-violet-300 shadow-sm'
    const inactiveClass =
        'text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-300'

    return (
        <div
            className={`grid grid-cols-2 bg-gray-100 dark:bg-gray-800 overflow-hidden ${isSidebar ? 'rounded-xl p-1' : 'rounded-lg p-0.5 w-full max-w-[10rem]'
                }`}
        >
            <button
                onClick={() => setMode('work')}
                className={`${baseBtn} ${sizeClass} ${mode === 'work' ? activeClass : inactiveClass
                    } min-w-0 truncate`}
            >
                {t('mode.work')}
            </button>
            <button
                onClick={() => setMode('life')}
                className={`${baseBtn} ${sizeClass} ${mode === 'life' ? activeClass : inactiveClass
                    } min-w-0 truncate`}
            >
                {t('mode.life')}
            </button>
        </div>
    )
}
