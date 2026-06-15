import { createContext } from 'react'

export interface ConfigState {
  turnstileSiteKey: string
  emailVerificationRequired: boolean
  loaded: boolean
}

export const ConfigContext = createContext<ConfigState>({
  turnstileSiteKey: '',
  emailVerificationRequired: false,
  loaded: false,
})
