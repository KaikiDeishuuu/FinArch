import { useContext } from 'react'
import { ModeContext } from '../contexts/modeContextCore'

export function useMode() {
  const ctx = useContext(ModeContext)
  if (!ctx) throw new Error('useMode must be used inside ModeProvider')
  return ctx
}
