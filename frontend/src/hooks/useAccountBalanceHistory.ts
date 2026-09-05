import { useQueries, useQuery } from '@tanstack/react-query'
import { getAccountBalanceHistory } from '../api/client'
import { useAuth } from './useAuth'
import { useMode } from './useMode'

export type BalanceRange = '7d' | '30d' | '90d' | '1y' | 'all'

export function useAccountBalanceHistory(range: BalanceRange, accountId?: string, enabled = true) {
  const { user } = useAuth()
  const { mode } = useMode()

  return useQuery({
    queryKey: ['account-balance-history', user?.id, mode, range, accountId || 'all'],
    queryFn: () => getAccountBalanceHistory(mode, range, accountId),
    enabled: enabled && !!user,
    staleTime: 30_000,
  })
}

export function useAccountBalanceHistories(range: BalanceRange, accountIds: string[]) {
  const { user } = useAuth()
  const { mode } = useMode()

  return useQueries({
    queries: accountIds.map((accountId) => ({
      queryKey: ['account-balance-history', user?.id, mode, range, accountId],
      queryFn: () => getAccountBalanceHistory(mode, range, accountId),
      enabled: !!user,
      staleTime: 30_000,
    })),
  })
}
