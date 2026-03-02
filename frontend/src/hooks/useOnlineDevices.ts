import { useQuery } from '@tanstack/react-query'
import { useAuth } from '../contexts/AuthContext'
import { getOnlineDevices } from '../api/client'

export function useOnlineDevices() {
  const { user } = useAuth()
  return useQuery({
    queryKey: ['onlineDevices', user?.id],
    queryFn: async () => {
      const res = await getOnlineDevices()
      return res.count
    },
    enabled: !!user,
    refetchInterval: 60_000, // refresh every minute
    staleTime: 30_000,
  })
}
