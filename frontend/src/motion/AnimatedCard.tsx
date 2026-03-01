/**
 * AnimatedCard — 卡片 hover 微动效
 * ─────────────────────────────────────────────────────────────────────────────
 *
 * 设计意图：
 *   hover 时 scale 至 1.015（几乎不可见的放大），配合 CSS shadow 提升。
 *   模拟卡片在物理空间中"微微抬起"的效果。
 *
 * 用户心理：
 *   隐性地告知用户"这里可以交互"。不使用色彩变化或边框高亮，
 *   避免在密集的财务数据界面中产生视觉噪音。
 *
 * 为什么不用炫酷动画：
 *   卡片 hover 是用户浏览时的高频行为。如果动效过大（scale > 1.05），
 *   用户快速扫视卡片时会看到"抖动的界面"，降低信任感。
 *
 * transition 参数：
 *   duration: 0.15s — hover 反馈需要即时
 *   ease: [0.4, 0, 0.2, 1] — 入和出都柔和
 */
import { motion } from 'framer-motion'
import type { ReactNode } from 'react'
import { cardHover, T_MICRO } from './tokens'

interface Props {
  children: ReactNode
  /** 额外 className */
  className?: string
  /** 是否禁用 hover 动画 */
  disabled?: boolean
}

export default function AnimatedCard({
  children,
  className = '',
  disabled = false,
}: Props) {
  if (disabled) {
    return (
      <div className={className}>
        {children}
      </div>
    )
  }

  return (
    <motion.div
      className={className}
      variants={cardHover}
      initial="rest"
      whileHover="hover"
      whileTap={{ scale: 0.985 }}
      transition={T_MICRO}
    >
      {children}
    </motion.div>
  )
}
