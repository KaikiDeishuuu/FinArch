import { createContext } from 'react'
import type { LoginRequest, RegisterRequest } from '../api/client'

export interface AuthUser {
  id: string
  email: string
  username: string
  nickname: string
  role: string
}

export interface AuthState {
  user: AuthUser | null
  token: string | null
  expiresAt: number | null
}

export interface AuthContextValue extends Omit<AuthState, 'expiresAt'> {
  login: (req: LoginRequest) => Promise<void>
  register: (req: RegisterRequest) => Promise<boolean>
  logout: () => void
  updateUser: (patch: Partial<AuthUser>) => void
  isAuthenticated: boolean
}

export const AuthContext = createContext<AuthContextValue | null>(null)
