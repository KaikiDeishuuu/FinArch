import { createContext, useContext, useEffect, useMemo, useState } from 'react'

export type AppMode = 'work' | 'life'

interface ModeContextValue {
  mode: AppMode
  setMode: (mode: AppMode) => void
  isWorkMode: boolean
}

const ModeContext = createContext<ModeContextValue | undefined>(undefined)
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

export function useMode() {
  const ctx = useContext(ModeContext)
  if (!ctx) throw new Error('useMode must be used inside ModeProvider')
  return ctx
}
