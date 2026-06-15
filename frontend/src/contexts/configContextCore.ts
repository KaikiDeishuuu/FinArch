import { createContext } from 'react'

export interface ConfigState {
  turnstileSiteKey: string
  captchaEnabled: boolean
  emailVerificationRequired: boolean
  loaded: boolean
  loadError: boolean
}

export const ConfigContext = createContext<ConfigState>({
  turnstileSiteKey: '',
  captchaEnabled: false,
  emailVerificationRequired: false,
  loaded: false,
  loadError: false,
})
