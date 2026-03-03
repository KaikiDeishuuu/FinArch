/**
 * PwaUpdatePrompt — 当 Service Worker 检测到新版本时，
 * 在屏幕底部弹出更新提示条，用户点击后刷新页面加载新版本。
 *
 * 配合 vite-plugin-pwa 的 registerType: 'prompt' 使用。
 * 使用 Framer Motion 实现流畅的滑入/退出动画。
 *
 * 防刷新循环机制：
 * - 更新后用 sessionStorage 标记，页面重载后不再重复弹窗
 * - 用户点"稍后"后本次会话不再弹窗
 * - 更新检查间隔 10 分钟，避免频繁触发
 */
import { useEffect, useState, useCallback } from 'react'
import { useRegisterSW } from 'virtual:pwa-register/react'
import { motion, AnimatePresence } from 'framer-motion'
import { useTranslation } from 'react-i18next'

const SW_JUST_UPDATED_KEY = 'pwa-just-updated'
const SW_DISMISSED_KEY   = 'pwa-dismissed'

export default function PwaUpdatePrompt() {
  const { t } = useTranslation()
  const [show, setShow] = useState(false)

  const {
    needRefresh: [needRefresh],
    updateServiceWorker,
  } = useRegisterSW({
    onRegistered(r) {
      // 每 2 分钟检查一次更新（生产环境）
      if (r) {
        setInterval(() => r.update(), 2 * 60 * 1000)
      }
    },
  })

  useEffect(() => {
    if (!needRefresh) return
    // 刚刚更新过 → 不再弹窗（防止刷新循环）
    if (sessionStorage.getItem(SW_JUST_UPDATED_KEY)) {
      sessionStorage.removeItem(SW_JUST_UPDATED_KEY)
      return
    }
    // 用户已点过"稍后" → 本次会话不再弹窗
    if (sessionStorage.getItem(SW_DISMISSED_KEY)) return
    setShow(true)
  }, [needRefresh])

  const doUpdate = useCallback(() => {
    // 标记"刚刚更新"，避免重载后再次弹窗形成循环
    sessionStorage.setItem(SW_JUST_UPDATED_KEY, Date.now().toString())
    updateServiceWorker(true)
  }, [updateServiceWorker])

  const dismiss = useCallback(() => {
    sessionStorage.setItem(SW_DISMISSED_KEY, '1')
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
          <div className="bg-gray-900/95 backdrop-blur-xl text-white rounded-2xl shadow-2xl shadow-black/20 px-4 py-3 flex items-center gap-3 ring-1 ring-white/10">
            {/* 图标 — 带呼吸脉动 */}
            <motion.div
              className="shrink-0 w-9 h-9 rounded-xl bg-violet-600 flex items-center justify-center"
              animate={{ scale: [1, 1.08, 1] }}
              transition={{ duration: 2, repeat: Infinity, ease: 'easeInOut' }}
            >
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} className="w-5 h-5">
                <path strokeLinecap="round" strokeLinejoin="round" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
              </svg>
            </motion.div>
            {/* 文字 */}
            <div className="flex-1 min-w-0">
              <p className="text-sm font-semibold leading-tight">{t('pwa.newVersion')}</p>
              <p className="text-xs text-gray-400 leading-tight mt-0.5">{t('pwa.updateDesc')}</p>
            </div>
            {/* 按钮组 */}
            <div className="flex items-center gap-2 shrink-0">
              <button
                onClick={dismiss}
                className="text-xs text-gray-400 hover:text-gray-200 px-2 py-1 rounded-lg transition-colors"
              >
                {t('pwa.later')}
              </button>
              <motion.button
                onClick={doUpdate}
                className="text-xs font-semibold bg-violet-600 hover:bg-violet-500 text-white px-3 py-1.5 rounded-xl transition-colors"
                whileTap={{ scale: 0.95 }}
              >
                {t('pwa.update')}
              </motion.button>
            </div>
          </div>
        </motion.div>
      )}
    </AnimatePresence>
  )
}
