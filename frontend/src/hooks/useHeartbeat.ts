import { useEffect, useRef } from 'react'
import { useAuth } from '../contexts/AuthContext'
import { sendHeartbeat } from '../api/client'

const DEVICE_ID_KEY = 'finarch_device_id'
const HEARTBEAT_INTERVAL = 2 * 60 * 1000 // 2 minutes

function getOrCreateDeviceId(): string {
  let id = localStorage.getItem(DEVICE_ID_KEY)
  if (!id) {
    id = crypto.randomUUID()
    localStorage.setItem(DEVICE_ID_KEY, id)
  }
  return id
}

/**
 * Sends periodic heartbeat to the server so the backend knows this device is online.
 * Should be called once at the app root level (e.g., in DashboardPage or a layout).
 */
export function useHeartbeat() {
  const { user } = useAuth()
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null)

  useEffect(() => {
    if (!user) return

    const deviceId = getOrCreateDeviceId()

    // Send immediately on mount
    sendHeartbeat(deviceId).catch(() => {})

    // Then every 2 minutes
    intervalRef.current = setInterval(() => {
      sendHeartbeat(deviceId).catch(() => {})
    }, HEARTBEAT_INTERVAL)

    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current)
    }
  }, [user])
}
