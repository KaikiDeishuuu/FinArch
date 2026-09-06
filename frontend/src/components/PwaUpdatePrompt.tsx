/**
 * PwaUpdatePrompt — 当 Service Worker 检测到新版本时，
 * 在屏幕底部弹出更新提示条，用户点击后刷新页面加载新版本。
 *
 * 配合 vite-plugin-pwa 的 registerType: 'prompt' 使用。
 * 使用 Framer Motion 实现流畅的滑入/退出动画。
 *
 * 防刷新循环机制：
 * - 更新后用 sessionStorage 标记，页面重载后不再重复弹窗
 * - 用户点"稍后"后进入 10 分钟冷却，之后可再次提醒
 * - 更新检查间隔 1 分钟，并在回到前台/恢复联网时主动检查
 */
import { useEffect, useState, useCallback, useRef } from 'react'
import { useRegisterSW } from 'virtual:pwa-register/react'
import { motion, AnimatePresence } from 'framer-motion'
import { RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from './ui/button'

const SW_JUST_UPDATED_KEY = 'pwa-just-updated'
const SW_DISMISSED_KEY = 'pwa-dismissed-at'
const DISMISS_SNOOZE_MS = 10 * 60 * 1000
const UPDATE_CHECK_INTERVAL_MS = 60 * 1000

export default function PwaUpdatePrompt() {
  const { t } = useTranslation()
  const [show, setShow] = useState(false)
  const registrationRef = useRef<ServiceWorkerRegistration | null>(null)

  const canShowPrompt = useCallback(() => {
    const dismissedAt = Number(sessionStorage.getItem(SW_DISMISSED_KEY) ?? 0)
    if (!dismissedAt) return true
    if (Date.now() - dismissedAt > DISMISS_SNOOZE_MS) {
      sessionStorage.removeItem(SW_DISMISSED_KEY)
      return true
    }
    return false
  }, [])

  const maybeShowPrompt = useCallback(() => {
    // 刚刚更新过 → 不再弹窗（防止刷新循环）
    if (sessionStorage.getItem(SW_JUST_UPDATED_KEY)) {
      sessionStorage.removeItem(SW_JUST_UPDATED_KEY)
      return
    }
    // 用户已点过“稍后”且仍在冷却窗口内
    if (!canShowPrompt()) return
    setShow(true)
  }, [canShowPrompt])

  const {
    needRefresh: [needRefresh],
    updateServiceWorker,
  } = useRegisterSW({
    immediate: true,
    onNeedRefresh() {
      maybeShowPrompt()
    },
    onRegisteredSW(_swUrl, registration) {
      if (!registration) return
      registrationRef.current = registration
      const checkForUpdates = () => {
        registration.update()
        if (registration.waiting) maybeShowPrompt()
      }
      // 注册后先主动检查一次，减少首轮更新提示延迟
      checkForUpdates()
      const timer = window.setInterval(checkForUpdates, UPDATE_CHECK_INTERVAL_MS)
      const onVisible = () => {
        if (document.visibilityState === 'visible') checkForUpdates()
      }
      const onOnline = () => checkForUpdates()
      const onFocus = () => checkForUpdates()
      window.addEventListener('visibilitychange', onVisible)
      window.addEventListener('online', onOnline)
      window.addEventListener('focus', onFocus)
      return () => {
        window.clearInterval(timer)
        window.removeEventListener('visibilitychange', onVisible)
        window.removeEventListener('online', onOnline)
        window.removeEventListener('focus', onFocus)
      }
    },
    onRegistered(r) {
      // 兼容旧版本 vite-plugin-pwa 的注册回调
      if (r) {
        r.update()
      }
    },
  })

  useEffect(() => {
    if (!needRefresh) return
    const id = window.setTimeout(maybeShowPrompt, 0)
    return () => window.clearTimeout(id)
  }, [needRefresh, maybeShowPrompt])

  useEffect(() => {
    if (!registrationRef.current?.waiting) return
    const id = window.setTimeout(maybeShowPrompt, 0)
    return () => window.clearTimeout(id)
  }, [maybeShowPrompt])

  const doUpdate = useCallback(() => {
    // 标记"刚刚更新"，避免重载后再次弹窗形成循环
    sessionStorage.setItem(SW_JUST_UPDATED_KEY, Date.now().toString())
    updateServiceWorker(true)
  }, [updateServiceWorker])

  const dismiss = useCallback(() => {
    sessionStorage.setItem(SW_DISMISSED_KEY, Date.now().toString())
    setShow(false)
  }, [])

  return (
    <AnimatePresence>
      {show && (
        <motion.div
          className="fixed bottom-4 left-1/2 z-[9999] w-[calc(100%-2rem)] max-w-sm"
          initial={{ opacity: 0, y: 60, x: '-50%' }}
          animate={{ opacity: 1, y: 0, x: '-50%' }}
          exit={{ opacity: 0, y: 40, x: '-50%', transition: { duration: 0.25, ease: 'easeIn' } }}
          transition={{ type: 'spring', damping: 26, stiffness: 300 }}
        >
          <div
            role="status"
            className="flex items-center gap-3 rounded-xl border border-border bg-card px-4 py-3 text-card-foreground shadow-[var(--shadow-sm)]"
          >
            <div className="grid size-9 shrink-0 place-items-center rounded-lg bg-accent-soft text-accent">
              <RefreshCw className="size-4.5" />
            </div>
            <div className="min-w-0 flex-1">
              <p className="text-sm font-semibold leading-tight">{t('pwa.newVersion')}</p>
              <p className="mt-0.5 text-xs leading-tight text-muted-foreground">{t('pwa.updateDesc')}</p>
            </div>
            <div className="flex shrink-0 items-center gap-1">
              <Button variant="ghost" size="sm" onClick={dismiss}>
                {t('pwa.later')}
              </Button>
              <Button size="sm" onClick={doUpdate}>
                {t('pwa.update')}
              </Button>
            </div>
          </div>
        </motion.div>
      )}
    </AnimatePresence>
  )
}
