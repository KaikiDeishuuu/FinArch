import { useState, useEffect } from 'react'
import type { ReactNode } from 'react'
import { getAppConfig } from '../api/client'
import { ConfigContext } from './configContextCore'
import type { ConfigState } from './configContextCore'

export function ConfigProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<ConfigState>({ turnstileSiteKey: '', loaded: false, emailVerificationRequired: false })

  useEffect(() => {
    getAppConfig()
      .then((cfg) => setState({ turnstileSiteKey: cfg.turnstile_site_key, emailVerificationRequired: cfg.email_verification_required ?? false, loaded: true }))
      .catch(() => setState({ turnstileSiteKey: '', emailVerificationRequired: false, loaded: true }))
  }, [])

  return <ConfigContext.Provider value={state}>{children}</ConfigContext.Provider>
}
