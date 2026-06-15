import { useContext } from 'react'
import { ConfigContext } from '../contexts/configContextCore'

export function useConfig() {
  return useContext(ConfigContext)
}
