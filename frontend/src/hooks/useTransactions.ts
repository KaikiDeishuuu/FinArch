import { useQuery, useQueryClient } from '@tanstack/react-query'
import { listTransactions } from '../api/client'
import { useAuth } from '../hooks/useAuth'
import { useMode } from '../hooks/useMode'

export const TRANSACTIONS_QUERY_KEY = (userId?: string, mode: 'work' | 'life' = 'work') =>
  ['transactions', userId, mode] as const

export function useTransactions(modeOverride?: 'work' | 'life') {
  const { user } = useAuth()
  const { mode } = useMode()
  const queryMode = modeOverride ?? mode
  return useQuery({
    queryKey: TRANSACTIONS_QUERY_KEY(user?.id, queryMode),
    queryFn: () => listTransactions(queryMode),
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
