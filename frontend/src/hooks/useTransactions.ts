import { useQuery, useQueryClient } from '@tanstack/react-query'
import { listTransactions } from '../api/client'
import { useAuth } from '../contexts/AuthContext'

export const TRANSACTIONS_QUERY_KEY = (userId?: string) =>
  ['transactions', userId] as const

export function useTransactions() {
  const { user } = useAuth()
  return useQuery({
    queryKey: TRANSACTIONS_QUERY_KEY(user?.id),
    queryFn: () => listTransactions(),
    staleTime: 30_000,
    enabled: !!user,
  })
}

export function useInvalidateTransactions() {
  const { user } = useAuth()
  const qc = useQueryClient()
  return () => qc.invalidateQueries({ queryKey: TRANSACTIONS_QUERY_KEY(user?.id) })
}
