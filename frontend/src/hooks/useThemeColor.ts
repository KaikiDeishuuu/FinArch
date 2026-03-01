import { useEffect } from 'react'

/**
 * Temporarily override the <meta name="theme-color"> value while the
 * component is mounted. Restores the original value on unmount.
 *
 * This affects the mobile browser status-bar / address-bar colour.
 */
export function useThemeColor(color: string) {
  useEffect(() => {
    const meta = document.querySelector('meta[name="theme-color"]')
    if (!meta) return
    const original = meta.getAttribute('content') || '#FAFAF9'
    meta.setAttribute('content', color)
    return () => { meta.setAttribute('content', original) }
  }, [color])
}
