import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useAuth } from '../hooks/useAuth'
import { listAccounts } from '../api/client'
import { useMode } from '../hooks/useMode'

export function useAccounts() {
  const { user } = useAuth()
  const { mode } = useMode()
  return useQuery({
    queryKey: ['accounts', user?.id, mode],
    queryFn: () => listAccounts(mode),
    enabled: !!user,
    staleTime: 30_000,
  })
}

export function useInvalidateAccounts() {
  const qc = useQueryClient()
  const { user } = useAuth()
  const { mode } = useMode()
  return () => qc.invalidateQueries({ queryKey: ['accounts', user?.id, mode] })
}
