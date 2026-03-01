/**
 * useHaptic — lightweight haptic feedback hook for mobile PWA.
 *
 * Calls navigator.vibrate() when available (Android Chrome, some iOS PWAs).
 * Silently no-ops on unsupported devices.
 */
export function useHaptic() {
  function vibrate(pattern: number | number[] = 50) {
    if (typeof navigator !== 'undefined' && 'vibrate' in navigator) {
      try {
        navigator.vibrate(pattern)
      } catch {
        // Silently ignore — some browsers throw on certain patterns
      }
    }
  }

  return {
    /** Short tap (default: 50ms) — for confirmations, toggles */
    tap: () => vibrate(50),
    /** Double pulse — for successful saves */
    success: () => vibrate([40, 30, 40]),
    /** Error buzz — for validation failures */
    error: () => vibrate([80, 40, 80]),
    /** Long press — for destructive actions */
    heavy: () => vibrate(120),
  }
}
