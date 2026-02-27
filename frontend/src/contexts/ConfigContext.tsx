import { createContext, useContext, useState, useEffect } from 'react'
import type { ReactNode } from 'react'
import { getAppConfig } from '../api/client'

interface ConfigState {
  turnstileSiteKey: string
  loaded: boolean
}

const ConfigContext = createContext<ConfigState>({ turnstileSiteKey: '', loaded: false })

export function ConfigProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<ConfigState>({ turnstileSiteKey: '', loaded: false })

  useEffect(() => {
    getAppConfig()
      .then((cfg) => setState({ turnstileSiteKey: cfg.turnstile_site_key, loaded: true }))
      .catch(() => setState({ turnstileSiteKey: '', loaded: true }))
  }, [])

  return <ConfigContext.Provider value={state}>{children}</ConfigContext.Provider>
}

export function useConfig(): ConfigState {
  return useContext(ConfigContext)
}
