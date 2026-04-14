import { useQuery } from '@tanstack/react-query'
import { getAccountBalanceHistory } from '../api/client'
import { useAuth } from '../contexts/AuthContext'
import { useMode } from '../contexts/ModeContext'

export type BalanceRange = '7d' | '30d' | '90d' | '1y' | 'all'

export function useAccountBalanceHistory(range: BalanceRange, accountId?: string) {
  const { user } = useAuth()
  const { mode } = useMode()

  return useQuery({
    queryKey: ['account-balance-history', user?.id, mode, range, accountId || 'all'],
    queryFn: () => getAccountBalanceHistory(mode, range, accountId),
    enabled: !!user,
    staleTime: 30_000,
  })
}
