/**
 * PageTransition — 页面级动效包装器
 * ─────────────────────────────────────────────────────────────────────────────
 *
 * 设计意图：
 *   用极轻的 opacity + 6px y-offset 淡入，营造内容从"虚空"中稳定浮现的感觉。
 *   退场时向上 4px 淡出，暗示"内容被收回"的空间连续性。
 *
 * 用户心理：
 *   财务数据需要"可信、稳定"的视觉印象。页面切换没有跳跃感，
 *   用户潜意识中感受到系统是连贯的，减少认知中断。
 *
 * 为什么不用炫酷动画：
 *   财务系统需要让用户专注于数字本身。大幅度滑动、弹跳会分散注意力，
 *   且在频繁切换 tab 时造成视觉疲劳。6px 淡入让切换"无声"地发生。
 *
 * transition 参数：
 *   duration: 0.28s — 足够感知顺滑，不会让用户等待
 *   ease: [0.4, 0, 0.2, 1] — Material/Apple 标准 easeInOut
 */
import { motion } from 'framer-motion'
import type { ReactNode } from 'react'
import { pageVariants, T_PAGE } from './tokens'

interface Props {
  children: ReactNode
  /** 可选：用作 AnimatePresence key */
  motionKey?: string
}

export default function PageTransition({ children, motionKey }: Props) {
  return (
    <motion.div
      key={motionKey}
      variants={pageVariants}
      initial="initial"
      animate="animate"
      exit="exit"
      transition={T_PAGE}
    >
      {children}
    </motion.div>
  )
}
