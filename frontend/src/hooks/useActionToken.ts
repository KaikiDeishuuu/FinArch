import { useLayoutEffect, useState } from 'react'
import { clearActionTokenFromURL, readActionToken } from '../utils/actionToken'

// Reading is deliberately pure so React Strict Mode can invoke the state
// initializer twice without losing the token. The layout effect scrubs the
// address bar before paint and is idempotent when Strict Mode replays effects.
export function useActionToken(): string {
  const [token] = useState(() => readActionToken())
  useLayoutEffect(() => {
    clearActionTokenFromURL()
  }, [])
  return token
}
