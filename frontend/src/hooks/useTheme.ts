import { useContext } from 'react'
import { ThemeContext } from '../contexts/themeContextCore'

export function useTheme() {
  return useContext(ThemeContext)
}
