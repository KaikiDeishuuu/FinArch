import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useAuth } from '../contexts/AuthContext'
import { listAccounts } from '../api/client'

export function useAccounts() {
  const { user } = useAuth()
  return useQuery({
    queryKey: ['accounts', user?.id],
    queryFn: listAccounts,
    enabled: !!user,
    staleTime: 60_000,
  })
}

export function useInvalidateAccounts() {
  const qc = useQueryClient()
  const { user } = useAuth()
  return () => qc.invalidateQueries({ queryKey: ['accounts', user?.id] })
}
