import { createContext } from 'react'

export interface ConfigState {
  turnstileSiteKey: string
  captchaEnabled: boolean
  emailVerificationRequired: boolean
  systemOperationsEnabled: boolean
  loaded: boolean
  loadError: boolean
}

export const ConfigContext = createContext<ConfigState>({
  turnstileSiteKey: '',
  captchaEnabled: false,
  emailVerificationRequired: false,
  systemOperationsEnabled: false,
  loaded: false,
  loadError: false,
})
