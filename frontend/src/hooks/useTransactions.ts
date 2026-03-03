import { useQuery, useQueryClient } from '@tanstack/react-query'
import { listTransactions } from '../api/client'
import { useAuth } from '../contexts/AuthContext'
import { useMode } from '../contexts/ModeContext'

export const TRANSACTIONS_QUERY_KEY = (userId?: string, mode: 'work' | 'life' = 'work') =>
  ['transactions', userId, mode] as const

export function useTransactions() {
  const { user } = useAuth()
  const { mode } = useMode()
  return useQuery({
    queryKey: TRANSACTIONS_QUERY_KEY(user?.id, mode),
    queryFn: () => listTransactions(mode),
    staleTime: 30_000,
    enabled: !!user,
  })
}

export function useInvalidateTransactions() {
  const { user } = useAuth()
  const { mode } = useMode()
  const qc = useQueryClient()
  return () => qc.invalidateQueries({ queryKey: TRANSACTIONS_QUERY_KEY(user?.id, mode) })
}
