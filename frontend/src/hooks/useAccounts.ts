import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useAuth } from '../contexts/AuthContext'
import { listAccounts } from '../api/client'
import { useMode } from '../contexts/ModeContext'

export function useAccounts() {
  const { user } = useAuth()
  const { mode } = useMode()
  return useQuery({
    queryKey: ['accounts', user?.id, mode],
    queryFn: () => listAccounts(mode),
    enabled: !!user,
    staleTime: 5_000,
  })
}

export function useInvalidateAccounts() {
  const qc = useQueryClient()
  const { user } = useAuth()
  const { mode } = useMode()
  return () => qc.invalidateQueries({ queryKey: ['accounts', user?.id, mode] })
}
