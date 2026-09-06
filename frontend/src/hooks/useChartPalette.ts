import { useMemo } from 'react'
import { useMode } from './useMode'
import { useTheme } from './useTheme'
import { getModeChartPalette } from '../utils/chartPalette'

export function useChartPalette() {
  const { mode } = useMode()
  const { resolved } = useTheme()

  return useMemo(() => getModeChartPalette(mode, resolved), [mode, resolved])
}
