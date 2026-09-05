import { useCallback } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useAuth } from '../hooks/useAuth'
import { useMode } from '../hooks/useMode'
import { TRANSACTIONS_QUERY_KEY } from './useTransactions'

export function useRefreshFinanceData() {
  const qc = useQueryClient()
  const { user } = useAuth()
  const { mode } = useMode()

  return useCallback(() => {
    qc.invalidateQueries({ queryKey: TRANSACTIONS_QUERY_KEY(user?.id, mode) })
    // A WORK transaction may use a personal account, whose account query is
    // intentionally scoped as LIFE. Invalidate both mode-specific caches.
    qc.invalidateQueries({ queryKey: ['accounts', user?.id] })
    qc.invalidateQueries({ queryKey: ['account-balance-history', user?.id] })
  }, [qc, user?.id, mode])
}
