import { createContext } from 'react'

export type AppMode = 'work' | 'life'

export interface ModeContextValue {
  mode: AppMode
  setMode: (mode: AppMode) => void
  isWorkMode: boolean
}

export const ModeContext = createContext<ModeContextValue | undefined>(undefined)
