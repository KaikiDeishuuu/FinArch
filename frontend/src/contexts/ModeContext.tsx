import { useEffect, useMemo, useState } from 'react'
import { ModeContext } from './modeContextCore'
import type { AppMode, ModeContextValue } from './modeContextCore'
const STORAGE_KEY = 'finarch_mode'

export function ModeProvider({ children }: { children: React.ReactNode }) {
  const [mode, setModeState] = useState<AppMode>(() => {
    const raw = localStorage.getItem(STORAGE_KEY)
    return raw === 'life' ? 'life' : 'work'
  })

  useEffect(() => {
    localStorage.setItem(STORAGE_KEY, mode)
    document.documentElement.setAttribute('data-mode', mode)
  }, [mode])

  const value = useMemo<ModeContextValue>(() => ({
    mode,
    setMode: setModeState,
    isWorkMode: mode === 'work',
  }), [mode])

  return <ModeContext.Provider value={value}>{children}</ModeContext.Provider>
}
