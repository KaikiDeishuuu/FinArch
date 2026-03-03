import { useCallback } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useAuth } from '../contexts/AuthContext'
import { useMode } from '../contexts/ModeContext'
import { TRANSACTIONS_QUERY_KEY } from './useTransactions'

export function useRefreshFinanceData() {
  const qc = useQueryClient()
  const { user } = useAuth()
  const { mode } = useMode()

  return useCallback(() => {
    qc.invalidateQueries({ queryKey: TRANSACTIONS_QUERY_KEY(user?.id, mode) })
    qc.invalidateQueries({ queryKey: ['accounts', user?.id, mode] })
  }, [qc, user?.id, mode])
}
