import { useCallback, useEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import {
  getToken,
  isSessionRefreshSuperseded,
  login as apiLogin,
  logoutSession as apiLogout,
  refreshSession,
  register as apiRegister,
  setSessionListener,
  setToken,
} from '../api/client'
import type { AuthResponse, LoginRequest, RegisterRequest } from '../api/client'
import { secureRandomHex } from '../utils/secureRandom'
import {
  getSessionRecoveryDelay,
  isDefinitiveSessionInvalid,
  SESSION_RECOVERY_MAX_ATTEMPTS,
} from '../utils/sessionRecovery'
import { AuthContext } from './authContextCore'
import type { AuthState, AuthUser } from './authContextCore'

const LEGACY_SESSION_KEY = 'finarch_session'
const AUTH_EVENT_KEY = 'finarch_auth_event'
const LOGOUT_PENDING_KEY = 'finarch_logout_pending'
const AUTH_CHANNEL = 'finarch-auth'

type AuthEvent = 'session_changed' | 'logout' | 'logout_pending'
type RecoveryControl = { cancel: () => void; retry: () => void }

const emptyState = (): AuthState => ({ user: null, token: null, expiresAt: null })
let logoutPendingInMemory = false

function isAuthEvent(value: unknown): value is AuthEvent {
  return value === 'session_changed' || value === 'logout' || value === 'logout_pending'
}

function setLogoutPending(pending: boolean) {
  logoutPendingInMemory = pending
  try {
    if (pending) localStorage.setItem(LOGOUT_PENDING_KEY, '1')
    else localStorage.removeItem(LOGOUT_PENDING_KEY)
  } catch {
    // Private browsing policies may disable storage.
  }
}

function persistedLogoutPending(): boolean | null {
  try {
    return localStorage.getItem(LOGOUT_PENDING_KEY) === '1'
  } catch {
    return null
  }
}

function isLogoutPending(): boolean {
  return logoutPendingInMemory || persistedLogoutPending() === true
}

function isAlreadyLoggedOut(error: unknown): boolean {
  return isDefinitiveSessionInvalid(error)
}

async function finishPendingLogout(): Promise<void> {
  if (!isLogoutPending()) return
  try {
    await apiLogout()
  } catch (error) {
    // An invalid or absent refresh cookie means the server-side logout goal is
    // already satisfied. Network and origin failures must retain the fence.
    if (!isAlreadyLoggedOut(error)) throw error
  }
  setLogoutPending(false)
}

function stateFromResponse(resp: AuthResponse): AuthState {
  const parsedExpiry = Date.parse(resp.expires_at)
  return {
    token: resp.token,
    expiresAt: Number.isFinite(parsedExpiry) ? parsedExpiry : null,
    user: {
      id: resp.user_id,
      email: resp.email,
      username: resp.username,
      nickname: resp.nickname || resp.username,
      role: resp.role,
    },
  }
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>(emptyState)
  const [isLoading, setIsLoading] = useState(true)
  const [sessionUnavailable, setSessionUnavailable] = useState(false)
  const channelRef = useRef<BroadcastChannel | null>(null)
  const recoveryControlRef = useRef<RecoveryControl | null>(null)

  const retrySession = useCallback(() => {
    recoveryControlRef.current?.retry()
  }, [])

  const applyAuth = useCallback((resp: AuthResponse) => {
    setToken(resp.token)
    setState(stateFromResponse(resp))
  }, [])

  const clearLocalSession = useCallback(() => {
    setToken(null)
    setState(emptyState())
  }, [])

  const publish = useCallback((event: AuthEvent) => {
    try {
      channelRef.current?.postMessage(event)
    } catch {
      // A channel can close while an async logout is settling.
    }
    // Storage events are a fallback for browsers without BroadcastChannel.
    // Only an event name and nonce are persisted; credentials never are.
    try {
      localStorage.setItem(AUTH_EVENT_KEY, JSON.stringify({ event, nonce: secureRandomHex(12) }))
      localStorage.removeItem(AUTH_EVENT_KEY)
    } catch {
      // Session correctness does not depend on cross-tab notification.
    }
  }, [])

  const login = useCallback(async (req: LoginRequest) => {
    // Do not let a new login race an older Set-Cookie response from logout.
    await finishPendingLogout()
    const resp = await apiLogin(req)
    setLogoutPending(false)
    setSessionUnavailable(false)
    setIsLoading(false)
    applyAuth(resp)
    publish('session_changed')
  }, [applyAuth, publish])

  const register = useCallback(async (req: RegisterRequest): Promise<boolean> => {
    await finishPendingLogout()
    const resp = await apiRegister(req)
    if (resp.token) {
      setLogoutPending(false)
      setSessionUnavailable(false)
      setIsLoading(false)
      applyAuth(resp as AuthResponse)
      publish('session_changed')
      return false
    }
    return true
  }, [applyAuth, publish])

  const logout = useCallback(async () => {
    setLogoutPending(true)
    recoveryControlRef.current?.cancel()
    setSessionUnavailable(false)
    setIsLoading(false)
    // Invalidate in-flight refreshes and notify other tabs before waiting for
    // the network. Login/register remain fenced until revocation completes.
    clearLocalSession()
    publish('logout_pending')
    let revoked = false
    try {
      await finishPendingLogout()
      revoked = true
    } catch {
      // Keep the non-sensitive pending flag. A later reload will retry server
      // revocation before attempting to hydrate the session.
    } finally {
      if (revoked) publish('logout')
      if (window.location.pathname !== '/login') {
        window.location.assign('/login')
      }
    }
  }, [clearLocalSession, publish])

  const clearSession = useCallback(() => {
    // Password reset, email change, and account deletion already revoke the
    // server session. The follow-up logout clears the stale cookie itself.
    setLogoutPending(true)
    recoveryControlRef.current?.cancel()
    setSessionUnavailable(false)
    setIsLoading(false)
    clearLocalSession()
    publish('logout_pending')
    void finishPendingLogout()
      .then(() => publish('logout'))
      .catch(() => {
        // Preserve the pending marker for a later retry.
      })
  }, [clearLocalSession, publish])

  const updateUser = useCallback((patch: Partial<AuthUser>) => {
    setState((previous) => {
      if (!previous.user) return previous
      return { ...previous, user: { ...previous.user, ...patch } }
    })
    publish('session_changed')
  }, [publish])

  useEffect(() => {
    // One-time migration: remove bearer tokens written by older releases.
    try {
      localStorage.removeItem(LEGACY_SESSION_KEY)
    } catch {
      // Storage can be unavailable under strict privacy policies.
    }

    let active = true
    let recoveryRunning = false
    let recoveryController: AbortController | null = null
    let wakeRetry: (() => void) | null = null

    const cancelRecovery = () => {
      recoveryController?.abort()
      wakeRetry?.()
    }

    const waitForRetry = (delayMs: number, signal: AbortSignal): Promise<boolean> => (
      new Promise((resolve) => {
        if (signal.aborted) {
          resolve(false)
          return
        }

        let settled = false
        const settle = (shouldContinue: boolean) => {
          if (settled) return
          settled = true
          globalThis.clearTimeout(timer)
          signal.removeEventListener('abort', onAbort)
          if (wakeRetry === wake) wakeRetry = null
          resolve(shouldContinue)
        }
        const onAbort = () => settle(false)
        const wake = () => settle(true)

        wakeRetry = wake
        signal.addEventListener('abort', onAbort, { once: true })
        const timer = globalThis.setTimeout(wake, delayMs)
      })
    )

    const startRecovery = async () => {
      if (!active || recoveryRunning) return

      const controller = new AbortController()
      recoveryController = controller
      recoveryRunning = true
      setIsLoading(true)
      let failureCount = 0

      try {
        while (active && !controller.signal.aborted) {
          try {
            await refreshSession()
            if (!active || controller.signal.aborted) return
            setSessionUnavailable(false)
            setIsLoading(false)
            return
          } catch (error) {
            if (!active || controller.signal.aborted) return
            // React StrictMode intentionally mounts, cleans up, and mounts the
            // provider again. Its cleanup fences the old shared refresh; the
            // new mount should retry immediately without reporting downtime.
            if (isSessionRefreshSuperseded(error)) continue
            if (isDefinitiveSessionInvalid(error)) {
              clearLocalSession()
              setSessionUnavailable(false)
              setIsLoading(false)
              return
            }

            failureCount += 1
            setSessionUnavailable(true)
            if (failureCount >= SESSION_RECOVERY_MAX_ATTEMPTS) {
              setIsLoading(false)
              return
            }

            const shouldContinue = await waitForRetry(
              getSessionRecoveryDelay(failureCount),
              controller.signal,
            )
            if (!shouldContinue) return
          }
        }
      } finally {
        recoveryRunning = false
        if (recoveryController === controller) recoveryController = null
        wakeRetry = null
      }
    }

    const retryRecovery = () => {
      if (!active) return
      if (wakeRetry) {
        wakeRetry()
        return
      }
      if (!recoveryRunning) void startRecovery()
    }

    const recoveryControl: RecoveryControl = { cancel: cancelRecovery, retry: retryRecovery }
    recoveryControlRef.current = recoveryControl

    const receive = (event: AuthEvent) => {
      if (!active) return
      if (event === 'logout' || event === 'logout_pending') {
        cancelRecovery()
        setLogoutPending(event === 'logout_pending')
        setSessionUnavailable(false)
        setIsLoading(false)
        clearLocalSession()
        return
      }
      // A delayed session_changed event cannot override an explicit logout
      // whose server revocation is still pending.
      if (persistedLogoutPending() === true) {
        cancelRecovery()
        setSessionUnavailable(false)
        setIsLoading(false)
        clearLocalSession()
        return
      }
      // The sender completed an explicit login and removed the shared pending
      // marker. Anonymous tabs use the same bounded recovery loop. An already
      // authenticated tab keeps its current access state on transient errors.
      setLogoutPending(false)
      if (!getToken()) {
        retryRecovery()
        return
      }
      void refreshSession().catch((error) => {
        if (!active || !isDefinitiveSessionInvalid(error)) return
        setLogoutPending(false)
        clearLocalSession()
        publish('logout')
      })
    }

    let useStorageFallback = true
    if ('BroadcastChannel' in window) {
      try {
        const channel = new BroadcastChannel(AUTH_CHANNEL)
        channel.onmessage = (message: MessageEvent<unknown>) => {
          if (isAuthEvent(message.data)) receive(message.data)
        }
        channelRef.current = channel
        useStorageFallback = false
      } catch {
        // Fall through to storage events when channel creation is blocked.
      }
    }

    const storageListener = (event: StorageEvent) => {
      if (event.key !== AUTH_EVENT_KEY || !event.newValue) return
      try {
        const parsed = JSON.parse(event.newValue) as { event?: AuthEvent }
        if (isAuthEvent(parsed.event)) receive(parsed.event)
      } catch {
        // Ignore malformed, non-credential coordination data.
      }
    }
    if (useStorageFallback) window.addEventListener('storage', storageListener)

    setSessionListener((session) => {
      if (!active) return
      if (session) {
        applyAuth(session)
        setSessionUnavailable(false)
        setIsLoading(false)
      } else {
        cancelRecovery()
        setLogoutPending(false)
        setSessionUnavailable(false)
        setIsLoading(false)
        clearLocalSession()
        publish('logout')
      }
    })

    const hydrate = async () => {
      if (isLogoutPending()) {
        try {
          await finishPendingLogout()
          publish('logout')
        } catch {
          // Do not refresh while an explicit logout still needs revocation.
        }
        if (!active) return
        setSessionUnavailable(false)
        setIsLoading(false)
        clearLocalSession()
        return
      }
      await startRecovery()
    }
    void hydrate()

    return () => {
      active = false
      cancelRecovery()
      // Fence and abort the module-level request as well as the local retry
      // loop. A late rotateSession response must not leave a zombie token
      // after this provider and its session listener have unmounted.
      setToken(null)
      if (recoveryControlRef.current === recoveryControl) recoveryControlRef.current = null
      setSessionListener(null)
      if (useStorageFallback) window.removeEventListener('storage', storageListener)
      channelRef.current?.close()
      channelRef.current = null
    }
  }, [applyAuth, clearLocalSession, publish])

  return (
    <AuthContext.Provider value={{
      user: state.user,
      login,
      register,
      logout,
      clearSession,
      updateUser,
      isAuthenticated: !!state.token,
      isLoading,
      sessionUnavailable,
      retrySession,
    }}>
      {children}
    </AuthContext.Provider>
  )
}
