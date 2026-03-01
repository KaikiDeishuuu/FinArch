import { createContext, useContext, useState, useCallback, useEffect } from 'react'
import type { ReactNode } from 'react'
import { login as apiLogin, register as apiRegister, refreshToken as apiRefresh, setToken } from '../api/client'
import type { AuthResponse, LoginRequest, RegisterRequest } from '../api/client'

const SESSION_KEY = 'finarch_session'
// Refresh when < 1 hour of TTL remains; check every 10 minutes
const REFRESH_THRESHOLD_MS = 60 * 60 * 1000
const CHECK_INTERVAL_MS = 10 * 60 * 1000

interface AuthState {
  user: { id: string; email: string; username: string; role: string } | null
  token: string | null
  expiresAt: number | null  // unix ms
}

interface AuthContextValue extends Omit<AuthState, 'expiresAt'> {
  login: (req: LoginRequest) => Promise<void>
  register: (req: RegisterRequest) => Promise<boolean>
  logout: () => void
  isAuthenticated: boolean
}

const AuthContext = createContext<AuthContextValue | null>(null)

/** Decode the exp claim from a JWT (no signature verification). */
function jwtExpMs(token: string): number | null {
  try {
    const payload = JSON.parse(atob(token.split('.')[1].replace(/-/g, '+').replace(/_/g, '/')))
    return typeof payload.exp === 'number' ? payload.exp * 1000 : null
  } catch { return null }
}

function loadSession(): AuthState {
  try {
    const raw = localStorage.getItem(SESSION_KEY)
    if (raw) {
      const saved = JSON.parse(raw) as AuthState
      if (saved.token && saved.user) {
        const exp = saved.expiresAt ?? jwtExpMs(saved.token)
        if (exp && Date.now() < exp) {
          setToken(saved.token)
          return { ...saved, expiresAt: exp }
        }
      }
    }
  } catch { /* ignore */ }
  return { user: null, token: null, expiresAt: null }
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>(loadSession)

  const applyAuth = useCallback((resp: AuthResponse) => {
    setToken(resp.token)
    const exp = jwtExpMs(resp.token)
    const next: AuthState = {
      token: resp.token,
      expiresAt: exp,
      user: { id: resp.user_id, email: resp.email, username: resp.username, role: resp.role },
    }
    localStorage.setItem(SESSION_KEY, JSON.stringify(next))
    setState(next)
  }, [])

  const login = useCallback(async (req: LoginRequest) => {
    const resp = await apiLogin(req)
    applyAuth(resp)
  }, [applyAuth])

  const register = useCallback(async (req: RegisterRequest): Promise<boolean> => {
    const resp = await apiRegister(req)
    if (resp.token) {
      applyAuth(resp as AuthResponse)
      return false
    }
    return true
  }, [applyAuth])

  const logout = useCallback(() => {
    setToken(null)
    localStorage.removeItem(SESSION_KEY)
    setState({ user: null, token: null, expiresAt: null })
    window.location.href = '/login'
  }, [])

  // Sliding-window auto-refresh: when token has < 1h remaining, silently re-issue
  useEffect(() => {
    if (!state.token) return
    const check = async () => {
      if (!state.expiresAt || !state.token) return
      const remaining = state.expiresAt - Date.now()
      if (remaining > 0 && remaining < REFRESH_THRESHOLD_MS) {
        try {
          const resp = await apiRefresh()
          applyAuth(resp)
        } catch { /* ignore; 401 will be caught by axios interceptor */ }
      }
    }
    check() // run immediately on mount / state change
    const id = setInterval(check, CHECK_INTERVAL_MS)
    return () => clearInterval(id)
  }, [state.token, state.expiresAt, applyAuth])

  return (
    <AuthContext.Provider value={{ user: state.user, token: state.token, login, register, logout, isAuthenticated: !!state.token }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within <AuthProvider>')
  return ctx
}
