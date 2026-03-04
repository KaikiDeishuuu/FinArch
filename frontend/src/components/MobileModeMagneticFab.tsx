import { useCallback, useEffect, useRef, useState } from 'react'
import { useMode } from '../contexts/ModeContext'
import { useTranslation } from 'react-i18next'

/**
 * Magnetic floating pill for mobile mode switching.
 * - Fixed bottom-right, md:hidden
 * - Magnetic hover: tracks pointer within ~80px radius, shifts max 4px
 * - Tap: scale bounce + optional vibrate + halo pulse
 */
export default function MobileModeMagneticFab() {
    const { mode, setMode } = useMode()
    const { t } = useTranslation()
    const fabRef = useRef<HTMLButtonElement>(null)
    const [offset, setOffset] = useState({ x: 0, y: 0 })
    const [tapping, setTapping] = useState(false)
    const [haloPulse, setHaloPulse] = useState(false)

    // Magnetic hover: track pointer proximity
    const handlePointerMove = useCallback((e: PointerEvent) => {
        const el = fabRef.current
        if (!el) return
        const rect = el.getBoundingClientRect()
        const cx = rect.left + rect.width / 2
        const cy = rect.top + rect.height / 2
        const dx = e.clientX - cx
        const dy = e.clientY - cy
        const dist = Math.sqrt(dx * dx + dy * dy)

        if (dist < 80) {
            const strength = 1 - dist / 80
            setOffset({
                x: (dx / dist) * strength * 4,
                y: (dy / dist) * strength * 4,
            })
        } else {
            setOffset({ x: 0, y: 0 })
        }
    }, [])

    useEffect(() => {
        window.addEventListener('pointermove', handlePointerMove, { passive: true })
        return () => window.removeEventListener('pointermove', handlePointerMove)
    }, [handlePointerMove])

    const handleTap = () => {
        // Haptic feedback
        if (navigator.vibrate) navigator.vibrate(10)

        // Scale animation
        setTapping(true)
        setTimeout(() => setTapping(false), 250)

        // Halo pulse
        setHaloPulse(true)
        setTimeout(() => setHaloPulse(false), 400)

        // Toggle mode
        setMode(mode === 'work' ? 'life' : 'work')
    }

    const isWork = mode === 'work'
    const nextLabel = isWork ? t('mode.life') : t('mode.work')

    return (
        <button
            ref={fabRef}
            onClick={handleTap}
            aria-label={`Switch to ${nextLabel}`}
            className="md:hidden fixed z-50 flex items-center gap-2 h-12 px-5 rounded-full backdrop-blur-md transition-all duration-150 select-none active:scale-95"
            style={{
                right: 20,
                bottom: 'calc(5rem + env(safe-area-inset-bottom))',
                transform: `translate(${offset.x}px, ${offset.y}px) scale(${tapping ? 1.05 : 1})`,
                background: isWork
                    ? 'linear-gradient(to right, rgba(59,130,246,0.9), rgba(99,102,241,0.9))'
                    : 'linear-gradient(to right, rgba(52,211,153,0.9), rgba(45,212,191,0.9))',
                boxShadow: isWork
                    ? '0 4px 20px rgba(59,130,246,0.3)'
                    : '0 4px 20px rgba(52,211,153,0.3)',
            }}
        >
            {/* Halo glow layer */}
            <span
                className="absolute inset-0 rounded-full blur-xl transition-opacity duration-300 pointer-events-none"
                style={{
                    background: isWork
                        ? 'linear-gradient(to right, rgba(59,130,246,0.5), rgba(99,102,241,0.5))'
                        : 'linear-gradient(to right, rgba(52,211,153,0.5), rgba(45,212,191,0.5))',
                    opacity: haloPulse ? 0.6 : 0.25,
                }}
            />

            {/* Icon */}
            <span className="relative z-10 text-white">
                {isWork ? (
                    <svg viewBox="0 0 20 20" fill="currentColor" className="w-4.5 h-4.5">
                        <path fillRule="evenodd" d="M6 3.75A2.75 2.75 0 018.75 1h2.5A2.75 2.75 0 0114 3.75v.443c.572.055 1.14.122 1.706.2C17.053 4.582 18 5.75 18 7.07v3.469c0 1.126-.694 2.191-1.83 2.54-1.952.6-4.03.921-6.17.921s-4.219-.322-6.17-.922C2.694 12.73 2 11.665 2 10.539V7.07c0-1.321.947-2.489 2.294-2.676A41.047 41.047 0 016 4.193V3.75zm6.5 0v.325a41.622 41.622 0 00-5 0V3.75c0-.69.56-1.25 1.25-1.25h2.5c.69 0 1.25.56 1.25 1.25zM10 10a1 1 0 00-1 1v.01a1 1 0 001 1h.01a1 1 0 001-1V11a1 1 0 00-1-1H10z" clipRule="evenodd" />
                        <path d="M3 15.055v-.684c.126.053.255.1.39.142 2.092.642 4.313.987 6.61.987 2.297 0 4.518-.345 6.61-.987.135-.041.264-.089.39-.142v.684c0 1.347-.985 2.53-2.363 2.686A41.454 41.454 0 0110 18c-1.572 0-3.118-.12-4.637-.26C3.985 17.586 3 16.402 3 15.056z" />
                    </svg>
                ) : (
                    <svg viewBox="0 0 20 20" fill="currentColor" className="w-4.5 h-4.5">
                        <path d="M10.707 2.293a1 1 0 00-1.414 0l-7 7a1 1 0 001.414 1.414L4 10.414V17a1 1 0 001 1h2a1 1 0 001-1v-2a1 1 0 011-1h2a1 1 0 011 1v2a1 1 0 001 1h2a1 1 0 001-1v-6.586l.293.293a1 1 0 001.414-1.414l-7-7z" />
                    </svg>
                )}
            </span>

            {/* Label */}
            <span className="relative z-10 text-white text-xs font-bold tracking-wide">
                {nextLabel}
            </span>
        </button>
    )
}
