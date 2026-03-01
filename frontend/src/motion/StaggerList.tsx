/**
 * StaggerList — 列表级 stagger 入场动效
 * ─────────────────────────────────────────────────────────────────────────────
 *
 * 设计意图：
 *   列表中的每个子元素依次以 40ms 间隔淡入 + 4px 上浮。
 *   模拟数据"有序加载"的感觉，而非一次性"砸"到屏幕上。
 *
 * 用户心理：
 *   有序出现暗示系统在"逐条处理"数据，增强专业感。
 *   间隔极短（40ms），用户感受到的是"柔和的整体出现"而非"一条一条"。
 *
 * 为什么不用炫酷动画：
 *   财务列表可能有 50+ 条目。如果每条都有独立的飞入动画，
 *   整个页面会变成"弹幕"。极轻的 stagger 淡入是最安静的方案。
 *
 * transition 参数：
 *   staggerChildren: 0.04s — 子项之间间隔
 *   delayChildren: 0.02s — 避免首项出现过快
 *   每个子项: duration 0.22s, ease [0, 0, 0.2, 1]
 */
import { motion} from 'framer-motion'
import type { ReactNode } from 'react'
import { staggerContainer, staggerItem, T_STANDARD } from './tokens'

// ─── Container ────────────────────────────────────────────────────────────────

interface ContainerProps {
  children: ReactNode
  className?: string
}

export function StaggerContainer({ children, className }: ContainerProps) {
  return (
    <motion.div
      className={className}
      variants={staggerContainer}
      initial="initial"
      animate="animate"
    >
      {children}
    </motion.div>
  )
}

// ─── Item ─────────────────────────────────────────────────────────────────────

interface ItemProps {
  children: ReactNode
  className?: string
}

export function StaggerItem({ children, className }: ItemProps) {
  return (
    <motion.div
      className={className}
      variants={staggerItem}
      transition={T_STANDARD}
    >
      {children}
    </motion.div>
  )
}
