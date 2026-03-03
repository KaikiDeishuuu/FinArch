import { useEffect } from 'react'
import { useTheme } from '../contexts/ThemeContext'

/**
 * Temporarily override the <meta name="theme-color"> value while the
 * component is mounted. Restores the original value on unmount.
 *
 * Accepts a light colour and an optional dark colour; the active colour is
 * chosen automatically based on the resolved theme.
 */
export function useThemeColor(lightColor: string, darkColor?: string) {
  const { resolvedTheme } = useTheme()

  useEffect(() => {
    const meta = document.querySelector('meta[name="theme-color"]')
    if (!meta) return
    const original = meta.getAttribute('content') || '#FAFAF9'
    const color = resolvedTheme === 'dark' ? (darkColor ?? '#1a1a2e') : lightColor
    meta.setAttribute('content', color)
    return () => { meta.setAttribute('content', original) }
  }, [lightColor, darkColor, resolvedTheme])
}
