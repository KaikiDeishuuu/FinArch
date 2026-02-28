import { createContext, useContext, useState, useCallback } from 'react'
import type { ReactNode } from 'react'
import { login as apiLogin, register as apiRegister, setToken } from '../api/client'
import type { AuthResponse, LoginRequest, RegisterRequest } from '../api/client'

const SESSION_KEY = 'finarch_session'

interface AuthState {
  user: { id: string; email: string; name: string; role: string } | null
  token: string | null
}

interface AuthContextValue extends AuthState {
  login: (req: LoginRequest) => Promise<void>
  // Returns true if email verification is pending (202), false if auto-logged in (201)
  register: (req: RegisterRequest) => Promise<boolean>
  logout: () => void
  isAuthenticated: boolean
}

const AuthContext = createContext<AuthContextValue | null>(null)

function loadSession(): AuthState {
  try {
    const raw = sessionStorage.getItem(SESSION_KEY)
    if (raw) {
      const saved = JSON.parse(raw) as AuthState
      if (saved.token && saved.user) {
        setToken(saved.token)
        return saved
      }
    }
  } catch { /* ignore */ }
  return { user: null, token: null }
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>(loadSession)

  const applyAuth = useCallback((resp: AuthResponse) => {
    setToken(resp.token)
    const next: AuthState = {
      token: resp.token,
      user: { id: resp.user_id, email: resp.email, name: resp.name, role: resp.role },
    }
    sessionStorage.setItem(SESSION_KEY, JSON.stringify(next))
    setState(next)
  }, [])

  const login = useCallback(async (req: LoginRequest) => {
    const resp = await apiLogin(req)
    applyAuth(resp)
  }, [applyAuth])

  const register = useCallback(async (req: RegisterRequest): Promise<boolean> => {
    const resp = await apiRegister(req)
    if (resp.token) {
      // email verification not required — auto-login
      applyAuth(resp as AuthResponse)
      return false
    }
    // 202: email verification pending
    return true
  }, [applyAuth])

  const logout = useCallback(() => {
    setToken(null)
    sessionStorage.removeItem(SESSION_KEY)
    setState({ user: null, token: null })
    window.location.href = '/login'
  }, [])

  return (
    <AuthContext.Provider value={{ ...state, login, register, logout, isAuthenticated: !!state.token }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within <AuthProvider>')
  return ctx
}
